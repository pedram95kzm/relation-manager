package questionnaire

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"relations/internal/database"
)

// Seed adds new immutable definitions atomically. Existing rows must represent
// the exact same canonical JSON; modifying an applied version fails startup.
func Seed(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, module := range []string{"self", "compatibility", "conflict"} {
			q, err := Current(module)
			if err != nil {
				return err
			}
			data, err := definitionJSON(q)
			if err != nil {
				return err
			}
			row := database.Questionnaire{Module: module, Version: q.Version, ScoringVersion: q.ScoringVersion, Definition: database.JSON(data)}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return fmt.Errorf("seed questionnaire: %w", err)
			}
			var stored database.Questionnaire
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("module = ? AND version = ?", module, q.Version).First(&stored).Error; err != nil {
				return fmt.Errorf("read seeded questionnaire: %w", err)
			}
			expected, err := canonicalDigest(data)
			if err != nil {
				return err
			}
			actual, err := canonicalDigest(stored.Definition)
			if err != nil || stored.ScoringVersion != q.ScoringVersion || actual != expected {
				return fmt.Errorf("immutable questionnaire version drift for %s/%s; publish a new version", module, q.Version)
			}
		}
		return nil
	})
}

func canonicalDigest(data []byte) ([32]byte, error) {
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return [32]byte{}, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(canonical), nil
}
