package catalogimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/eurorefurb/retail/internal/rowimport"
)

func catalogStream(t *testing.T, categoryParents map[string]string) ([]byte, string) {
	t.Helper()
	data, err := os.ReadFile("../rowimport/field_plans.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tables []struct {
			Source string `json:"source"`
			Fields []struct {
				Source, Transform string
				Nullable          bool
			}
		}
	}
	if json.Unmarshal(data, &doc) != nil {
		t.Fatal("fixture layout")
	}
	var out bytes.Buffer
	emit := func(v any) {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		out.Write(b)
		out.WriteByte('\n')
	}
	h := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
	emit(map[string]any{"kind": "snapshot", "format_version": 2, "plan_sha256": hex.EncodeToString(h[:])})
	for _, table := range doc.Tables {
		emit(map[string]any{"kind": "table_begin", "table": table.Source})
		n := 0
		if table.Source == "product_category" {
			for id, parent := range categoryParents {
				cells := map[string]any{}
				for _, f := range table.Fields {
					if f.Nullable {
						cells[f.Source] = map[string]any{"sql_null": true}
						continue
					}
					var v any = "fixture"
					switch f.Transform {
					case "strict_boolean":
						v = false
					case "checked_int32", "checked_int64":
						v = 0
					case "utc_datetime6":
						v = "2026-10-31T00:00:00Z"
					}
					if f.Source == "id" || f.Source == "handle" {
						v = id
					}
					raw, _ := json.Marshal(v)
					cells[f.Source] = map[string]any{"sql_null": false, "value_json": string(raw)}
				}
				if parent != "" {
					raw, _ := json.Marshal(parent)
					cells["parent_category_id"] = map[string]any{"sql_null": false, "value_json": string(raw)}
				}
				emit(map[string]any{"format_version": 2, "table": table.Source, "cells": cells})
				n++
			}
		}
		emit(map[string]any{"kind": "table_end", "table": table.Source, "rows": n})
	}
	emit(map[string]any{"kind": "snapshot_complete", "tables": 159})
	h = sha256.Sum256(out.Bytes())
	return out.Bytes(), hex.EncodeToString(h[:])
}

func TestCatalogPreparationVerifiesCompleteInputAndSortsParents(t *testing.T) {
	data, sha := catalogStream(t, map[string]string{"child": "root", "root": ""})
	p, err := PrepareCatalog(context.Background(), bytes.NewReader(data), sha, 100, 1<<20)
	if err != nil {
		t.Fatal("complete catalog rejected")
	}
	r := p.Report()
	if r.Tables != 46 || r.Rows != 2 || r.SourceRows != 2 || r.Applied || r.CompleteImportAccepted {
		t.Fatal("scope or acceptance incorrect")
	}
	if string(p.rows["product_category"][0]["id"].Raw) != `"root"` {
		t.Fatal("self parent must precede child")
	}
	position := map[string]int{}
	for i, name := range p.order {
		position[name] = i
	}
	if position["stock_location_address"] >= position["stock_location"] {
		t.Fatal("cross-table parents not sorted")
	}
	for _, invalid := range [][]byte{data[:len(data)-1], append(append([]byte{}, data...), []byte("extra\n")...)} {
		if p, err := PrepareCatalog(context.Background(), bytes.NewReader(invalid), sha, 100, 1<<20); err == nil || p != nil {
			t.Fatal("partial or changed stream accepted")
		}
	}
	for _, limits := range [][2]int64{{1, 1 << 20}, {100, 1}} {
		if p, err := PrepareCatalog(context.Background(), bytes.NewReader(data), sha, limits[0], limits[1]); err == nil || p != nil {
			t.Fatal("unbounded retained data")
		}
	}
}

func TestCatalogRejectsMissingParentsAndSelfCycles(t *testing.T) {
	for _, parents := range []map[string]string{{"child": "missing"}, {"child": "root", "root": "child"}} {
		data, sha := catalogStream(t, parents)
		if p, err := PrepareCatalog(context.Background(), bytes.NewReader(data), sha, 100, 1<<20); err == nil || p != nil {
			t.Fatal("invalid closure accepted")
		}
	}
	if (&CatalogSnapshot{}).Stage(context.Background(), nil) != ErrPolicy {
		t.Fatal("empty stage accepted")
	}
	var tx *CatalogSnapshot
	if tx.Stage(context.Background(), nil) != ErrPolicy {
		t.Fatal("nil stage accepted")
	}
}

func TestCompiledLayoutsAreIndependentCopies(t *testing.T) {
	a, err := rowimport.Layouts()
	if err != nil {
		t.Fatal(err)
	}
	original := a[0].Columns[0].Target
	a[0].Columns[0].Target = "mutated"
	a[0].Primary[0] = "mutated"
	b, err := rowimport.Layouts()
	if err != nil || b[0].Columns[0].Target != original || b[0].Primary[0] == "mutated" {
		t.Fatal("compiled layout mutated")
	}
}
