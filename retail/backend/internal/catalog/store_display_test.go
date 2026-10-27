package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStoreDisplayProjection(t *testing.T) {
	raw := `{"source":"swappie-html-snapshot","source_url":"private","private":{"nested":"secret"},"card_colors":[{"name":"Blue","hex":"#AABBcc","private":"secret"},{"name":"bad","hex":"url(secret)"},null],"release_year":1999,"apple_model_specs":{"release_year":2021,"display_size_inches":6.1,"chip":"A15","apple_model_numbers":["A0123",{"private":"secret"}],"tech_specs_url":"https://support.apple.com/en-us/111872","private":"secret"},"apple_launch_prices":{"256 GB":{"amount":979,"currency_code":"eur","private":"secret"},"128 GB":{"amount":9007199254740993.000000000000000001,"currency_code":"eur"},"bad":{"amount":"869","currency_code":"eur"}}}`
	p, e := productDisplay(&raw)
	if e != nil || p.Source == nil || p.Specs == nil || p.Specs.ReleaseYear != "2021" || p.Specs.DisplaySize != "6.1" || len(p.Colors) != 1 || p.Colors[0].Name != "Blue" || len(p.Specs.ModelNumbers) != 1 || p.Specs.ModelNumbers[0] != "A0123" || len(p.LaunchPrices) != 2 || p.LaunchPrices[0].Storage != "128 GB" || p.LaunchPrices[0].Amount != "9007199254740993.000000000000000001" {
		t.Fatal("reviewed product display fields differ")
	}
	b, _ := json.Marshal(p)
	for _, private := range []string{"secret", "private", "source_url", "1999"} {
		if strings.Contains(string(b), private) {
			t.Fatal("unreviewed product display field escaped")
		}
	}
	vraw := `{"slug":"iphone-13-128gb-blue-S85","demo_price_basis":"model-starting-price","apple_launch_price":{"amount":869.125000000000000001,"currency_code":"eur"},"demo_stock_seed":"secret","private":"secret"}`
	v, e := variantDisplay(&vraw)
	if e != nil || v.Slug == nil || *v.Slug != "iphone-13-128gb-blue-S85" || v.DemoPriceBasis == nil || v.LaunchPrice == nil || v.LaunchPrice.Amount != "869.125000000000000001" {
		t.Fatal("reviewed variant display fields differ")
	}
	b, _ = json.Marshal(v)
	if strings.Contains(string(b), "secret") {
		t.Fatal("private variant source escaped")
	}
}

func TestStoreDisplayOptionalGuardsAndFallback(t *testing.T) {
	for _, raw := range []*string{nil, ptr("null"), ptr("{}"), ptr(`{"private":"secret"}`)} {
		p, e := productDisplay(raw)
		v, f := variantDisplay(raw)
		if e != nil || f != nil || p.Colors == nil || p.LaunchPrices == nil || p.Source != nil || p.Specs != nil || len(p.Colors) != 0 || len(p.LaunchPrices) != 0 || v.Slug != nil || v.DemoPriceBasis != nil || v.LaunchPrice != nil {
			t.Fatal("empty display projection differs")
		}
	}
	raw := `{"apple_model_specs":null,"release_year":2020,"display_size_inches":6.10,"rear_camera_count":0,"height_mm":"146.7","tech_specs_url":"https://apple.com/specs"}`
	p, e := productDisplay(&raw)
	if e != nil || p.Specs == nil || p.Specs.ReleaseYear != "2020" || p.Specs.DisplaySize != "6.1" || p.Specs.RearCameraCount != "0" || p.Specs.Height != "146.7" || p.Specs.TechSpecsURL == "" {
		t.Fatal("reviewed root specification fallback differs")
	}
	raw = `{"apple_model_specs":{},"release_year":2020}`
	p, e = productDisplay(&raw)
	if e != nil || p.Specs == nil || p.Specs.ReleaseYear != "" {
		t.Fatal("nested specs incorrectly merged with root")
	}
	for _, href := range []string{"javascript:alert(1)", "http://apple.com/specs", "https://apple.com.evil.example/specs", "https://user@apple.com/specs", "https://apple.com:443/specs", "https://example.com/specs"} {
		raw = `{"apple_model_specs":{"tech_specs_url":` + quoted(href) + `}}`
		p, e = productDisplay(&raw)
		if e != nil || p.Specs == nil || p.Specs.TechSpecsURL != "" {
			t.Fatal("unreviewed technical link escaped")
		}
	}
	for _, raw := range []string{`{"slug":"javascript:bad","demo_price_basis":"secret"}`, `{"slug":"iphone-13-S85?secret"}`, `{"slug":"iphone-13-s85"}`, `{"apple_launch_price":{"amount":0,"currency_code":"eur"}}`, `{"apple_launch_price":{"amount":-1,"currency_code":"eur"}}`, `{"apple_launch_price":{"amount":10,"currency_code":"EUR"}}`, `{"apple_launch_price":{"amount":"10","currency_code":"eur"}}`} {
		v, e := variantDisplay(&raw)
		if e != nil || v.Slug != nil || v.DemoPriceBasis != nil || v.LaunchPrice != nil {
			t.Fatal("optional display type guard differs")
		}
	}
}

func TestStoreDisplayBounds(t *testing.T) {
	for _, raw := range []string{`[]`, `"secret"`, `{`, `{} {}`, `{"slug":1,"slug":2}`, `{"private":{"x":1,"x":2}}`, "{\"private\":\"" + strings.Repeat("x", 65536) + "\"}", `{"private":` + strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33) + "}", string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})} {
		if _, e := productDisplay(&raw); e == nil {
			t.Fatal("unsafe metadata container accepted")
		}
		if _, e := variantDisplay(&raw); e == nil {
			t.Fatal("unsafe variant metadata container accepted")
		}
	}
	for _, raw := range []string{`{"card_colors":[` + strings.Repeat(`{},`, 64) + `{}]}`, `{"apple_model_specs":{"apple_model_numbers":[` + strings.Repeat(`"A",`, 64) + `"A"]}}`} {
		if _, e := productDisplay(&raw); e == nil {
			t.Fatal("display relation bound ignored")
		}
	}
}

func quoted(s string) string { b, _ := json.Marshal(s); return string(b) }
func ptr(s string) *string   { return &s }
