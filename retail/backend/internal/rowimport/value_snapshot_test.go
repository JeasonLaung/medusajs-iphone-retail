package rowimport

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPinnedV2ValueCoverage(t *testing.T) {
	path := os.Getenv("ROWIMPORT_VALUE_SNAPSHOT")
	if path == "" {
		t.Skip("explicit private snapshot required")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("private value snapshot unavailable")
	}
	defer file.Close()
	const hash = "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
	type tableCount struct {
		Rows                int64            `json:"rows"`
		Fields              int64            `json:"fields"`
		SQLNulls            int64            `json:"sql_nulls"`
		OrdinaryConvertible int64            `json:"ordinary_convertible_rows"`
		ProtectedRows       int64            `json:"protected_rows"`
		HistoryPolicyRows   int64            `json:"history_policy_rows"`
		InvalidRows         int64            `json:"invalid_rows"`
		InvalidColumns      map[string]int64 `json:"invalid_columns"`
		ProtectedColumns    map[string]int64 `json:"protected_columns"`
		NumberPairFailures  int64            `json:"number_pair_failures"`
		SourceCheckFailures int64            `json:"source_check_failures"`
	}
	plans, _ := compiledPlans()
	counts := map[string]*tableCount{}
	for table := range plans {
		counts[table] = &tableCount{InvalidColumns: map[string]int64{}, ProtectedColumns: map[string]int64{}}
	}
	rows, ordinary, invalid := int64(0), int64(0), int64(0)
	verified, err := VerifyStream(file, hash, func(table string, row SourceRow) error {
		v, err := InspectValues(table, row)
		if err != nil {
			return err
		}
		c := counts[table]
		c.Rows++
		c.Fields += int64(v.Fields)
		c.SQLNulls += int64(v.SQLNulls)
		rows++
		if v.OrdinaryConvertible {
			bound, err := Convert(table, row)
			if err != nil || len(bound.Columns) != v.Fields {
				return ErrRow
			}
			c.OrdinaryConvertible++
			ordinary++
		}
		if len(v.ProtectedColumns) > 0 {
			c.ProtectedRows++
		}
		if v.RequiresHistoryPolicy {
			c.HistoryPolicyRows++
		}
		for _, column := range v.ProtectedColumns {
			c.ProtectedColumns[column]++
		}
		for _, column := range v.InvalidColumns {
			c.InvalidColumns[column]++
		}
		if !v.NumberPairsValid {
			c.NumberPairFailures++
		}
		if !v.SourceChecksValid {
			c.SourceCheckFailures++
		}
		if len(v.InvalidColumns) > 0 || !v.NumberPairsValid || !v.SourceChecksValid {
			c.InvalidRows++
			invalid++
		}
		return nil
	})
	if err != nil || len(verified.Tables) != 159 || rows != 27140 {
		t.Fatal("complete value audit stream invalid")
	}
	encoded, _ := json.Marshal(map[string]any{"tables": counts, "source_rows": rows, "ordinary_convertible_rows": ordinary, "invalid_rows": invalid, "source_verified_tables": len(verified.Tables), "snapshot_sha256": hash})
	t.Log("VALUE_COVERAGE_AUDIT " + string(encoded))
}
