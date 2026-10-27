package rowimport

import (
	"encoding/json"
	"os"
	"testing"
)

func recordFixture() []byte {
	cells := map[string]any{
		"id":         map[string]any{"sql_null": false, "value_json": `"fixture-id"`},
		"value":      map[string]any{"sql_null": false, "value_json": `" Exact tag "`},
		"metadata":   map[string]any{"sql_null": false, "value_json": `null`},
		"created_at": map[string]any{"sql_null": false, "value_json": `"2026-10-27T08:00:00.123456+08:00"`},
		"updated_at": map[string]any{"sql_null": false, "value_json": `"2026-10-27T08:00:00.123456+08:00"`},
		"deleted_at": map[string]any{"sql_null": true}, "external_id": map[string]any{"sql_null": true},
	}
	data, _ := json.Marshal(map[string]any{"format_version": 2, "table": "product_tag", "cells": cells})
	return data
}

func TestRecordNullsAndRendering(t *testing.T) {
	table, row, err := DecodeRecord(recordFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !row["deleted_at"].SQLNull || row["metadata"].SQLNull || string(row["metadata"].Raw) != "null" {
		t.Fatal("NULL distinction lost")
	}
	bound, err := Convert(table, row)
	if err != nil || bound.Table != "catalog_tags" {
		t.Fatal("record conversion failed", err)
	}
}

func TestRecordMalformedAndDuplicateKeys(t *testing.T) {
	for _, raw := range []string{
		`{"format_version":2,"format_version":2,"table":"product_tag","cells":{}}`,
		`{"format_version":1,"table":"product_tag","cells":{}}`,
		`{"format_version":2,"table":"user","cells":{}}`,
		`{"format_version":2,"table":"product_tag","cells":{},"extra":true}`,
		`{} {}`, `[]`, `null`,
	} {
		if _, _, err := DecodeRecord([]byte(raw)); err != ErrRow {
			t.Fatal("unsafe record accepted")
		}
	}
	var original map[string]json.RawMessage
	_ = json.Unmarshal(recordFixture(), &original)
	var cells map[string]json.RawMessage
	_ = json.Unmarshal(original["cells"], &cells)
	for _, bad := range []string{`{"sql_null":true,"value_json":"null"}`, `{"sql_null":false}`, `{"sql_null":false,"value_json":null}`, `{"sql_null":false,"value_json":"NaN"}`, `{"sql_null":false,"sql_null":false,"value_json":"null"}`, `{"sql_null":true,"extra":true}`} {
		cells["metadata"] = json.RawMessage(bad)
		original["cells"], _ = json.Marshal(cells)
		record, _ := json.Marshal(original)
		if _, _, err := DecodeRecord(record); err != ErrRow {
			t.Fatal("unsafe cell accepted", bad)
		}
	}
}

func TestActualPostgresCellRecords(t *testing.T) {
	path := os.Getenv("ROWIMPORT_POSTGRES_FIXTURE")
	if path == "" {
		t.Skip("requires isolated artificial PostgreSQL fixture output")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("fixture record file unavailable")
	}
	var records []json.RawMessage
	if json.Unmarshal(data, &records) != nil || len(records) != 3 {
		t.Fatal("fixture record count changed")
	}
	for i, record := range records {
		table, row, err := DecodeRecord(record)
		if err != nil {
			t.Fatal("actual PostgreSQL record rejected", err)
		}
		if table != "product_tag" {
			t.Fatal("unexpected source table")
		}
		bound, err := Convert(table, row)
		if err != nil {
			t.Fatal("actual source conversion failed", err)
		}
		byColumn := map[string]any{}
		for n, column := range bound.Columns {
			byColumn[column] = bound.Values[n]
		}
		if byColumn["created_at"] != "2026-10-27 00:00:00.123456" {
			t.Fatal("actual source timezone changed")
		}
		if i == 0 {
			if !row["metadata"].SQLNull || byColumn["metadata"] != nil {
				t.Fatal("SQL NULL not preserved")
			}
		}
		if i == 1 {
			if row["metadata"].SQLNull || byColumn["metadata"] != "null" {
				t.Fatal("JSON null collapsed")
			}
		}
		if i == 2 {
			if string(row["metadata"].Raw) != `{"big": 9007199254740993, "amount": 1.2300}` {
				t.Fatal("numeric lexemes changed")
			}
		}
	}
}
