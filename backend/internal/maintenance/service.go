package maintenance

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const BatchSize = 500
const Timeout = 5 * time.Second

type Service struct {
	db    *gorm.DB
	now   func() time.Time
	batch int
}

func New(db *gorm.DB) *Service { return &Service{db: db, now: time.Now, batch: BatchSize} }

type Stats struct {
	RefreshSessions int64 `json:"refresh_sessions"`
	OAuthStates     int64 `json:"oauth_states"`
}

// Cleanup removes bounded batches of expired authentication data only. Used
// refresh ancestors remain while their family is live so replay can still revoke
// that family. All account writers lock users before sessions; cleanup uses the
// same order and rechecks family activity after those user locks are acquired.
func (s *Service) Cleanup(ctx context.Context) (Stats, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	now := s.now().UTC()
	result := Stats{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var users []string
		err := tx.Raw(`SELECT u.id FROM users u
			WHERE EXISTS (
				SELECT 1 FROM refresh_sessions expired WHERE expired.user_id = u.id AND expired.expires_at <= ?
				AND NOT EXISTS (SELECT 1 FROM refresh_sessions live WHERE live.family_id = expired.family_id
					AND live.revoked_at IS NULL AND live.expires_at > ?))
			ORDER BY u.id LIMIT ? FOR UPDATE OF u SKIP LOCKED`, now, now, s.batch).Scan(&users).Error
		if err != nil {
			return err
		}
		if len(users) > 0 {
			// This separate statement observes child sessions committed before the
			// user locks, and no rotation can now add another child for these users.
			deleted := tx.Exec(`WITH expired AS MATERIALIZED (
				SELECT old.id FROM refresh_sessions old WHERE old.user_id IN ? AND old.expires_at <= ?
				AND NOT EXISTS (SELECT 1 FROM refresh_sessions live WHERE live.family_id = old.family_id
					AND live.revoked_at IS NULL AND live.expires_at > ?)
				ORDER BY old.expires_at,old.id LIMIT ? FOR UPDATE OF old SKIP LOCKED)
				DELETE FROM refresh_sessions doomed USING expired WHERE doomed.id = expired.id`, users, now, now, s.batch)
			if deleted.Error != nil {
				return deleted.Error
			}
			result.RefreshSessions = deleted.RowsAffected
		}
		deleted := tx.Exec(`WITH expired AS MATERIALIZED (
			SELECT state_hash FROM oauth_states WHERE expires_at <= ? ORDER BY expires_at,state_hash
			LIMIT ? FOR UPDATE SKIP LOCKED)
			DELETE FROM oauth_states doomed USING expired WHERE doomed.state_hash = expired.state_hash`, now, s.batch)
		if deleted.Error != nil {
			return deleted.Error
		}
		result.OAuthStates = deleted.RowsAffected
		return nil
	})
	if err != nil {
		return Stats{}, err
	}
	return result, nil
}
