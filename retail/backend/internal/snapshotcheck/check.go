// Package snapshotcheck provides offline diagnostics, never write acceptance.
package snapshotcheck

import (
	"errors"
	"io"

	"github.com/eurorefurb/retail/internal/rowimport"
)

var ErrInvalid = errors.New("snapshot diagnostic input invalid or incomplete")

type Counts struct {
	Tables        int   `json:"tables"`
	Rows          int64 `json:"rows"`
	Fields        int64 `json:"fields"`
	SQLNulls      int64 `json:"sql_null_fields"`
	OrdinaryRows  int64 `json:"ordinary_convertible_rows"`
	ProtectedRows int64 `json:"protected_rows"`
	HistoryRows   int64 `json:"history_policy_rows"`
	InvalidRows   int64 `json:"invalid_rows"`
}

type Report struct {
	Version                        int                        `json:"format_version"`
	SnapshotSHA256                 string                     `json:"snapshot_sha256"`
	Counts                         Counts                     `json:"counts"`
	Relations                      rowimport.RelationEvidence `json:"relations"`
	ScalarAndReferenceChecksPassed bool                       `json:"scalar_and_reference_checks_passed"`
	WritesEnabled                  bool                       `json:"writes_enabled"`
	CompleteImportAccepted         bool                       `json:"complete_import_accepted"`
}

// Check verifies the complete file twice, with identical expected digests.
// Reports contain aggregate counts and fixed schema names, never source IDs or
// values. No partial report is returned for framing/hash/reader/limit failures.
// Even a clean diagnostic cannot accept protection, history, business semantics,
// files, schema or a complete import.
func Check(input io.ReadSeeker, expectedSHA string, limits rowimport.RelationLimits) (Report, error) {
	if input == nil || limits.MaxRows <= 0 || limits.KeyBytes <= 0 {
		return Report{}, ErrInvalid
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return Report{}, ErrInvalid
	}
	counts := Counts{}
	verified, err := rowimport.VerifyStream(input, expectedSHA, func(table string, row rowimport.SourceRow) error {
		if counts.Rows >= limits.MaxRows {
			return ErrInvalid
		}
		v, err := rowimport.InspectValues(table, row)
		if err != nil {
			return err
		}
		counts.Rows++
		counts.Fields += int64(v.Fields)
		counts.SQLNulls += int64(v.SQLNulls)
		if v.OrdinaryConvertible {
			if _, err := rowimport.Convert(table, row); err != nil {
				return err
			}
			counts.OrdinaryRows++
		}
		if len(v.ProtectedColumns) > 0 {
			counts.ProtectedRows++
		}
		if v.RequiresHistoryPolicy {
			counts.HistoryRows++
		}
		if len(v.InvalidColumns) > 0 || !v.NumberPairsValid || !v.SourceChecksValid {
			counts.InvalidRows++
		}
		return nil
	})
	if err != nil {
		return Report{}, ErrInvalid
	}
	counts.Tables = len(verified.Tables)
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return Report{}, ErrInvalid
	}
	relations, err := rowimport.VerifyRelationships(input, expectedSHA, limits)
	if err != nil && !errors.Is(err, rowimport.ErrReferences) {
		return Report{}, ErrInvalid
	}
	if relations.Snapshot.SHA256 != verified.SHA256 || relations.Relations.Rows != counts.Rows {
		return Report{}, ErrInvalid
	}
	return Report{Version: 1, SnapshotSHA256: verified.SHA256, Counts: counts, Relations: relations.Relations,
		ScalarAndReferenceChecksPassed: err == nil && counts.InvalidRows == 0}, nil
}
