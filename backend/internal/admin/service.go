package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"relations/internal/ai"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
)

type Service struct {
	cfg      config.Config
	db       *gorm.DB
	provider *ai.Service
}

func New(cfg config.Config, db *gorm.DB, provider *ai.Service) *Service {
	return &Service{cfg, db, provider}
}
func (s *Service) Provider() *ai.Service { return s.provider }

type User struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	Role       string    `json:"role"`
	Disabled   bool      `json:"disabled"`
	DailyLimit *int      `json:"daily_limit"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func userView(u database.User) User {
	return User{u.ID, u.Email, u.Name, u.Role, u.Disabled, u.DailyLimit, u.CreatedAt, u.UpdatedAt}
}

type UserInput struct {
	Disabled   *bool           `json:"disabled"`
	DailyLimit json.RawMessage `json:"daily_limit"`
}

func (input *UserInput) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("user update must be an object")
	}
	type plain UserInput
	var value plain
	if err := platform.StrictJSON(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["disabled"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("disabled cannot be null")
	}
	*input = UserInput(value)
	return nil
}

func (s *Service) Users(ctx context.Context, query string, page, size int) (platform.Page[User], error) {
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) > 120 {
		return platform.Page[User]{}, platform.Err(400, "invalid_search", "Search must contain at most 120 characters.")
	}
	query = strings.TrimSpace(query)
	q := s.db.WithContext(ctx).Model(&database.User{})
	if query != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
		q = q.Where("email ILIKE ? ESCAPE '\\' OR name ILIKE ? ESCAPE '\\'", "%"+escaped+"%", "%"+escaped+"%")
	}
	result := platform.Page[User]{Items: []User{}, Page: page, PageSize: size}
	if err := q.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var users []database.User
	if err := q.Select("id,email,name,role,disabled,daily_limit,created_at,updated_at").Order("created_at DESC,id").Offset((page - 1) * size).Limit(size).Find(&users).Error; err != nil {
		return result, err
	}
	for _, u := range users {
		result.Items = append(result.Items, userView(u))
	}
	return result, nil
}

func (s *Service) UpdateUser(ctx context.Context, actor, id string, input UserInput) (User, error) {
	if _, err := uuid.Parse(id); err != nil {
		return User{}, platform.Err(404, "not_found", "User not found.")
	}
	if input.Disabled == nil && len(input.DailyLimit) == 0 {
		return User{}, platform.Err(400, "invalid_user_update", "Provide a disabled state or daily limit.")
	}
	var dailyLimit *int
	if len(input.DailyLimit) > 0 && string(input.DailyLimit) != "null" {
		var value int
		if json.Unmarshal(input.DailyLimit, &value) != nil || value < 1 || value > 10000 {
			return User{}, platform.Err(400, "invalid_limit", "Daily limit must be between 1 and 10000, or null to use the default.")
		}
		dailyLimit = &value
	}
	var user database.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return platform.Err(404, "not_found", "User not found.")
			}
			return err
		}
		if user.ID == actor || user.Role == "admin" {
			return platform.Err(403, "protected_administrator", "Administrator accounts cannot be changed through user management.")
		}
		updates := map[string]any{"updated_at": time.Now().UTC()}
		metadata := map[string]any{}
		if input.Disabled != nil {
			updates["disabled"] = *input.Disabled
			metadata["disabled"] = *input.Disabled
			if *input.Disabled != user.Disabled {
				updates["auth_version"] = gorm.Expr("auth_version + 1")
				if err := tx.Model(&database.RefreshSession{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", time.Now().UTC()).Error; err != nil {
					return err
				}
			}
			user.Disabled = *input.Disabled
		}
		if len(input.DailyLimit) > 0 {
			updates["daily_limit"] = dailyLimit
			user.DailyLimit = dailyLimit
			metadata["daily_limit"] = dailyLimit
		}
		if err := tx.Model(&database.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		data, _ := json.Marshal(metadata)
		if err := tx.Create(&database.AuditEvent{ID: uuid.NewString(), ActorID: &actor, TargetUserID: &id, Action: "user.updated", Metadata: data}).Error; err != nil {
			return err
		}
		return tx.First(&user, "id = ?", id).Error
	})
	return userView(user), err
}

type DeleteUserInput struct {
	CurrentPassword string `json:"current_password"`
	ConfirmEmail    string `json:"confirm_email"`
}

// Erasure is available for ordinary accounts, including Google-only users. Both
// explicit target confirmation and administrator reauthentication are required.
func (s *Service) DeleteUser(ctx context.Context, actor, id string, input DeleteUserInput) error {
	if _, err := uuid.Parse(id); err != nil {
		return platform.Err(404, "not_found", "User not found.")
	}
	if actor == id {
		return platform.Err(403, "protected_administrator", "Administrator accounts cannot be erased through user management.")
	}
	var administrator database.User
	if err := s.db.WithContext(ctx).First(&administrator, "id = ?", actor).Error; err != nil {
		return platform.Err(401, "reauthentication_required", "Verify your current administrator password.")
	}
	if administrator.Role != "admin" || administrator.Disabled || administrator.PasswordHash == "" || len(input.CurrentPassword) > 72 || bcrypt.CompareHashAndPassword([]byte(administrator.PasswordHash), []byte(input.CurrentPassword)) != nil {
		return platform.Err(401, "reauthentication_required", "Verify your current administrator password.")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", actor).Error; err != nil {
			return err
		}
		if current.Role != "admin" || current.Disabled || current.PasswordHash != administrator.PasswordHash {
			return platform.Err(401, "reauthentication_required", "Verify your current administrator password.")
		}
		var target database.User
		// An unlocked role check prevents opposite admin-target locks; roles are
		// immutable through every account-management API in this application.
		if err := tx.First(&target, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return platform.Err(404, "not_found", "User not found.")
			}
			return err
		}
		if target.Role == "admin" {
			return platform.Err(403, "protected_administrator", "Administrator accounts cannot be erased through user management.")
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&target, "id = ?", id).Error; err != nil {
			return err
		}
		if target.Role == "admin" {
			return platform.Err(403, "protected_administrator", "Administrator accounts cannot be erased through user management.")
		}
		if input.ConfirmEmail != target.Email {
			return platform.Err(400, "confirmation_required", "Confirm the target account's exact email address.")
		}
		if err := tx.Where("scope_key = ?", "user:"+id).Delete(&database.DailyUsage{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&target).Error; err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"account_erased": true, "deleted_account_id": id})
		return tx.Create(&database.AuditEvent{ID: uuid.NewString(), ActorID: &actor, Action: "user.deleted", Metadata: metadata}).Error
	})
}

type Limits struct {
	GlobalDaily int `json:"global_daily"`
	UserDaily   int `json:"user_daily"`
}

func (s *Service) GetLimits(ctx context.Context) (Limits, error) {
	settings := database.AppSettings{ID: 1, GlobalDaily: s.cfg.GlobalDailyLimit, UserDaily: s.cfg.UserDailyLimit}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error; err != nil {
		return Limits{}, err
	}
	if err := s.db.WithContext(ctx).First(&settings, "id = 1").Error; err != nil {
		return Limits{}, err
	}
	return Limits{settings.GlobalDaily, settings.UserDaily}, nil
}
func (s *Service) SetLimits(ctx context.Context, actor string, input Limits) (Limits, error) {
	if input.GlobalDaily < 1 || input.GlobalDaily > 1000000 || input.UserDaily < 1 || input.UserDaily > 10000 {
		return Limits{}, platform.Err(400, "invalid_limit", "Global limit must be 1–1000000 and default user limit 1–10000.")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settings := database.AppSettings{ID: 1, GlobalDaily: input.GlobalDaily, UserDaily: input.UserDaily}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"global_daily", "user_daily", "updated_at"})}).Create(&settings).Error; err != nil {
			return err
		}
		metadata, _ := json.Marshal(input)
		return tx.Create(&database.AuditEvent{ID: uuid.NewString(), ActorID: &actor, Action: "limits.updated", Metadata: metadata}).Error
	})
	return input, err
}
func (s *Service) Audit(ctx context.Context, page, size int) (platform.Page[database.AuditEvent], error) {
	result := platform.Page[database.AuditEvent]{Items: []database.AuditEvent{}, Page: page, PageSize: size}
	q := s.db.WithContext(ctx).Model(&database.AuditEvent{})
	if err := q.Count(&result.Total).Error; err != nil {
		return result, err
	}
	err := q.Order("created_at DESC,id").Offset((page - 1) * size).Limit(size).Find(&result.Items).Error
	return result, err
}

type Daily struct {
	Day         string `json:"day"`
	Generations int64  `json:"generations"`
	Successes   int64  `json:"successes"`
	Failures    int64  `json:"failures"`
}
type ModuleCount struct {
	Module string `json:"module"`
	Count  int64  `json:"count"`
}
type ProviderCount struct {
	Provider string `json:"provider"`
	Count    int64  `json:"count"`
}
type Analytics struct {
	Users                 int64           `json:"users"`
	ActiveUsers           int64           `json:"active_users"`
	DisabledUsers         int64           `json:"disabled_users"`
	Analyses              int64           `json:"analyses"`
	Generations           int64           `json:"generations"`
	SuccessfulGenerations int64           `json:"successful_generations"`
	FailedGenerations     int64           `json:"failed_generations"`
	InputTokens           int64           `json:"input_tokens"`
	OutputTokens          int64           `json:"output_tokens"`
	TodayGlobalUsage      int64           `json:"today_global_usage"`
	Days                  int             `json:"days"`
	Daily                 []Daily         `json:"daily"`
	Modules               []ModuleCount   `json:"modules"`
	Providers             []ProviderCount `json:"providers"`
}

func (s *Service) Analytics(ctx context.Context, days int) (Analytics, error) {
	if days < 1 || days > 90 {
		return Analytics{}, platform.Err(400, "invalid_range", "Analytics days must be between 1 and 90.")
	}
	result := Analytics{Days: days, Daily: []Daily{}, Modules: []ModuleCount{}, Providers: []ProviderCount{}}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	since := today.AddDate(0, 0, -days+1)
	db := s.db.WithContext(ctx)
	var users struct{ Users, ActiveUsers, DisabledUsers int64 }
	if err := db.Raw("SELECT COUNT(*) AS users, COUNT(*) FILTER (WHERE NOT disabled) AS active_users, COUNT(*) FILTER (WHERE disabled) AS disabled_users FROM users").Scan(&users).Error; err != nil {
		return result, err
	}
	result.Users = users.Users
	result.ActiveUsers = users.ActiveUsers
	result.DisabledUsers = users.DisabledUsers
	if err := db.Model(&database.Analysis{}).Count(&result.Analyses).Error; err != nil {
		return result, err
	}
	var usage struct{ Generations, SuccessfulGenerations, FailedGenerations, InputTokens, OutputTokens int64 }
	if err := db.Raw("SELECT COUNT(*) AS generations,COUNT(*) FILTER (WHERE status = 'success') AS successful_generations,COUNT(*) FILTER (WHERE status <> 'success') AS failed_generations,COALESCE(SUM(input_tokens),0) AS input_tokens,COALESCE(SUM(output_tokens),0) AS output_tokens FROM usage_events WHERE created_at >= ?", since).Scan(&usage).Error; err != nil {
		return result, err
	}
	result.Generations = usage.Generations
	result.SuccessfulGenerations = usage.SuccessfulGenerations
	result.FailedGenerations = usage.FailedGenerations
	result.InputTokens = usage.InputTokens
	result.OutputTokens = usage.OutputTokens
	if err := db.Raw("SELECT COALESCE((SELECT counter FROM daily_usage WHERE day = ? AND scope_key = 'global'),0)", today.Format("2006-01-02")).Scan(&result.TodayGlobalUsage).Error; err != nil {
		return result, err
	}
	var daily []Daily
	if err := db.Raw("SELECT to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD') AS day,COUNT(*) AS generations,COUNT(*) FILTER (WHERE status = 'success') AS successes,COUNT(*) FILTER (WHERE status <> 'success') AS failures FROM usage_events WHERE created_at >= ? GROUP BY day ORDER BY day", since).Scan(&daily).Error; err != nil {
		return result, err
	}
	byDay := map[string]Daily{}
	for _, d := range daily {
		byDay[d.Day] = d
	}
	for i := 0; i < days; i++ {
		day := since.AddDate(0, 0, i).Format("2006-01-02")
		d, ok := byDay[day]
		if !ok {
			d.Day = day
		}
		result.Daily = append(result.Daily, d)
	}
	if err := db.Model(&database.Analysis{}).Select("module,COUNT(*) AS count").Where("created_at >= ?", since).Group("module").Order("module").Scan(&result.Modules).Error; err != nil {
		return result, err
	}
	if err := db.Model(&database.UsageEvent{}).Select("provider,COUNT(*) AS count").Where("created_at >= ?", since).Group("provider").Order("provider").Scan(&result.Providers).Error; err != nil {
		return result, err
	}
	return result, nil
}
