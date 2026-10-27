package catalogimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"testing"

	cataloghttp "github.com/eurorefurb/retail/internal/catalog"
)

// Independent baseline oracle for the pinned source's actually consumed keys.
// It does not invoke the production display parser. This source has no launch
// price extensions; their precision and invalid cases have artificial SQL tests.
func checkActualStoreDisplay(t *testing.T, ctx context.Context, db *sql.DB, p cataloghttp.StoreProduct, counts map[string]int64) {
	t.Helper()
	read := func(table, id string) map[string]any {
		t.Helper()
		var raw sql.NullString
		if db.QueryRowContext(ctx, "SELECT metadata FROM "+table+" WHERE id=?", id).Scan(&raw) != nil {
			t.Fatal("actual display metadata lookup failed")
		}
		if !raw.Valid {
			return nil
		}
		d := json.NewDecoder(strings.NewReader(raw.String))
		d.UseNumber()
		var value map[string]any
		if d.Decode(&value) != nil {
			t.Fatal("actual display source decode failed")
		}
		return value
	}
	equal := func(a, b any) bool {
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		var left, right any
		return json.Unmarshal(x, &left) == nil && json.Unmarshal(y, &right) == nil && reflect.DeepEqual(left, right)
	}
	m := read("catalog_products", p.ID)
	colors := []cataloghttp.ModelColor{}
	if entries, ok := m["card_colors"].([]any); ok {
		for _, entry := range entries {
			c, ok := entry.(map[string]any)
			if !ok {
				t.Fatal("pinned source color shape changed")
			}
			name, a := c["name"].(string)
			hex, b := c["hex"].(string)
			if !a || !b {
				t.Fatal("pinned source color fields changed")
			}
			colors = append(colors, cataloghttp.ModelColor{Name: name, Hex: hex})
		}
	}
	if !equal(colors, p.Display.Colors) {
		t.Fatal("actual public colors differ from source")
	}
	counts["http_public_actual_display_colors"] += int64(len(colors))
	var expectedSource *string
	if m["source"] == "swappie-html-snapshot" {
		value := "swappie-html-snapshot"
		expectedSource = &value
	}
	if !equal(expectedSource, p.Display.Source) {
		t.Fatal("actual public source notice differs")
	}
	if expectedSource != nil {
		counts["http_public_actual_display_source_notices"]++
	}
	if m["apple_launch_prices"] != nil || len(p.Display.LaunchPrices) != 0 {
		t.Fatal("pinned product launch-price baseline changed")
	}
	specs, ok := m["apple_model_specs"].(map[string]any)
	if !ok {
		t.Fatal("pinned model specification baseline changed")
	}
	expected := map[string]any{}
	for _, key := range []string{"release_year", "display_size_inches", "display_name", "display_resolution_px", "pixel_density_ppi", "chip", "rear_camera_system", "rear_camera_count", "front_camera", "height_mm", "width_mm", "depth_mm", "weight_g", "connector", "biometric", "tech_specs_url"} {
		switch value := specs[key].(type) {
		case string:
			if value != "" {
				expected[key] = value
			}
		case json.Number:
			n, valid := new(big.Rat).SetString(string(value))
			if !valid {
				t.Fatal("actual model specification number invalid")
			}
			s := strings.TrimRight(strings.TrimRight(n.FloatString(128), "0"), ".")
			expected[key] = s
		}
	}
	if numbers, ok := specs["apple_model_numbers"].([]any); ok && len(numbers) > 0 {
		expected["apple_model_numbers"] = numbers
	}
	if !equal(expected, p.Display.Specs) {
		t.Fatal("actual public model specifications differ from consumed source fields")
	}
	counts["http_public_actual_display_spec_fields"] += int64(len(expected))
	counts["http_public_actual_display_products"]++
	validSlug := regexp.MustCompile(`^iphone-[a-z0-9-]+-[SA](85|100)$`)
	for _, v := range p.Variants {
		m := read("catalog_variants", v.ID)
		var slug, basis *string
		if s, ok := m["slug"].(string); ok && validSlug.MatchString(s) {
			slug = &s
		}
		if m["demo_price_basis"] == "model-starting-price" {
			s := "model-starting-price"
			basis = &s
		}
		if !equal(slug, v.Display.Slug) || !equal(basis, v.Display.DemoPriceBasis) {
			t.Fatal("actual public variant slug or demo notice differs")
		}
		if m["apple_launch_price"] != nil || v.Display.LaunchPrice != nil {
			t.Fatal("pinned variant launch-price baseline changed")
		}
		if slug != nil {
			counts["http_public_actual_display_variant_slugs"]++
		}
		if basis != nil {
			counts["http_public_actual_display_variant_demo_notices"]++
		}
		counts["http_public_actual_display_variants"]++
	}
}
