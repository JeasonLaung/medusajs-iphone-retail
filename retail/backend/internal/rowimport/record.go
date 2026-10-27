package rowimport

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/eurorefurb/retail/internal/importvalue"
)

// Each source cell embeds its exact PostgreSQL JSON rendering as a STRING,
// never as a decoded numeric value. v1 object snapshots are rejected: nulls in
// that format cannot prove SQL NULL versus JSON document null.
const MaxRecordBytes = 16 << 20

func object(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrRow
	}
	values := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrRow
		}
		key, ok := token.(string)
		if !ok {
			return nil, ErrRow
		}
		if _, exists := values[key]; exists {
			return nil, ErrRow
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, ErrRow
		}
		values[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrRow
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, ErrRow
	}
	return values, nil
}

// DecodeRecord validates the full v2 row structure and duplicate keys before
// handing cells to Convert. It performs no SQL and returns no partial row. A
// caller must retain record/source checksums and commit only after verifying
// the complete input; successful decoding is not complete snapshot acceptance.
func DecodeRecord(record []byte) (string, SourceRow, error) {
	if len(record) > MaxRecordBytes {
		return "", nil, ErrRow
	}
	if _, err := importvalue.JSONText(record); err != nil {
		return "", nil, ErrRow
	}
	root, err := object(record)
	if err != nil || len(root) != 3 {
		return "", nil, ErrRow
	}
	version, err := importvalue.Integer(root["format_version"], 32)
	if err != nil || version != 2 {
		return "", nil, ErrRow
	}
	table, err := importvalue.Text(root["table"], 191)
	if err != nil {
		return "", nil, ErrRow
	}
	plans, err := compiledPlans()
	if err != nil {
		return "", nil, ErrRow
	}
	plan, exists := plans[table]
	if !exists {
		return "", nil, ErrRow
	}
	rawCells, err := object(root["cells"])
	if err != nil || len(rawCells) != len(plan.Fields) {
		return "", nil, ErrRow
	}
	row := SourceRow{}
	for _, f := range plan.Fields {
		raw, exists := rawCells[f.Source]
		if !exists {
			return "", nil, ErrRow
		}
		cell, err := object(raw)
		if err != nil {
			return "", nil, ErrRow
		}
		isNull, err := importvalue.Boolean(cell["sql_null"])
		if err != nil {
			return "", nil, ErrRow
		}
		if isNull {
			if len(cell) != 1 || !f.Nullable {
				return "", nil, ErrRow
			}
			row[f.Source] = Cell{SQLNull: true}
		} else {
			if len(cell) != 2 {
				return "", nil, ErrRow
			}
			value, err := importvalue.Text(cell["value_json"], 0)
			if err != nil {
				return "", nil, ErrRow
			}
			if _, err := importvalue.JSONText(json.RawMessage(value)); err != nil {
				return "", nil, ErrRow
			}
			row[f.Source] = Cell{Raw: json.RawMessage(value)}
		}
	}
	return table, row, nil
}
