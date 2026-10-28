package cart

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

//go:embed schema_plan.json
var schemaPlan []byte

var ErrSchema = errors.New("cart schema unavailable")

// Ready verifies the compiled evidence schema. It grants neither customer
// ownership nor write authority; no cart possession, mutation or HTTP route is enabled.
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
		SchemaVersion               int
		ModelSHA256                 string
		ReadsEnabled, WritesEnabled bool
	}
	if db.WithContext(ctx).Table("cart_runtime_schema").Where("id=1").Take(&actual).Error != nil || actual.SchemaVersion != expected.SchemaVersion || actual.ModelSHA256 != expected.ModelSHA || !actual.ReadsEnabled || actual.WritesEnabled {
		return ErrSchema
	}
	return nil
}
