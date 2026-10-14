package database

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestJSONDatabaseAndEncoding(t *testing.T) {
	source := []byte(`{"answer":5,"unknown":null}`)
	var value JSON
	if err := value.Scan(source); err != nil {
		t.Fatal(err)
	}
	source[2] = 'x'
	if string(value) != `{"answer":5,"unknown":null}` {
		t.Fatal("Scan retained driver-owned memory")
	}
	encoded, err := json.Marshal(struct {
		Answers JSON `json:"answers"`
	}{Answers: value})
	if err != nil || string(encoded) != `{"answers":{"answer":5,"unknown":null}}` {
		t.Fatal("JSON was double encoded", string(encoded), err)
	}
	driverValue, err := value.Value()
	if err != nil || driverValue != string(value) {
		t.Fatal(driverValue, err)
	}
	if err = value.Scan(42); err == nil {
		t.Fatal("accepted unsupported driver value")
	}
	if err = value.Scan("broken"); err == nil {
		t.Fatal("accepted malformed JSON")
	}
	if _, err = JSON("broken").Value(); err == nil {
		t.Fatal("persisted malformed JSON")
	}
	if err = value.Scan(nil); err != nil || value != nil {
		t.Fatal(value, err)
	}
	encoded, err = json.Marshal(value)
	if err != nil || string(encoded) != "null" {
		t.Fatal(string(encoded), err)
	}
}

// Each integration test creates its own transaction-local schema. Even rollback
// tests cannot drop tables or fixtures in the shared test application's schema.
func isolatedModelDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL integration test")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.Contains(strings.ToLower(u.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sql, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sql.Close() })
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	schema := "test_models_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = tx.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec(`SET LOCAL search_path TO "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, tx); err != nil {
		t.Fatal(err)
	}
	return tx
}

func fixtureUser(t *testing.T, db *gorm.DB) User {
	t.Helper()
	user := User{ID: uuid.NewString(), Email: uuid.NewString() + "@example.test", Name: "Test account", Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func fixtureAnalysis(t *testing.T, db *gorm.DB, userID string) Analysis {
	t.Helper()
	version := "test-" + uuid.NewString()
	questionnaire := Questionnaire{Module: "self", Version: version, ScoringVersion: "1.0", Definition: JSON(`{"questions":[]}`)}
	if err := db.Create(&questionnaire).Error; err != nil {
		t.Fatal(err)
	}
	analysis := Analysis{
		ID: uuid.NewString(), UserID: userID, Module: "self", Locale: "en", QuestionnaireVersion: version,
		ScoringVersion: "1.0", Status: "draft", Answers: JSON(`{"q1":5,"q2":null}`), Context: JSON(`{}`), Scores: JSON(`{}`),
	}
	if err := db.Create(&analysis).Error; err != nil {
		t.Fatal(err)
	}
	return analysis
}

func rejectSQL(t *testing.T, db *gorm.DB, statement string, args ...any) {
	t.Helper()
	var rejected error
	_ = db.Transaction(func(tx *gorm.DB) error {
		rejected = tx.Exec(statement, args...).Error
		if rejected == nil {
			return errors.New("rollback accepted constraint probe")
		}
		return rejected
	})
	if rejected == nil {
		t.Errorf("database accepted invalid record: %s", statement)
	}
}

func TestDomainConstraintsAndJSONPersistence(t *testing.T) {
	db := isolatedModelDB(t)
	user := fixtureUser(t, db)
	analysis := fixtureAnalysis(t, db, user.ID)
	var loaded Analysis
	if err := db.First(&loaded, "id = ?", analysis.ID).Error; err != nil {
		t.Fatal(err)
	}
	var answers map[string]*int
	if err := json.Unmarshal(loaded.Answers, &answers); err != nil || answers["q1"] == nil || *answers["q1"] != 5 || answers["q2"] != nil {
		t.Fatal("lost structured questionnaire responses", answers, err)
	}
	var kind string
	if err := db.Raw("SELECT jsonb_typeof(answers) FROM analyses WHERE id = ?", analysis.ID).Scan(&kind).Error; err != nil || kind != "object" {
		t.Fatal("JSONB contains encoded string", kind, err)
	}
	rejectSQL(t, db, "INSERT INTO users (id,email) VALUES (?,?)", uuid.NewString(), user.Email)
	rejectSQL(t, db, "INSERT INTO users (id,email) VALUES (?,?)", uuid.NewString(), "UPPER@example.test")
	rejectSQL(t, db, "UPDATE users SET age = 17 WHERE id = ?", user.ID)
	rejectSQL(t, db, "UPDATE users SET role = 'superadmin' WHERE id = ?", user.ID)
	rejectSQL(t, db, "UPDATE users SET gender = 'invented' WHERE id = ?", user.ID)
	rejectSQL(t, db, "UPDATE users SET daily_limit = 0 WHERE id = ?", user.ID)
	rejectSQL(t, db, "UPDATE analyses SET user_id = ? WHERE id = ?", uuid.NewString(), analysis.ID)
	rejectSQL(t, db, "UPDATE analyses SET questionnaire_version = 'missing' WHERE id = ?", analysis.ID)
	rejectSQL(t, db, "UPDATE analyses SET scoring_version = 'missing' WHERE id = ?", analysis.ID)
	rejectSQL(t, db, "UPDATE analyses SET locale = 'xx' WHERE id = ?", analysis.ID)
	rejectSQL(t, db, "UPDATE analyses SET answers = '[]'::jsonb WHERE id = ?", analysis.ID)
	rejectSQL(t, db, "UPDATE analyses SET status = 'completed' WHERE id = ?", analysis.ID)
	rejectSQL(t, db, "UPDATE analyses SET status = 'processing' WHERE id = ?", analysis.ID)
	rejectSQL(t, db, "UPDATE analyses SET revision = 0 WHERE id = ?", analysis.ID)
	rejectSQL(t, db, "DELETE FROM questionnaires WHERE module = ? AND version = ?", analysis.Module, analysis.QuestionnaireVersion)
	rejectSQL(t, db, "INSERT INTO daily_usage(day,scope_key,counter) VALUES (current_date,'global',-1)")
	rejectSQL(t, db, "INSERT INTO provider_config(id,provider,model) VALUES (2,'openai','model')")
	rejectSQL(t, db, "INSERT INTO provider_config(id,provider,model,active) VALUES (1,'openai','model',true)")
	rejectSQL(t, db, "INSERT INTO provider_config(id,provider,model) VALUES (1,'arbitrary-url','model')")
}

func TestDeletionCascadeAndSanitizedMetadataRetention(t *testing.T) {
	db := isolatedModelDB(t)
	user := fixtureUser(t, db)
	analysis := fixtureAnalysis(t, db, user.ID)
	identity := AuthIdentity{ID: uuid.NewString(), UserID: user.ID, Provider: "google", Subject: uuid.NewString()}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(uuid.NewString())))
	session := RefreshSession{ID: uuid.NewString(), UserID: user.ID, FamilyID: uuid.NewString(), TokenHash: tokenHash, ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	usage := UsageEvent{ID: uuid.NewString(), UserID: &user.ID, AnalysisID: &analysis.ID, Provider: "openai", Model: "test-model", Status: "success"}
	if err := db.Create(&usage).Error; err != nil {
		t.Fatal(err)
	}
	audit := AuditEvent{ID: uuid.NewString(), ActorID: &user.ID, TargetUserID: &user.ID, Action: "account_delete", Metadata: JSON(`{"reason":"requested"}`)}
	if err := db.Create(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&User{}, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&AuthIdentity{}, &RefreshSession{}, &Analysis{}} {
		var count int64
		if err := db.Model(model).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("sensitive dependent %T retained: %d %v", model, count, err)
		}
	}
	var retainedUsage UsageEvent
	if err := db.First(&retainedUsage, "id = ?", usage.ID).Error; err != nil || retainedUsage.UserID != nil || retainedUsage.AnalysisID != nil {
		t.Fatal("usage metadata retention must remove identifiers", retainedUsage, err)
	}
	var retainedAudit AuditEvent
	if err := db.First(&retainedAudit, "id = ?", audit.ID).Error; err != nil || retainedAudit.ActorID != nil || retainedAudit.TargetUserID != nil {
		t.Fatal("audit metadata retention must remove identifiers", retainedAudit, err)
	}
}

func TestDomainRollbackAndTransactionalFailure(t *testing.T) {
	db := isolatedModelDB(t)
	user := fixtureUser(t, db)
	marker := errors.New("requested rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", user.ID).Update("name", "must be rolled back").Error; err != nil {
			return err
		}
		return marker
	}); !errors.Is(err, marker) {
		t.Fatal(err)
	}
	var unchanged User
	if err := db.First(&unchanged, "id = ?", user.ID).Error; err != nil || unchanged.Name != user.Name {
		t.Fatal("transaction leaked rolled-back update", unchanged.Name, err)
	}
	// Roll back migrations only in this isolated schema, newest first, until the
	// domain schema is removed. This remains valid as later migrations are added.
	for range 100 {
		var present bool
		if err := db.Raw("SELECT to_regclass('users') IS NOT NULL").Scan(&present).Error; err != nil {
			t.Fatal(err)
		}
		if !present {
			break
		}
		if err := RollbackLast(db.Statement.Context, db); err != nil {
			t.Fatal(err)
		}
	}
	var removed bool
	if err := db.Raw("SELECT to_regclass('users') IS NULL").Scan(&removed).Error; err != nil || !removed {
		t.Fatal("domain rollback failed", removed, err)
	}
	if err := Migrate(db.Statement.Context, db); err != nil {
		t.Fatal("fresh migration after rollback failed", err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal("rolled-back schema was not reproducible", err)
	}
}
