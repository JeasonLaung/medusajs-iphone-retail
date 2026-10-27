package rowimport

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/eurorefurb/retail/internal/legacysecret"
	"github.com/eurorefurb/retail/internal/protected"
)

func TestConfigStringSealingAndContext(t *testing.T) {
	ring, _ := protected.New("artificial-key", map[string][]byte{"artificial-key": bytes.Repeat([]byte{7}, 32)})
	source, _ := legacysecret.New(nil)
	converter, err := NewConfigSecrets(ring, source)
	if err != nil {
		t.Fatal(err)
	}
	plans, _ := compiledPlans()
	count := 0
	for _, table := range []string{"mail_account", "seo_provider", "seo_setting", "webhook_endpoint", "mollie_config", "mail_contact", "paypal_connection"} {
		plan := plans[table]
		row := SourceRow{}
		for _, f := range plan.Fields {
			switch {
			case f.Transform == "protected_string":
				row[f.Source] = Cell{Raw: json.RawMessage(`""`)}
			case f.Nullable:
				row[f.Source] = Cell{SQLNull: true}
			default:
				row[f.Source] = Cell{Raw: artificial(f)}
			}
		}
		if table == "paypal_connection" || table == "mail_contact" {
			for _, f := range plan.Fields {
				if f.Transform == "protected_string" {
					row[f.Source] = Cell{Raw: json.RawMessage(`" 原文 merchant 🚀 "`)}
				}
			}
		}
		row["id"] = Cell{Raw: json.RawMessage(`"fixture-row"`)}
		satisfyFixtureChecks(plan, row)
		if table == "seo_setting" {
			row["is_secret"] = Cell{Raw: json.RawMessage(`false`)}
			row["value"] = Cell{Raw: json.RawMessage(`"not.ciphertext.at.all"`)}
		}
		bound, err := converter.Convert(table, row)
		if err != nil {
			t.Fatal("config fixture rejected", table, err)
		}
		for i, f := range plan.Fields {
			if f.Transform != "protected_string" {
				continue
			}
			count++
			_, purpose, err := configDomain(table, f.Source, row)
			if err != nil {
				t.Fatal(err)
			}
			context := protected.Context{Purpose: purpose, Table: plan.Target, Column: f.Target, Row: "fixture-row"}
			sealed, ok := bound.Values[i].([]byte)
			if !ok || len(sealed) == 0 {
				t.Fatal("protected string was not sealed")
			}
			plain, err := ring.Open(context, sealed)
			want := ""
			if table == "seo_setting" {
				want = "not.ciphertext.at.all"
			}
			if table == "paypal_connection" || table == "mail_contact" {
				want = " 原文 merchant 🚀 "
			}
			if err != nil || string(plain) != want {
				t.Fatal("protected value semantics changed")
			}
			context.Row = "another-row"
			if _, err := ring.Open(context, sealed); err == nil {
				t.Fatal("envelope row not bound")
			}
		}
		if _, err := Convert(table, row); err != ErrProtection {
			t.Fatal("ordinary importer bypassed protected fields")
		}
		if table == "seo_setting" {
			row["is_secret"] = Cell{Raw: json.RawMessage(`true`)}
			if bound, err := converter.Convert(table, row); err != ErrProtection || len(bound.Values) != 0 {
				t.Fatal("secret SEO setting treated as plaintext")
			}
		}
		if table == "mail_account" {
			row["api_key"] = Cell{Raw: json.RawMessage(`"unrecognized-ciphertext"`)}
			if bound, err := converter.Convert(table, row); err != ErrProtection || len(bound.Values) != 0 {
				t.Fatal("invalid ciphertext returned partial row")
			}
		}
		if table == "paypal_connection" {
			row["auth_code"] = Cell{SQLNull: true}
			bound, err := converter.Convert(table, row)
			if err != nil {
				t.Fatal("nullable merchant secret rejected")
			}
			for i, f := range plan.Fields {
				if f.Source == "auth_code" && bound.Values[i] != nil {
					t.Fatal("SQL NULL secret became envelope")
				}
			}
			row["metadata"] = Cell{Raw: json.RawMessage(`{"token":"must-remain-held"}`)}
			if bound, err := converter.Convert(table, row); err != ErrProtection || len(bound.Values) != 0 {
				t.Fatal("protected JSON bypassed review")
			}
		}
	}
	if count != 12 {
		t.Fatal("config field coverage drift", count)
	}
	if _, err := NewConfigSecrets(nil, source); err != ErrProtection {
		t.Fatal("missing new ring accepted")
	}
}

func TestActualNodeLegacyRowConversion(t *testing.T) {
	path := os.Getenv("LEGACY_SECRET_NODE_FIXTURE")
	if path == "" {
		t.Skip("requires artificial actual TypeScript source fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("artificial Node fixture missing")
	}
	var fixtures []struct{ Domain, Secret, Plaintext, Ciphertext string }
	if json.Unmarshal(data, &fixtures) != nil || len(fixtures) != 16 {
		t.Fatal("invalid artificial Node fixture")
	}
	plans, _ := compiledPlans()
	ring, _ := protected.New("fixture-new", map[string][]byte{"fixture-new": bytes.Repeat([]byte{13}, 32)})
	for _, fixture := range fixtures {
		table := map[string]string{"mail": "mail_account", "seo": "seo_provider", "webhook": "webhook_endpoint", "mollie": "mollie_config"}[fixture.Domain]
		keys, err := legacysecret.New(map[string]string{fixture.Domain: fixture.Secret})
		if err != nil {
			t.Fatal("fixture keys invalid")
		}
		converter, _ := NewConfigSecrets(ring, keys)
		plan := plans[table]
		row := SourceRow{}
		for _, f := range plan.Fields {
			switch {
			case f.Transform == "protected_string":
				raw, _ := json.Marshal(fixture.Ciphertext)
				row[f.Source] = Cell{Raw: raw}
			case f.Nullable:
				row[f.Source] = Cell{SQLNull: true}
			default:
				row[f.Source] = Cell{Raw: artificial(f)}
			}
		}
		row["id"] = Cell{Raw: json.RawMessage(`"fixture-source-row"`)}
		satisfyFixtureChecks(plan, row)
		bound, err := converter.Convert(table, row)
		if err != nil {
			t.Fatal("actual source ciphertext row rejected")
		}
		for i, f := range plan.Fields {
			if f.Transform != "protected_string" {
				continue
			}
			sealed, ok := bound.Values[i].([]byte)
			if !ok {
				t.Fatal("secret returned without binary envelope")
			}
			plain, err := ring.Open(protected.Context{Purpose: "merchant-config", Table: plan.Target, Column: f.Target, Row: "fixture-source-row"}, sealed)
			if err != nil || string(plain) != fixture.Plaintext {
				t.Fatal("actual source-to-target row bytes changed")
			}
		}
	}
}
