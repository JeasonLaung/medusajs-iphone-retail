package historyimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/rowimport"
)

func emptySnapshot(t *testing.T) ([]byte, string) {
	t.Helper()
	data, err := os.ReadFile("../rowimport/field_plans.json")
	if err != nil {
		t.Fatal(err)
	}
	var plans struct{ Tables []struct{ Source string } }
	if json.Unmarshal(data, &plans) != nil {
		t.Fatal("fixture layout")
	}
	var b bytes.Buffer
	hash := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
	fmt.Fprintf(&b, "{\"kind\":\"snapshot\",\"format_version\":2,\"plan_sha256\":%q}\n", hex.EncodeToString(hash[:]))
	for _, p := range plans.Tables {
		fmt.Fprintf(&b, "{\"kind\":\"table_begin\",\"table\":%q}\n", p.Source)
		fmt.Fprintf(&b, "{\"kind\":\"table_end\",\"table\":%q,\"rows\":0}\n", p.Source)
	}
	b.WriteString("{\"kind\":\"snapshot_complete\",\"tables\":159}\n")
	hash = sha256.Sum256(b.Bytes())
	return b.Bytes(), hex.EncodeToString(hash[:])
}

func TestCompletePreparationAndBoundaries(t *testing.T) {
	data, sha := emptySnapshot(t)
	p, err := Prepare(t.Context(), bytes.NewReader(data), sha, 100, 1<<20)
	if err != nil || len(p.orders) != 28 || len(p.inventory) != 2 || p.Report().CompleteImportAccepted {
		t.Fatal("complete empty scope")
	}
	report := p.Report()
	report.Catalog.OutsideMissing["private"] = 1
	if len(p.Report().Catalog.OutsideMissing) != 0 {
		t.Fatal("report aliases policy")
	}
	for _, input := range [][]byte{data[:len(data)-1], data[:len(data)/2], append(append([]byte{}, data...), data...)} {
		if _, err = Prepare(t.Context(), bytes.NewReader(input), sha, 100, 1<<20); err == nil {
			t.Fatal("unverified stream accepted")
		}
	}
	if _, err = Prepare(t.Context(), bytes.NewReader(data), strings.Repeat("0", 64), 100, 1<<20); err == nil {
		t.Fatal("wrong hash accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Empty streams have no callbacks; canceled work must still fail at entry.
	if _, err = Prepare(ctx, bytes.NewReader(data), sha, 100, 1<<20); err == nil {
		t.Fatal("canceled plan accepted")
	}
	if _, err = p.Stage(t.Context(), nil, nil); err == nil {
		t.Fatal("missing transaction/keyring accepted")
	}
	if _, err = p.StageWithPayments(t.Context(), nil, nil, nil); err == nil {
		t.Fatal("missing payment keyring accepted")
	}
	if _, err = p.StageLogistics(t.Context(), nil); err == nil {
		t.Fatal("missing logistics transaction accepted")
	}
	if _, err = p.StageCarts(t.Context(), nil); err == nil {
		t.Fatal("missing cart transaction accepted")
	}
}

func TestOnlyDeletedReservationLogicalGapsArePreserved(t *testing.T) {
	for _, sample := range []struct {
		name                     string
		missing, deleted, policy int64
		physical                 bool
		ok                       bool
	}{
		{"reservation_item.line_item_id", 72, 72, 72, false, true},
		{"reservation_item.line_item_id", 72, 71, 71, false, false},
		{"reservation_item.line_item_id", 72, 72, 0, false, false},
		{"reservation_item.location_id", 1, 1, 1, false, false},
		{"inventory_level.inventory_item_id", 1, 1, 1, false, false},
		{"reservation_item.line_item_id", 72, 72, 72, true, false},
		{"order_fulfillment.order_id", 72, 0, 0, false, true},
	} {
		r := rowimport.RelationEvidence{Missing: map[string]int64{sample.name: sample.missing}, MissingOnDeletedRows: map[string]int64{sample.name: sample.deleted}, HistoryPolicyMissing: map[string]int64{sample.name: sample.policy}, PhysicalMissing: map[string]int64{}}
		if sample.physical {
			r.PhysicalMissing["fixture_fk"] = 1
		}
		_, err := reviewInventoryReferences(r)
		if (err == nil) != sample.ok {
			t.Fatalf("reference boundary %s", sample.name)
		}
	}
}
