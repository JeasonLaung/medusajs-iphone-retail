package devices

import "testing"

func TestConfigurationMatchesSource(t *testing.T) {
	legacy, e := ResolveConfiguration([]VariantOption{{"Condition", "Very good"}}, Configuration{})
	if e != nil || legacy.Grade == nil || *legacy.Grade != "VERY GOOD" {
		t.Fatal("source SKU grade vocabulary narrowed")
	}

	grade := "A"
	battery := 92
	options := []VariantOption{{"Condition", "B"}, {"Battery", "85%"}}
	got, e := ResolveConfiguration(options, Configuration{})
	if e != nil || got.Grade == nil || *got.Grade != "B" || got.BatteryHealth == nil || *got.BatteryHealth != 85 {
		t.Fatal("SKU defaults lost")
	}
	if _, e := ResolveConfiguration(options, Configuration{BatteryHealth: &battery}); e != nil {
		t.Fatal("measured health above minimum rejected")
	}
	if _, e := ResolveConfiguration(options, Configuration{Grade: &grade}); e != ErrInput {
		t.Fatal("grade mismatch accepted")
	}
	battery = 84
	if _, e := ResolveConfiguration(options, Configuration{BatteryHealth: &battery}); e != ErrInput {
		t.Fatal("battery below tier accepted")
	}
	if _, e := ResolveConfiguration([]VariantOption{{"Battery", "85.5%"}}, Configuration{}); e != ErrInput {
		t.Fatal("fractional default silently truncated")
	}
	battery = 86
	if _, e := ResolveConfiguration([]VariantOption{{"Battery", "85.5%"}}, Configuration{BatteryHealth: &battery}); e != nil {
		t.Fatal("valid measurement rejected")
	}
	if _, e := ResolveConfiguration([]VariantOption{{"Storage", "128GB"}}, Configuration{}); e != nil {
		t.Fatal("optional grade/battery invented")
	}
}
