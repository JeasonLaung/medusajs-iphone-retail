package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

// These are reviewed storefront presentation facts, not a public metadata bag.
type ModelColor struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
}
type DisplayPrice struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency_code"`
}
type StorageLaunchPrice struct {
	Storage string `json:"storage"`
	DisplayPrice
}
type ModelSpecs struct {
	ReleaseYear      string   `json:"release_year,omitempty"`
	DisplaySize      string   `json:"display_size_inches,omitempty"`
	DisplayName      string   `json:"display_name,omitempty"`
	Resolution       string   `json:"display_resolution_px,omitempty"`
	PixelDensity     string   `json:"pixel_density_ppi,omitempty"`
	Chip             string   `json:"chip,omitempty"`
	RearCameraSystem string   `json:"rear_camera_system,omitempty"`
	RearCameraCount  string   `json:"rear_camera_count,omitempty"`
	FrontCamera      string   `json:"front_camera,omitempty"`
	Height           string   `json:"height_mm,omitempty"`
	Width            string   `json:"width_mm,omitempty"`
	Depth            string   `json:"depth_mm,omitempty"`
	Weight           string   `json:"weight_g,omitempty"`
	Connector        string   `json:"connector,omitempty"`
	Biometric        string   `json:"biometric,omitempty"`
	ModelNumbers     []string `json:"apple_model_numbers,omitempty"`
	TechSpecsURL     string   `json:"tech_specs_url,omitempty"`
}
type ProductDisplay struct {
	Colors       []ModelColor         `json:"colors"`
	Specs        *ModelSpecs          `json:"model_specs"`
	LaunchPrices []StorageLaunchPrice `json:"launch_prices"`
	Source       *string              `json:"source"`
}
type VariantDisplay struct {
	Slug           *string       `json:"slug"`
	DemoPriceBasis *string       `json:"demo_price_basis"`
	LaunchPrice    *DisplayPrice `json:"apple_launch_price"`
}

var errDisplay = errors.New("product display unavailable")
var displayHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var displaySlug = regexp.MustCompile(`^iphone-[a-z0-9-]+-[SA](85|100)$`)
var displayCurrency = regexp.MustCompile(`^[a-z]{3}$`)

// Bound nested unknown values as well. Duplicate JSON object keys fail closed;
// unknown keys are never copied, and malformed optional values are omitted in
// the same places the original storefront's type guards omitted them.
func displayJSON(raw *string) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	if len(*raw) > 65536 || !utf8.ValidString(*raw) {
		return nil, errDisplay
	}
	d := json.NewDecoder(bytes.NewBufferString(*raw))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 32 {
			return nil, errDisplay
		}
		token, e := d.Token()
		if e != nil {
			return nil, errDisplay
		}
		switch token {
		case json.Delim('{'):
			out := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, errDisplay
				}
				key, ok := k.(string)
				if !ok {
					return nil, errDisplay
				}
				if _, ok = out[key]; ok {
					return nil, errDisplay
				}
				v, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				out[key] = v
			}
			if end, e := d.Token(); e != nil || end != json.Delim('}') {
				return nil, errDisplay
			}
			return out, nil
		case json.Delim('['):
			out := []any{}
			for d.More() {
				v, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				out = append(out, v)
			}
			if end, e := d.Token(); e != nil || end != json.Delim(']') {
				return nil, errDisplay
			}
			return out, nil
		default:
			return token, nil
		}
	}
	v, e := read(0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errDisplay
	}
	if v == nil {
		return nil, nil
	}
	out, ok := v.(map[string]any)
	if !ok {
		return nil, errDisplay
	}
	return out, nil
}
func displayText(v any) string {
	s, ok := v.(string)
	if !ok || len(s) > 4096 || !utf8.ValidString(s) {
		return ""
	}
	for _, r := range s {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' || r == 127 {
			return ""
		}
	}
	return s
}
func displayScalar(v any) string {
	if n, ok := v.(json.Number); ok {
		s, e := totals.Resolve(totals.Number{Decimal: string(n)})
		if e == nil {
			return s
		}
		return ""
	}
	return displayText(v)
}
func launchPrice(v any) *DisplayPrice {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	n, ok := m["amount"].(json.Number)
	if !ok {
		return nil
	}
	s, e := totals.Resolve(totals.Number{Decimal: string(n)})
	currency, ok := m["currency_code"].(string)
	if e != nil || s == "0" || strings.HasPrefix(s, "-") || !ok || !displayCurrency.MatchString(currency) {
		return nil
	}
	return &DisplayPrice{s, currency}
}
func productDisplay(raw *string) (ProductDisplay, error) {
	out := ProductDisplay{Colors: []ModelColor{}, LaunchPrices: []StorageLaunchPrice{}}
	m, e := displayJSON(raw)
	if e != nil {
		return out, e
	}
	if m["source"] == "swappie-html-snapshot" {
		s := "swappie-html-snapshot"
		out.Source = &s
	}
	if colors, ok := m["card_colors"].([]any); ok {
		if len(colors) > 64 {
			return out, errDisplay
		}
		for _, v := range colors {
			c, ok := v.(map[string]any)
			if !ok {
				continue
			}
			name, hex := displayText(c["name"]), displayText(c["hex"])
			if name != "" && utf8.RuneCountInString(name) <= 191 && displayHex.MatchString(hex) {
				out.Colors = append(out.Colors, ModelColor{name, hex})
			}
		}
	}
	specs, nested := m["apple_model_specs"].(map[string]any)
	if !nested {
		specs = m
	}
	s := &ModelSpecs{}
	for key, field := range map[string]*string{"release_year": &s.ReleaseYear, "display_size_inches": &s.DisplaySize, "display_name": &s.DisplayName, "display_resolution_px": &s.Resolution, "pixel_density_ppi": &s.PixelDensity, "chip": &s.Chip, "rear_camera_system": &s.RearCameraSystem, "rear_camera_count": &s.RearCameraCount, "front_camera": &s.FrontCamera, "height_mm": &s.Height, "width_mm": &s.Width, "depth_mm": &s.Depth, "weight_g": &s.Weight, "connector": &s.Connector, "biometric": &s.Biometric} {
		*field = displayScalar(specs[key])
	}
	if numbers, ok := specs["apple_model_numbers"].([]any); ok {
		if len(numbers) > 64 {
			return out, errDisplay
		}
		for _, n := range numbers {
			if value := displayText(n); value != "" {
				s.ModelNumbers = append(s.ModelNumbers, value)
			}
		}
	}
	if value := displayText(specs["tech_specs_url"]); value != "" {
		u, e := url.Parse(value)
		if e == nil && u.Scheme == "https" && (u.Host == "apple.com" || u.Host == "support.apple.com") && u.User == nil && strings.HasPrefix(u.Path, "/") {
			s.TechSpecsURL = value
		}
	}
	b, _ := json.Marshal(s)
	if nested || string(b) != "{}" {
		out.Specs = s
	}
	if prices, ok := m["apple_launch_prices"].(map[string]any); ok {
		if len(prices) > 128 {
			return out, errDisplay
		}
		keys := []string{}
		for storage := range prices {
			keys = append(keys, storage)
		}
		sort.Strings(keys)
		for _, storage := range keys {
			if validProductID(storage) {
				if p := launchPrice(prices[storage]); p != nil {
					out.LaunchPrices = append(out.LaunchPrices, StorageLaunchPrice{storage, *p})
				}
			}
		}
	}
	return out, nil
}
func variantDisplay(raw *string) (VariantDisplay, error) {
	out := VariantDisplay{}
	m, e := displayJSON(raw)
	if e != nil {
		return out, e
	}
	if slug := displayText(m["slug"]); validProductID(slug) && displaySlug.MatchString(slug) {
		out.Slug = &slug
	}
	if m["demo_price_basis"] == "model-starting-price" {
		s := "model-starting-price"
		out.DemoPriceBasis = &s
	}
	out.LaunchPrice = launchPrice(m["apple_launch_price"])
	return out, nil
}

// All sizes are checked before fetching private JSON. Only the already-visible
// products/variants participate, inside the caller's repeatable read snapshot.
func loadStoreDisplay(tx *gorm.DB, products []StoreProduct) error {
	productIDs, variantIDs := []string{}, []string{}
	for _, p := range products {
		productIDs = append(productIDs, p.ID)
		for _, v := range p.Variants {
			variantIDs = append(variantIDs, v.ID)
		}
	}
	budget := int64(4 * 1024 * 1024)
	for _, scope := range []struct {
		table string
		ids   []string
	}{{"catalog_products", productIDs}, {"catalog_variants", variantIDs}} {
		if len(scope.ids) == 0 {
			continue
		}
		var size struct{ Total, Largest int64 }
		if e := tx.Table(scope.table).Select("COALESCE(SUM(OCTET_LENGTH(metadata)),0) AS total,COALESCE(MAX(OCTET_LENGTH(metadata)),0) AS largest").Where("id IN ? AND deleted_at IS NULL", scope.ids).Scan(&size).Error; e != nil {
			return e
		}
		budget -= size.Total
		if size.Largest > 65536 || budget < 0 {
			return errDisplay
		}
	}
	productRaw, variantRaw := map[string]*string{}, map[string]*string{}
	for _, scope := range []struct {
		table string
		ids   []string
		out   map[string]*string
	}{{"catalog_products", productIDs, productRaw}, {"catalog_variants", variantIDs, variantRaw}} {
		if len(scope.ids) == 0 {
			continue
		}
		var rows []struct {
			ID       string
			Metadata *string
		}
		if e := tx.Table(scope.table).Select("id,metadata").Where("id IN ? AND deleted_at IS NULL", scope.ids).Scan(&rows).Error; e != nil {
			return e
		}
		if len(rows) != len(scope.ids) {
			return errDisplay
		}
		for _, r := range rows {
			scope.out[r.ID] = r.Metadata
		}
	}
	for i := range products {
		p := &products[i]
		var e error
		p.Display, e = productDisplay(productRaw[p.ID])
		if e != nil {
			return e
		}
		for j := range p.Variants {
			v := &p.Variants[j]
			v.Display, e = variantDisplay(variantRaw[v.ID])
			if e != nil {
				return e
			}
		}
	}
	return nil
}
