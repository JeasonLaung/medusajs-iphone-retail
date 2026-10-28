package fulfillment

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

//go:embed geo_schema_plan.json
var geoSchemaPlan []byte

var ErrGeoSchema = errors.New("fulfillment geo schema unavailable")

// Ready verifies the compiled evidence schema. It grants neither shipping
// or inventory authority; no fulfillment command or worker is enabled.
func GeoReady(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return ErrGeoSchema
	}
	var expected struct {
		SchemaVersion int    `json:"schema_version"`
		ModelSHA      string `json:"model_sha256"`
	}
	if json.Unmarshal(geoSchemaPlan, &expected) != nil || expected.SchemaVersion != 1 || len(expected.ModelSHA) != 64 {
		return ErrGeoSchema
	}
	var actual struct {
		SchemaVersion               int
		ModelSHA256                 string
		ReadsEnabled, WritesEnabled bool
	}
	if db.WithContext(ctx).Table("geo_runtime_schema").Where("id=1").Take(&actual).Error != nil || actual.SchemaVersion != expected.SchemaVersion || actual.ModelSHA256 != expected.ModelSHA || !actual.ReadsEnabled || actual.WritesEnabled {
		return ErrGeoSchema
	}
	return nil
}
