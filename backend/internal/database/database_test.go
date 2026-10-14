package database

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMigrations(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL integration test")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.Contains(strings.ToLower(u.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, e := Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if e = Migrate(ctx, db); e != nil {
		t.Fatal(e)
	}
	if e = Migrate(ctx, db); e != nil {
		t.Fatal("idempotency", e)
	}
	var count int64
	if e = db.Raw("SELECT count(*) FROM schema_migrations").Scan(&count).Error; e != nil || count < 1 {
		t.Fatal(count, e)
	}
	tx := db.Begin()
	if e = tx.Exec("INSERT INTO app_settings(id,global_daily,user_daily) VALUES (2,10,10)").Error; e == nil {
		t.Error("singleton constraint not enforced")
	}
	tx.Rollback()
}
