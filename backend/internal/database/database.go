package database

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"relations/migrations"
)

func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), TranslateError: true, DisableAutomaticPing: true})
	if e != nil {
		return nil, fmt.Errorf("database connection failed")
	}
	sql, e := db.DB()
	if e != nil {
		return nil, e
	}
	sql.SetMaxOpenConns(20)
	sql.SetMaxIdleConns(5)
	sql.SetConnMaxLifetime(30 * time.Minute)
	if e = sql.PingContext(ctx); e != nil {
		_ = sql.Close()
		return nil, fmt.Errorf("database ping failed")
	}
	return db, nil
}

func Migrate(ctx context.Context, db *gorm.DB) error {
	names, e := fs.Glob(migrations.Files, "*.up.sql")
	if e != nil {
		return e
	}
	sort.Strings(names)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(78290531)").Error; e != nil {
			return e
		}
		if e := tx.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())").Error; e != nil {
			return e
		}
		for _, n := range names {
			data, e := migrations.Files.ReadFile(n)
			if e != nil {
				return e
			}
			sum := fmt.Sprintf("%x", sha256.Sum256(data))
			var rows []struct{ Checksum string }
			if e = tx.Raw("SELECT checksum FROM schema_migrations WHERE version = ?", n).Scan(&rows).Error; e != nil {
				return e
			}
			if len(rows) > 0 {
				if rows[0].Checksum != sum {
					return fmt.Errorf("migration checksum changed: %s", n)
				}
				continue
			}
			if e = tx.Exec(string(data)).Error; e != nil {
				return fmt.Errorf("migration %s failed: %w", n, e)
			}
			if e = tx.Exec("INSERT INTO schema_migrations (version,checksum) VALUES (?,?)", n, sum).Error; e != nil {
				return e
			}
		}
		return nil
	})
}

// RollbackLast is explicit and intended for disposable development databases. Production uses forward corrective migrations.
func RollbackLast(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(78290531)").Error; e != nil {
			return e
		}
		var rows []struct{ Version string }
		if e := tx.Raw("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&rows).Error; e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		name := strings.TrimSuffix(rows[0].Version, ".up.sql") + ".down.sql"
		data, e := migrations.Files.ReadFile(name)
		if e != nil {
			return e
		}
		if e = tx.Exec(string(data)).Error; e != nil {
			return e
		}
		return tx.Exec("DELETE FROM schema_migrations WHERE version = ?", rows[0].Version).Error
	})
}
