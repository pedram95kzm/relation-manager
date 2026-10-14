package analysis

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"relations/internal/ai"
	"relations/internal/database"
	"relations/internal/platform"
)

// InitializeSettings preserves operator changes; env limits seed defaults only.
func (s *Service) InitializeSettings(ctx context.Context) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&database.AppSettings{
		ID: 1, GlobalDaily: max(1, s.cfg.GlobalDailyLimit), UserDaily: max(1, s.cfg.UserDailyLimit), UpdatedAt: s.now().UTC()}).Error
}

func (s *Service) settings(tx *gorm.DB) (database.AppSettings, error) {
	var settings database.AppSettings
	err := tx.Where("id = 1").First(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		settings = database.AppSettings{ID: 1, GlobalDaily: max(1, s.cfg.GlobalDailyLimit), UserDaily: max(1, s.cfg.UserDailyLimit), UpdatedAt: s.now().UTC()}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error; err != nil {
			return settings, err
		}
		err = tx.Where("id = 1").First(&settings).Error
	}
	return settings, err
}

func usageDay(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// All writers lock global before user counters. Reservations count attempts,
// including provider failures, so retries cannot evade cost limits.
func (s *Service) reserve(tx *gorm.DB, user database.User, now time.Time) error {
	settings, err := s.settings(tx)
	if err != nil {
		return err
	}
	userLimit := settings.UserDaily
	if user.DailyLimit != nil {
		userLimit = *user.DailyLimit
	}
	limits := []struct {
		key   string
		limit int
		code  string
	}{{"global", settings.GlobalDaily, "global_daily_limit"}, {"user:" + user.ID, userLimit, "user_daily_limit"}}
	for _, limit := range limits {
		counter := database.DailyUsage{Day: usageDay(now), ScopeKey: limit.key, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&counter).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("day = ? AND scope_key = ?", counter.Day, limit.key).First(&counter).Error; err != nil {
			return err
		}
		if counter.Counter >= int64(limit.limit) {
			return platform.Err(429, limit.code, "The daily AI generation limit has been reached. Deterministic scoring remains available.")
		}
		if err := tx.Model(&database.DailyUsage{}).Where("day = ? AND scope_key = ?", counter.Day, limit.key).
			Updates(map[string]any{"counter": counter.Counter + 1, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Usage(ctx context.Context, userID string) (Usage, error) {
	result := Usage{Day: usageDay(s.now()).Format("2006-01-02")}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := s.lockedUser(tx, userID, false)
		if err != nil {
			return err
		}
		settings, err := s.settings(tx)
		if err != nil {
			return err
		}
		result.UserLimit, result.GlobalLimit = settings.UserDaily, settings.GlobalDaily
		if user.DailyLimit != nil {
			result.UserLimit = *user.DailyLimit
		}
		var counters []database.DailyUsage
		if err = tx.Where("day = ? AND scope_key IN ?", usageDay(s.now()), []string{"global", "user:" + userID}).Find(&counters).Error; err != nil {
			return err
		}
		for _, counter := range counters {
			if counter.ScopeKey == "global" {
				result.GlobalUsed = counter.Counter
			} else {
				result.UserUsed = counter.Counter
			}
		}
		return nil
	})
	if err == nil && s.generator != nil {
		result.ReportAvailable, err = s.generator.Available(ctx)
		if metadata, ok := s.generator.(interface {
			GetProvider(context.Context) (ai.ProviderView, error)
		}); ok && err == nil {
			view, metadataErr := metadata.GetProvider(ctx)
			if metadataErr != nil {
				return result, metadataErr
			}
			result.Provider, result.IsExample = view.Provider, view.Provider == "mock"
		}
	}
	return result, err
}

// RecoverStale fails abandoned synchronous attempts after their bounded network
// deadline. Scores remain intact, and consumed reservations are not refunded.
// Call at startup and periodically; a concurrent completion checks state again.
func (s *Service) RecoverStale(ctx context.Context) (int, error) {
	staleAfter := max(3*time.Minute, s.cfg.AITimeout*time.Duration(s.cfg.AIRetries+1)+time.Minute)
	var candidates []database.Analysis
	if err := s.db.WithContext(ctx).Select("id", "user_id").Where("status = 'processing' AND processing_started_at < ?", s.now().UTC().Add(-staleAfter)).Limit(100).Find(&candidates).Error; err != nil {
		return 0, err
	}
	recovered := 0
	for _, candidate := range candidates {
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var user database.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", candidate.UserID).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			now := s.now().UTC()
			update := tx.Model(&database.Analysis{}).Where("id = ? AND user_id = ? AND status = 'processing' AND processing_started_at < ?",
				candidate.ID, candidate.UserID, now.Add(-staleAfter)).Updates(map[string]any{
				"status": "failed", "failure_code": "processing_interrupted", "processing_started_at": nil,
				"revision": gorm.Expr("revision + 1"), "updated_at": now})
			if update.Error != nil || update.RowsAffected == 0 {
				return update.Error
			}
			if err := tx.Create(&database.UsageEvent{ID: uuid.NewString(), UserID: &candidate.UserID, AnalysisID: &candidate.ID,
				Status: "error", ErrorCode: "processing_interrupted", CreatedAt: now}).Error; err != nil {
				return err
			}
			recovered++
			return nil
		})
		if err != nil {
			return recovered, err
		}
	}
	return recovered, nil
}
