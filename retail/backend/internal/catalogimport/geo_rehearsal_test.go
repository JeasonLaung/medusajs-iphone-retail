package catalogimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/rowimport"
)

// Explicit SQL/source comparisons independent of rowimport.Convert and the
// importer's compiled plan. Never print source/target identifiers or values.
func checkActualGeoRows(t *testing.T, ctx context.Context, db *sql.DB, source []rowimport.SourceRow, counts map[string]int64) {
	t.Helper()
	if len(source) != 7 {
		t.Fatal("pinned geo row count differs")
	}
	columns := []string{"id", "type", "country_code", "province_code", "city", "service_zone_id", "postal_expression", "metadata", "created_at", "updated_at", "deleted_at"}
	for _, row := range source {
		var id string
		if json.Unmarshal(row["id"].Raw, &id) != nil || id == "" {
			t.Fatal("geo source identity unavailable")
		}
		values := make([]sql.NullString, 11)
		args := make([]any, 11)
		for i := range values {
			args[i] = &values[i]
		}
		if db.QueryRowContext(ctx, `SELECT id,type,country_code,province_code,city,service_zone_id,postal_expression,metadata,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s.%f'),DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s.%f'),DATE_FORMAT(deleted_at,'%Y-%m-%d %H:%i:%s.%f') FROM fulfillment_geo_zones WHERE id=?`, id).Scan(args...) != nil {
			t.Fatal("geo target evidence unavailable")
		}
		for i, column := range columns {
			cell, ok := row[column]
			if !ok || cell.SQLNull == values[i].Valid {
				t.Fatal("geo field/null coverage differs")
			}
			if cell.SQLNull {
				continue
			}
			var expected string
			if column == "postal_expression" || column == "metadata" {
				expected = string(cell.Raw)
			} else {
				if json.Unmarshal(cell.Raw, &expected) != nil {
					t.Fatal("geo source text invalid")
				}
				if i >= 8 {
					at, err := time.Parse(time.RFC3339Nano, expected)
					if err != nil {
						t.Fatal("geo source time invalid")
					}
					expected = at.UTC().Format("2006-01-02 15:04:05.000000")
				}
			}
			if values[i].String != expected {
				t.Fatal("geo source/target field differs")
			}
			counts["geo_independent_nonnull_fields"]++
		}
	}
	counts["geo_independent_rows"] = int64(len(source))
	counts["geo_independent_fields"] = int64(len(source) * len(columns))
}
