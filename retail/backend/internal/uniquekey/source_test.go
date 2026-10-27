package uniquekey

import (
	"encoding/json"
	"testing"
)

func row(values map[string]any) TargetRow {
	out := TargetRow{}
	for key, value := range values {
		out[key], _ = json.Marshal(value)
	}
	return out
}

func TestAllSourcePlans(t *testing.T) {
	var document struct {
		Plans []sourcePlan `json:"plans"`
	}
	if err := json.Unmarshal(sourcePlans, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Plans) != 28 {
		t.Fatal("source plan coverage drift")
	}
	scopes := map[string]bool{}
	protected := 0
	for _, plan := range document.Plans {
		t.Run(plan.Scope, func(t *testing.T) {
			if scopes[plan.Scope] || !scopePattern.MatchString(plan.Scope) {
				t.Fatal("scope collision")
			}
			scopes[plan.Scope] = true
			values := map[string]any{"id": "artificial-row"}
			// Include every plan on the table (tax_region/mail_contact have two).
			for _, sibling := range document.Plans {
				if sibling.TargetTable != plan.TargetTable {
					continue
				}
				for _, field := range sibling.Keys {
					if field.Kind == "bool" {
						values[field.Column] = false
					} else {
						values[field.Column] = "Case-sensitive 完整值 "
					}
				}
			}
			for _, sibling := range document.Plans {
				if sibling.TargetTable != plan.TargetTable {
					continue
				}
				for _, condition := range sibling.Conditions {
					if condition.Operation == "is_null" {
						values[condition.Column] = nil
					} else {
						values[condition.Column] = false
					}
				}
			}
			changes, err := SourceChanges(plan.TargetTable, row(values))
			if plan.Protected {
				protected++
				if err != ErrProtected {
					t.Fatal("protected plaintext accepted")
				}
				return
			}
			if plan.TargetTable == "mail_contacts" {
				if err != ErrProtected {
					t.Fatal("partial protected table claims accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var found *Change
			for i := range changes {
				if changes[i].Scope == plan.Scope {
					found = &changes[i]
				}
			}
			if found == nil || !found.Active || found.Owner != "artificial-row" {
				t.Fatal("missing active adapter")
			}
			for _, condition := range plan.Conditions {
				bad := row(values)
				delete(bad, condition.Column)
				if _, err := SourceChanges(plan.TargetTable, bad); err != ErrInvalid {
					t.Fatal("missing participation input accepted")
				}
			}
			bad := row(values)
			delete(bad, plan.Keys[0].Column)
			if _, err := SourceChanges(plan.TargetTable, bad); err != ErrInvalid {
				t.Fatal("missing key accepted")
			}
		})
	}
	if protected != 1 {
		t.Fatal("protected scope count drift")
	}
}

func TestSourcePredicateBoundaries(t *testing.T) {
	r := row(map[string]any{"id": "option", "title": "", "is_exclusive": false, "deleted_at": nil})
	cs, err := SourceChanges("catalog_options", r)
	if err != nil || len(cs) != 1 || !cs[0].Active || *cs[0].Components[0] != "" {
		t.Fatal("empty active title lost")
	}
	var d struct {
		Plans []sourcePlan `json:"plans"`
	}
	_ = json.Unmarshal(sourcePlans, &d)
	for _, p := range d.Plans {
		for _, c := range p.Conditions {
			if c.Operation != "is_false" {
				continue
			}
			r["is_exclusive"] = json.RawMessage("true")
			out, err := SourceChanges(p.TargetTable, r)
			if err != nil || out[0].Active {
				t.Fatal("exclusive participates")
			}
			r["is_exclusive"] = json.RawMessage("false")
			r["deleted_at"] = json.RawMessage(`"2026-10-27T00:00:00Z"`)
			out, err = SourceChanges(p.TargetTable, r)
			if err != nil || out[0].Active {
				t.Fatal("deleted participates")
			}
		}
	}
}

func TestTypedInputDoesNotNormalize(t *testing.T) {
	f := sourceField{Column: "value", Kind: "text", Nullable: true}
	for _, raw := range []string{`123`, `true`, `"\ud800"`, `"\udc00"`, `"\ud800x"`} {
		if _, err := fieldValue(TargetRow{"value": json.RawMessage(raw)}, f); err != ErrInvalid {
			t.Fatalf("malformed input accepted: %s", raw)
		}
	}
	for _, raw := range []string{`"\ud83d\ude80"`, `"Case "`, `"\\ud800"`, `""`, ` null `} {
		if _, err := fieldValue(TargetRow{"value": json.RawMessage(raw)}, f); err != nil {
			t.Fatalf("valid input rejected: %s", raw)
		}
	}
	if _, err := SourceChanges("unknown", TargetRow{}); err != ErrInvalid {
		t.Fatal("unknown table accepted")
	}
}

func TestHistoricalCustomerAndTaxKeys(t *testing.T) {
	var d struct {
		Plans []sourcePlan `json:"plans"`
	}
	_ = json.Unmarshal(sourcePlans, &d)
	for _, p := range d.Plans {
		for _, key := range p.Keys {
			if key.Column != "legacy_had_account" {
				continue
			}
			base := map[string]any{"id": "historical-subject", "email": "Same@Example.test", "legacy_had_account": false, "deleted_at": nil}
			a, err := SourceChanges(p.TargetTable, row(base))
			if err != nil {
				t.Fatal(err)
			}
			if *a[0].Components[1] != "false" {
				t.Fatal("false legacy marker lost")
			}
			delete(base, "legacy_had_account")
			base["has_account"] = false
			if _, err := SourceChanges(p.TargetTable, row(base)); err != ErrInvalid {
				t.Fatal("legacy/source alias accepted as target row")
			}
		}
		if len(p.Conditions) != 2 || p.Conditions[0].Column != "province_code" {
			continue
		}
		for _, province := range []any{nil, "", "region"} {
			changes, err := SourceChanges(p.TargetTable, row(map[string]any{"id": "tax", "country_code": "dk", "province_code": province, "deleted_at": nil}))
			if err != nil || len(changes) != 2 {
				t.Fatal("tax plans missing", err)
			}
			for _, change := range changes {
				if change.Scope == p.Scope && change.Active != (province == nil) {
					t.Fatal("country-only condition changed")
				}
				if change.Scope != p.Scope && province == nil && change.Components[1] != nil {
					t.Fatal("NULL province coerced")
				}
			}
		}
	}
}
