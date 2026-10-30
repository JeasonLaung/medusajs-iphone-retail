package identity_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// These commands use independent committed transactions in the owned artificial
// database, discarded by the runner. No outer transaction serializes the race.
func checkDeviceReplacementConcurrency(t *testing.T, db *gorm.DB, actor identity.User, assignment string) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	const order, request = "allocation-order", "asr-receive-exchange"
	before, err := h.ReadReplacement(ctx, actor, order, request, assignment)
	if err != nil || !before.Eligible || !before.WriteAvailable {
		t.Fatal("concurrent replacement preparation", err, before)
	}
	stock := func() string {
		t.Helper()
		var rows []map[string]any
		if err := db.Raw("SELECT * FROM inventory_levels ORDER BY id").Scan(&rows).Error; err != nil {
			t.Fatal("concurrent stock fingerprint", err)
		}
		data, _ := json.Marshal(rows)
		return string(data)
	}
	stockBefore := stock()
	count := func(table string) int64 {
		t.Helper()
		var n int64
		if err := db.Table(table).Count(&n).Error; err != nil {
			t.Fatal("concurrent replacement count", table, err)
		}
		return n
	}
	tables := map[string]int64{"order_exchanges": 1, "returns": 1, "return_items": 1, "order_changes": 2, "order_change_actions": 3, "order_exchange_items": 1, "order_line_items": 1, "order_items": 4, "order_summaries": 2, "device_assignments": 1, "device_service_receipts": 1, "device_events": 1, "payment_collections": 1, "order_payment_collections": 1, "order_transactions": 0, "payment_captures": 0, "payment_refunds": 0}
	baseline := map[string]int64{}
	for table := range tables {
		baseline[table] = count(table)
	}
	type result struct {
		input devices.ReplacementInput
		out   devices.ReplacementResult
		err   error
	}
	run := func(inputs []devices.ReplacementInput) []result {
		start := make(chan struct{})
		results := make(chan result, len(inputs))
		var wg sync.WaitGroup
		for _, input := range inputs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				out, err := h.ReplaceDevice(ctx, actor, order, request, assignment, input)
				results <- result{input, out, err}
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		var out []result
		for v := range results {
			out = append(out, v)
		}
		return out
	}
	inputs := make([]devices.ReplacementInput, 4)
	for i := range inputs {
		inputs[i] = devices.ReplacementInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, DeviceID: "allocation-c"}
	}
	var winner result
	wins, conflicts := 0, 0
	for _, r := range run(inputs) {
		if r.err == nil {
			wins++
			winner = r
			if r.out.Replayed {
				t.Fatal("independent replacement replayed")
			}
		} else if errors.Is(r.err, devices.ErrConflict) {
			conflicts++
		} else {
			t.Fatal("unexpected concurrent replacement error", r.err)
		}
	}
	if wins != 1 || conflicts != 3 || winner.out.OrderVersion != before.OrderVersion+2 {
		t.Fatal("independent replacement race", wins, conflicts, winner.out)
	}
	for table, delta := range tables {
		if actual := count(table); actual != baseline[table]+delta {
			t.Fatal("concurrent replacement duplicate/leak", table, actual, baseline[table], delta)
		}
	}
	// Recover a lost successful response with concurrent retries of its UUID.
	for _, r := range run([]devices.ReplacementInput{winner.input, winner.input, winner.input, winner.input}) {
		want := winner.out
		want.Replayed = true
		if r.err != nil || r.out != want {
			t.Fatal("concurrent replacement receipt recovery", r.err, r.out, want)
		}
	}
	for table, delta := range tables {
		if count(table) != baseline[table]+delta {
			t.Fatal("concurrent replay created data", table)
		}
	}
	if stock() != stockBefore {
		t.Fatal("replacement allocation changed quantity inventory")
	}
	var version int
	if db.Table("orders").Select("version").Where("id=?", order).Scan(&version).Error != nil || version != before.OrderVersion+2 {
		t.Fatal("concurrent persisted order version", version)
	}
	var nativeReturn string
	if db.Table("device_assignments").Select("return_id").Where("id=?", assignment).Scan(&nativeReturn).Error != nil || nativeReturn != winner.out.ReturnID {
		t.Fatal("concurrent original return binding")
	}
	var replacements int64
	if db.Table("device_assignments").Where("replacement_of=? AND after_sales_request_id=? AND item_id=? AND status='reserved' AND deleted_at IS NULL", assignment, request, winner.out.ItemID).Count(&replacements).Error != nil || replacements != 1 {
		t.Fatal("concurrent replacement new item binding")
	}
	t.Log(`DEVICE_REPLACEMENT_CONCURRENCY {"independent_mysql_transactions":true,"distinct_operations":4,"one_winner":true,"three_conflicts":true,"concurrent_receipt_replays":4,"immutable_result":true,"two_versions_only":true,"one_native_exchange_and_return":true,"one_assignment_event_and_receipt":true,"quantity_stock_unchanged":true}`)
}
