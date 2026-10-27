package snapshotcheck

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/eurorefurb/retail/internal/rowimport"
)

func checksum(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func fixture(t *testing.T, missingParent, invalidTime bool) []byte {
	t.Helper()
	data, err := os.ReadFile("../rowimport/field_plans.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Tables []struct {
			Source string `json:"source"`
			Fields []struct {
				Source    string `json:"source"`
				Transform string `json:"transform"`
				Nullable  bool   `json:"nullable"`
			} `json:"fields"`
		} `json:"tables"`
	}
	if json.Unmarshal(data, &plan) != nil {
		t.Fatal("fixture plan invalid")
	}
	var out bytes.Buffer
	emit := func(value any) {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		out.Write(data)
		out.WriteByte('\n')
	}
	emit(map[string]any{"kind": "snapshot", "format_version": 2, "plan_sha256": checksum(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))})
	for _, table := range plan.Tables {
		emit(map[string]any{"kind": "table_begin", "table": table.Source})
		rows := 0
		if table.Source == "product_tag" || table.Source == "product_category" {
			cells := map[string]any{}
			for _, f := range table.Fields {
				if f.Nullable {
					cells[f.Source] = map[string]any{"sql_null": true}
					continue
				}
				var value any = "private-business-marker"
				switch f.Transform {
				case "strict_boolean":
					value = true
				case "checked_int32":
					value = 0
				case "utc_datetime6":
					value = "2026-10-27T00:00:00.000001Z"
				}
				if invalidTime && f.Source == "updated_at" {
					value = "invalid-private-time"
				}
				raw, _ := json.Marshal(value)
				cells[f.Source] = map[string]any{"sql_null": false, "value_json": string(raw)}
			}
			if table.Source == "product_category" && missingParent {
				cells["parent_category_id"] = map[string]any{"sql_null": false, "value_json": `"private-missing-parent"`}
			}
			emit(map[string]any{"format_version": 2, "table": table.Source, "cells": cells})
			rows = 1
		}
		emit(map[string]any{"kind": "table_end", "table": table.Source, "rows": rows})
	}
	emit(map[string]any{"kind": "snapshot_complete", "tables": 159})
	return out.Bytes()
}

func TestDiagnosticsNeverAcceptImportAndExposeOnlyAggregates(t *testing.T) {
	limits := rowimport.RelationLimits{MaxRows: 10, KeyBytes: 1 << 20}
	for _, sample := range []struct{ missing, invalid bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		data := fixture(t, sample.missing, sample.invalid)
		r, err := Check(bytes.NewReader(data), checksum(data), limits)
		if err != nil || r.Counts.Tables != 159 || r.Counts.Rows != 2 || r.WritesEnabled || r.CompleteImportAccepted {
			t.Fatal("incorrect diagnostic boundary")
		}
		if r.ScalarAndReferenceChecksPassed != (!sample.missing && !sample.invalid) {
			t.Fatal("failures not propagated")
		}
		if sample.invalid && r.Counts.InvalidRows != 2 {
			t.Fatal("invalid later fields masked")
		}
		if sample.missing && r.Relations.Missing["product_category.parent_category_id"] != 1 {
			t.Fatal("missing reference lost")
		}
		encoded, _ := json.Marshal(r)
		if bytes.Contains(encoded, []byte("private-")) {
			t.Fatal("private value leaked")
		}
	}
}

type changingReader struct {
	*bytes.Reader
	replacement []byte
	seeks       int
	fail        bool
}

func (r *changingReader) Seek(offset int64, whence int) (int64, error) {
	r.seeks++
	if r.seeks == 2 {
		if r.fail {
			return 0, errors.New("private-reader-error")
		}
		r.Reader = bytes.NewReader(r.replacement)
	}
	return r.Reader.Seek(offset, whence)
}

func TestIncompleteChangingAndOverLimitInputsReturnNoPartialReport(t *testing.T) {
	data := fixture(t, false, false)
	limits := rowimport.RelationLimits{MaxRows: 10, KeyBytes: 1 << 20}
	for _, reader := range []*changingReader{
		{Reader: bytes.NewReader(data), replacement: data[:len(data)-1]},
		{Reader: bytes.NewReader(data), fail: true},
	} {
		r, err := Check(reader, checksum(data), limits)
		if err != ErrInvalid || !reflect.DeepEqual(r, Report{}) {
			t.Fatal("partial evidence returned after reader change")
		}
	}
	for _, bad := range []struct {
		data   []byte
		sha    string
		limits rowimport.RelationLimits
	}{
		{data, checksum([]byte("wrong")), limits},
		{data[:len(data)-1], checksum(data[:len(data)-1]), limits},
		{data, checksum(data), rowimport.RelationLimits{MaxRows: 1, KeyBytes: 1 << 20}},
		{data, checksum(data), rowimport.RelationLimits{MaxRows: 10, KeyBytes: 1}},
	} {
		r, err := Check(bytes.NewReader(bad.data), bad.sha, bad.limits)
		if err != ErrInvalid || !reflect.DeepEqual(r, Report{}) {
			t.Fatal("partial evidence returned for invalid input")
		}
	}
}
