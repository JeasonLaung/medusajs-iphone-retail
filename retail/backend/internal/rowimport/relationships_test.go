package rowimport

import (
	"bytes"
	"encoding/json"
	"testing"
)

func relationFixture(c *relationCollector, table, id string) SourceRow {
	plan := c.plans[table]
	row := SourceRow{}
	for _, f := range plan.Fields {
		if f.Nullable {
			row[f.Source] = Cell{SQLNull: true}
		} else {
			row[f.Source] = Cell{Raw: artificial(f)}
		}
	}
	for _, column := range plan.PrimaryKey {
		if c.fields[table][column].Transform == "strict_boolean" {
			row[column] = Cell{Raw: json.RawMessage(`true`)}
			continue
		}
		value, _ := json.Marshal(id)
		row[column] = Cell{Raw: value}
	}
	return row
}

func TestAllPinnedPrimaryKeysAndReferences(t *testing.T) {
	c := relationCollectorTest(t)
	for _, plan := range c.plans {
		row := relationFixture(c, plan.Source, "owner")
		for _, rule := range plan.References {
			relationText(row, rule.Column, "missing")
		}
		// Composite PK columns can themselves be foreign keys; all use their
		// complete tuple, even when the individual values are equal.
		if err := c.add(plan.Source, row); err != nil {
			t.Fatal("pinned relationship row refused", plan.Source, err)
		}
	}
	evidence, err := c.finish()
	if err != ErrReferences || evidence.Rows != 159 || evidence.NonNullReferences != 224 || len(evidence.Missing) != 224 || len(evidence.PhysicalMissing) != 88 {
		t.Fatal("pinned reference coverage mismatch")
	}
}

func relationText(row SourceRow, column, value string) {
	raw, _ := json.Marshal(value)
	row[column] = Cell{Raw: raw}
}
func relationCollectorTest(t *testing.T) *relationCollector {
	t.Helper()
	c, err := newRelationCollector(RelationLimits{MaxRows: 1000, KeyBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestForwardDeletedAndExactReferences(t *testing.T) {
	for _, id := range []string{"Exact", "Exact ", "exact", "é", "e\u0301", "", "a\x00b"} {
		c := relationCollectorTest(t)
		child := relationFixture(c, "product_option_value", "child")
		relationText(child, "option_id", id)
		parent := relationFixture(c, "product_option", id)
		relationText(parent, "deleted_at", "2026-10-27T00:00:00.123456Z")
		if c.add("product_option_value", child) != nil || c.add("product_option", parent) != nil {
			t.Fatal("forward fixture rejected")
		}
		evidence, err := c.finish()
		if err != nil || evidence.ResolvedReferences != 1 || evidence.ResolvedToDeletedRows["product_option_value.option_id"] != 1 {
			t.Fatal("soft deleted parent not resolved")
		}
	}
	for _, value := range []string{"Exact ", "exact", "EXACT", "É"} {
		c := relationCollectorTest(t)
		child := relationFixture(c, "product_option_value", "child")
		relationText(child, "option_id", value)
		if c.add("product_option_value", child) != nil || c.add("product_option", relationFixture(c, "product_option", "Exact")) != nil {
			t.Fatal("fixture rejected")
		}
		evidence, err := c.finish()
		if err != ErrReferences || evidence.Missing["product_option_value.option_id"] != 1 || len(evidence.PhysicalMissing) != 1 {
			t.Fatal("reference normalized or orphan hidden")
		}
	}
}

func TestPrimaryKeysCompositeEncodingAndDuplicates(t *testing.T) {
	c := relationCollectorTest(t)
	for i, values := range [][2]string{{"ab", "c"}, {"a", "bc"}, {"a\x00", "b"}, {"a", "\x00b"}} {
		row := relationFixture(c, "product_tags", "unused")
		relationText(row, "product_id", values[0])
		relationText(row, "product_tag_id", values[1])
		if err := c.add("product_tags", row); err != nil {
			t.Fatal("distinct composite keys collided", i)
		}
	}
	row := relationFixture(c, "product_tags", "unused")
	relationText(row, "product_id", "ab")
	relationText(row, "product_tag_id", "c")
	if c.add("product_tags", row) != ErrRow {
		t.Fatal("duplicate composite key accepted")
	}
	c = relationCollectorTest(t)
	row = relationFixture(c, "product_tag", "same")
	if c.add("product_tag", row) != nil {
		t.Fatal("primary fixture rejected")
	}
	relationText(row, "deleted_at", "2026-10-27T00:00:00Z")
	if c.add("product_tag", row) != ErrRow {
		t.Fatal("soft deletion allowed duplicate source PK")
	}
}

func TestHistoricalMissingStillFails(t *testing.T) {
	c := relationCollectorTest(t)
	row := relationFixture(c, "reservation_item", "reservation")
	relationText(row, "deleted_at", "2026-10-27T00:00:00Z")
	relationText(row, "line_item_id", "missing-line")
	relationText(row, "inventory_item_id", "missing-inventory")
	relationText(row, "location_id", "missing-location")
	if c.add("reservation_item", row) != nil {
		t.Fatal("fixture rejected")
	}
	evidence, err := c.finish()
	if err != ErrReferences || evidence.HistoryPolicyMissing["reservation_item.line_item_id"] != 1 || len(evidence.MissingOnDeletedRows) != 3 {
		t.Fatal("history orphan accepted or omitted")
	}
}

func TestCyclicReferencesAndNulls(t *testing.T) {
	c := relationCollectorTest(t)
	for _, ids := range [][2]string{{"region-a", "region-b"}, {"region-b", "region-a"}} {
		row := relationFixture(c, "tax_region", ids[0])
		relationText(row, "parent_id", ids[1])
		if c.add("tax_region", row) != nil {
			t.Fatal("cyclic fixture rejected")
		}
	}
	evidence, err := c.finish()
	if err != nil || evidence.ResolvedReferences != 2 || evidence.NullReferences != 2 {
		t.Fatal("forward cycle or nullable reference incorrect")
	}
	// This is relational presence only, not acyclic domain-tree acceptance.
}

func TestRelationshipLimitsAndInvalidKeys(t *testing.T) {
	for _, limits := range []RelationLimits{{}, {MaxRows: -1, KeyBytes: 1}, {MaxRows: 1}} {
		if _, err := newRelationCollector(limits); err != ErrRelationLimit {
			t.Fatal("missing limits accepted")
		}
	}
	c := relationCollectorTest(t)
	c.limits.MaxRows = 1
	if c.add("product_tag", relationFixture(c, "product_tag", "a")) != nil || c.add("product_tag", relationFixture(c, "product_tag", "b")) != ErrRelationLimit {
		t.Fatal("row bound ignored")
	}
	c = relationCollectorTest(t)
	c.limits.KeyBytes = 1
	if c.add("product_tag", relationFixture(c, "product_tag", "a")) != ErrRelationLimit {
		t.Fatal("payload bound ignored")
	}
	for _, cell := range []Cell{{SQLNull: true}, {Raw: json.RawMessage(`7`)}, {Raw: json.RawMessage(`"\ud800"`)}} {
		c = relationCollectorTest(t)
		row := relationFixture(c, "product_tag", "a")
		row["id"] = cell
		if c.add("product_tag", row) != ErrRow {
			t.Fatal("invalid primary key accepted")
		}
	}
}

func TestVerifiedRelationshipStreamAndFailureEvidence(t *testing.T) {
	data := streamFixture()
	limits := RelationLimits{MaxRows: 1000, KeyBytes: 1 << 20}
	evidence, err := VerifyRelationships(bytes.NewReader(data), sum(data), limits)
	if err != nil || evidence.Relations.Rows != 1 || len(evidence.Snapshot.Tables) != 159 {
		t.Fatal("verified stream refused", err)
	}
	badHash := sum([]byte("other"))
	evidence, err = VerifyRelationships(bytes.NewReader(data), badHash, limits)
	if err != ErrRow || evidence.Snapshot.Tables != nil || evidence.Relations.Missing != nil {
		t.Fatal("unverified evidence escaped")
	}
	limits.KeyBytes = 1
	evidence, err = VerifyRelationships(bytes.NewReader(data), sum(data), limits)
	if err != ErrRelationLimit || evidence.Snapshot.Tables != nil {
		t.Fatal("stream limit not propagated")
	}
}

func relationStreamFixture(t *testing.T, rows map[string][]SourceRow) []byte {
	t.Helper()
	data := streamFixture()
	var result bytes.Buffer
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var control map[string]json.RawMessage
		if json.Unmarshal(line, &control) != nil {
			t.Fatal("invalid base fixture")
		}
		var kind, table string
		_ = json.Unmarshal(control["kind"], &kind)
		_ = json.Unmarshal(control["table"], &table)
		if kind == "table_end" && rows[table] != nil {
			for _, row := range rows[table] {
				cells := map[string]any{}
				for col, cell := range row {
					if cell.SQLNull {
						cells[col] = map[string]any{"sql_null": true}
					} else {
						cells[col] = map[string]any{"sql_null": false, "value_json": string(cell.Raw)}
					}
				}
				record, _ := json.Marshal(map[string]any{"format_version": 2, "table": table, "cells": cells})
				result.Write(record)
				result.WriteByte('\n')
			}
			control["rows"], _ = json.Marshal(len(rows[table]))
			line, _ = json.Marshal(control)
		}
		result.Write(line)
		result.WriteByte('\n')
	}
	return result.Bytes()
}

func TestOrphanStreamKeepsEvidenceOnlyAfterFullIntegrity(t *testing.T) {
	c := relationCollectorTest(t)
	child := relationFixture(c, "product_option_value", "child")
	relationText(child, "option_id", "missing-parent")
	data := relationStreamFixture(t, map[string][]SourceRow{"product_option_value": {child}})
	limits := RelationLimits{MaxRows: 100, KeyBytes: 1 << 20}
	evidence, err := VerifyRelationships(bytes.NewReader(data), sum(data), limits)
	if err != ErrReferences || len(evidence.Snapshot.Tables) != 159 || evidence.Relations.Missing["product_option_value.option_id"] != 1 {
		t.Fatal("orphan stream evidence incorrect", err)
	}
	for _, bad := range [][]byte{data[:len(data)-1], append(bytes.Clone(data), '\n')} {
		evidence, err = VerifyRelationships(bytes.NewReader(bad), sum(bad), limits)
		if err != ErrRow || evidence.Relations.Missing != nil || evidence.Snapshot.Tables != nil {
			t.Fatal("unverified orphan diagnostics escaped")
		}
	}
	evidence, err = VerifyRelationships(bytes.NewReader(data), sum([]byte("bad hash")), limits)
	if err != ErrRow || evidence.Relations.Missing != nil {
		t.Fatal("hash mismatch preserved orphan evidence")
	}
}
