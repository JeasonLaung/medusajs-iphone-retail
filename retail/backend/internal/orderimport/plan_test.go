package orderimport

import (
	"testing"
)

func TestReviewedOrderScope(t *testing.T) {
	p, err := plans()
	if err != nil {
		t.Fatal("order plan unavailable")
	}
	columns, deferred := 0, 0
	for _, table := range p.Tables {
		columns += len(table.Columns)
		for _, c := range table.Columns {
			if c.Deferred {
				deferred++
				if table.Source != "order_line_item" && table.Source != "return_reason" {
					t.Fatal("unreviewed deferred reference")
				}
			}
		}
	}
	if columns != 378 || deferred != 2 {
		t.Fatal("reviewed order scope differs")
	}
	names, _ := SourceTables()
	first := names[0]
	names[0] = "customer"
	next, _ := SourceTables()
	if len(next) != 28 || next[0] != first {
		t.Fatal("caller mutated compiled plan")
	}
	if _, err := Import(t.Context(), nil, "", nil); err != ErrPolicy {
		t.Fatal("missing transaction accepted")
	}
}
