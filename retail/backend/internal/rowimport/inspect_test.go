package rowimport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectValuesMatchesAllOrdinaryConversions(t *testing.T) {
	plans, _ := compiledPlans()
	for table, p := range plans {
		r := SourceRow{}
		for _, f := range p.Fields {
			r[f.Source] = Cell{Raw: artificial(f)}
		}
		satisfyFixtureChecks(p, r)
		inspection, err := InspectValues(table, r)
		if err != nil || inspection.Fields != len(p.Fields) {
			t.Fatal("inspection coverage differs", table)
		}
		_, conversionErr := Convert(table, r)
		if inspection.OrdinaryConvertible != (conversionErr == nil) {
			t.Fatal("inspection disagrees with ordinary gate", table)
		}
		for _, f := range p.Fields {
			if !strings.HasPrefix(f.Transform, "protected_") {
				continue
			}
			found := false
			for _, column := range inspection.ProtectedColumns {
				if column == f.Source {
					found = true
				}
			}
			if !found {
				t.Fatal("protected column omitted", table, f.Source)
			}
		}
	}
}

func TestInspectDoesNotMaskInvalidValuesBehindHistoryOrSecrets(t *testing.T) {
	plans, _ := compiledPlans()
	for _, table := range []string{"mail_account", "payment_session", "payment", "mail_message"} {
		p := plans[table]
		r := SourceRow{}
		for _, f := range p.Fields {
			r[f.Source] = Cell{Raw: artificial(f)}
		}
		satisfyFixtureChecks(p, r)
		r["updated_at"] = Cell{Raw: json.RawMessage(`"private-invalid-timestamp"`)}
		v, err := InspectValues(table, r)
		if err != nil || v.OrdinaryConvertible || len(v.InvalidColumns) == 0 {
			t.Fatal("ordinary invalid input hidden by policy", table)
		}
		found := false
		for _, column := range v.InvalidColumns {
			if column == "updated_at" {
				found = true
			}
		}
		if !found {
			t.Fatal("later timestamp not inspected", table)
		}
		encoded, _ := json.Marshal(v)
		if strings.Contains(string(encoded), "private-invalid") {
			t.Fatal("private value leaked")
		}
		if table == "payment" || table == "payment_session" {
			r["amount"] = Cell{Raw: json.RawMessage(`"2.34"`)}
			v, err = InspectValues(table, r)
			if err != nil || v.NumberPairsValid {
				t.Fatal("bad pair hidden behind history/protection")
			}
		}
	}
}

func TestInspectRejectsCoverageAndMalformedSQLNull(t *testing.T) {
	plans, _ := compiledPlans()
	p := plans["mail_account"]
	r := SourceRow{}
	for _, f := range p.Fields {
		r[f.Source] = Cell{Raw: artificial(f)}
	}
	r["deleted_at"] = Cell{SQLNull: true, Raw: json.RawMessage(`null`)}
	v, err := InspectValues("mail_account", r)
	if err != nil || v.OrdinaryConvertible || v.SQLNulls != 1 || len(v.InvalidColumns) != 1 || v.InvalidColumns[0] != "deleted_at" {
		t.Fatal("malformed NULL accepted")
	}
	delete(r, "name")
	r["private-unknown-column"] = Cell{Raw: json.RawMessage(`"private-value"`)}
	if v, err := InspectValues("mail_account", r); err != ErrRow || v.Fields != 0 {
		t.Fatal("unknown/missing columns returned partial evidence")
	}
	if v, err := InspectValues("private-table", r); err != ErrRow || v.Fields != 0 {
		t.Fatal("unknown table accepted")
	}
}
