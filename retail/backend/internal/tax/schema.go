package tax

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

//go:embed schema_plan.json
var schemaPlan []byte
var ErrSchema = errors.New("tax schema unavailable or incompatible")

// Ready checks the declared read model, never creates tables or enables writes.
func Ready(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return ErrSchema
	}
	var expected struct {
		SchemaVersion int    `json:"schema_version"`
		ModelSHA      string `json:"model_sha256"`
	}
	if json.Unmarshal(schemaPlan, &expected) != nil || expected.SchemaVersion != 1 || len(expected.ModelSHA) != 64 {
		return ErrSchema
	}
	var actual struct {
		SchemaVersion int
		ModelSHA256   string
		ReadsEnabled  bool
		WritesEnabled bool
	}
	if db.WithContext(ctx).Table("tax_runtime_schema").Where("id=1").Take(&actual).Error != nil || actual.SchemaVersion != expected.SchemaVersion || actual.ModelSHA256 != expected.ModelSHA || !actual.ReadsEnabled || actual.WritesEnabled {
		return ErrSchema
	}
	return nil
}
