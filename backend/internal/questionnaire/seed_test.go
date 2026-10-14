package questionnaire

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"relations/internal/database"
)

func TestPostgreSQLSeedImmutability(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL integration test")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(strings.ToLower(u.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	for range 2 {
		if err := Seed(ctx, tx); err != nil {
			t.Fatal("seed must accept JSONB normalization and be idempotent", err)
		}
	}
	var rows []database.Questionnaire
	if err := tx.Find(&rows).Error; err != nil || len(rows) != 3 {
		t.Fatal("catalog persisted", len(rows), err)
	}
	if err := tx.Model(&database.Questionnaire{}).Where("module = ? AND version = ?", "self", Version).
		Update("definition", gorm.Expr(`jsonb_set(definition, '{questions,0,reverse}', 'true'::jsonb)`)).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(ctx, tx); err == nil || !strings.Contains(err.Error(), "version drift") {
		t.Fatal("changed applied definition was overwritten or accepted", err)
	}
	var changed database.Questionnaire
	if err := tx.Where("module = ? AND version = ?", "self", Version).First(&changed).Error; err != nil || !strings.Contains(string(changed.Definition), `"reverse": true`) {
		t.Fatal("drift rejection must not overwrite stored definition", err)
	}
}
