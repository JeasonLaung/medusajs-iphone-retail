package historyimport

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

func deviceAuditFacts() map[string][]rowimport.SourceRow {
	text := func(v string) rowimport.Cell { b, _ := json.Marshal(v); return rowimport.Cell{Raw: b} }
	device := rowimport.SourceRow{"serial_number": text("SN-ONE"), "imei": text("123456789012345"), "imei2": {SQLNull: true}, "eid": {SQLNull: true}, "variant_id": text("variant"), "deleted_at": {SQLNull: true}, "photos": {Raw: []byte(`[]`)}}
	product := rowimport.SourceRow{"id": text("product"), "title": text("iPhone"), "handle": text("iphone"), "type_id": {SQLNull: true}, "metadata": {Raw: []byte(`{"requires_imei":true}`)}}
	variant := rowimport.SourceRow{"id": text("variant"), "product_id": text("product")}
	return map[string][]rowimport.SourceRow{"device": {device}, "product": {product}, "product_variant": {variant}, "product_type": nil}
}

func TestDeviceAuditPreservesIdentitiesAndProductPrecedence(t *testing.T) {
	facts := deviceAuditFacts()
	before, _ := json.Marshal(facts)
	r, e := auditDeviceRows(facts)
	after, _ := json.Marshal(facts)
	if e != nil || r.Active != 1 || r.InvalidActiveIdentities != 0 || r.NonCanonicalActiveIdentities != 0 || r.DuplicateActiveIdentities != 0 || !bytes.Equal(before, after) {
		t.Fatal("canonical source audit changed facts")
	}
	facts["device"][0]["serial_number"] = rowimport.Cell{Raw: []byte(`" sn-one "`)}
	r, e = auditDeviceRows(facts)
	if e != nil || r.NonCanonicalActiveIdentities != 1 {
		t.Fatal("noncanonical source silently accepted")
	}
	facts = deviceAuditFacts()
	facts["device"][0]["imei"] = rowimport.Cell{SQLNull: true}
	r, e = auditDeviceRows(facts)
	if e != nil || r.InvalidActiveIdentities != 1 {
		t.Fatal("product radio requirement ignored")
	}
	facts["product"][0]["metadata"] = rowimport.Cell{Raw: []byte(`{"requires_imei":false}`)}
	r, e = auditDeviceRows(facts)
	if e != nil || r.InvalidActiveIdentities != 0 {
		t.Fatal("explicit product false must override title")
	}
	facts["device"][0]["deleted_at"] = rowimport.Cell{Raw: []byte(`"2026-01-01T00:00:00Z"`)}
	facts["device"][0]["serial_number"] = rowimport.Cell{SQLNull: true}
	r, e = auditDeviceRows(facts)
	if e != nil || r.Deleted != 1 || r.Active != 0 || r.InvalidActiveIdentities != 0 {
		t.Fatal("deleted history was normalized")
	}
}

func TestDeviceAuditCrossFieldCollisionsAndPhotos(t *testing.T) {
	facts := deviceAuditFacts()
	other := deviceAuditFacts()["device"][0]
	other["serial_number"] = rowimport.Cell{Raw: []byte(`"SN-TWO"`)}
	other["imei"] = rowimport.Cell{SQLNull: true}
	other["imei2"] = facts["device"][0]["imei"]
	facts["device"] = append(facts["device"], other)
	r, e := auditDeviceRows(facts)
	if e != nil || r.DuplicateActiveIdentities != 1 {
		t.Fatal("cross-field radio collision missed")
	}
	facts = deviceAuditFacts()
	facts["device"][0]["photos"] = rowimport.Cell{Raw: []byte(`["legacy-private-key"]`)}
	r, e = auditDeviceRows(facts)
	if e != nil || r.PhotoReferences != 1 {
		t.Fatal("file audit requirement missed")
	}
	for _, invalid := range []string{`null`, ` null `, `{}`, `"[]"`} {
		facts["device"][0]["photos"] = rowimport.Cell{Raw: []byte(invalid)}
		if _, e = auditDeviceRows(facts); e == nil {
			t.Fatal("nonarray photo source accepted")
		}
	}
	facts = deviceAuditFacts()
	facts["product"] = nil
	r, e = auditDeviceRows(facts)
	if e != nil || r.MissingProductFacts != 1 {
		t.Fatal("missing product silently replaced")
	}
}

func TestDeviceAuditRequiresCompleteStream(t *testing.T) {
	data, sha := emptySnapshot(t)
	r, e := AuditDeviceHistory(t.Context(), bytes.NewReader(data), sha, 100, 1<<20)
	if e != nil || r.Devices != 0 || r.CompleteImportAccepted {
		t.Fatal("empty full snapshot audit")
	}
	for _, bad := range [][]byte{data[:len(data)-1], append(append([]byte{}, data...), data...)} {
		hash := sha256.Sum256(bad)
		if _, e = AuditDeviceHistory(t.Context(), bytes.NewReader(bad), hex.EncodeToString(hash[:]), 100, 1<<20); e == nil {
			t.Fatal("invalid framing accepted even with matching digest")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e = AuditDeviceHistory(ctx, bytes.NewReader(data), sha, 100, 1<<20); e == nil {
		t.Fatal("canceled audit accepted")
	}
}

func TestNativeDeviceHistorySourceAudit(t *testing.T) {
	if os.Getenv("DEVICE_HISTORY_SOURCE_REHEARSAL") != "1" {
		t.Skip("explicit private source required")
	}
	f, e := os.Open(os.Getenv("NATIVE_HISTORY_SNAPSHOT"))
	if e != nil {
		t.Fatal("private source unavailable")
	}
	defer f.Close()
	r, e := AuditDeviceHistory(t.Context(), f, os.Getenv("NATIVE_HISTORY_SHA256"), 100000, 64<<20)
	if e != nil || r.Devices != 52 || r.Active != 7 || r.Deleted != 45 || r.Assignments != 1 || r.Events != 50 || r.AfterSalesRequests != 1 || r.InvalidActiveIdentities != 0 || r.NonCanonicalActiveIdentities != 0 || r.DuplicateActiveIdentities != 0 || r.PhotoReferences != 0 || r.InvalidAssignmentRelations != 0 || r.InvalidAfterSalesLines != 0 || r.IdentityImportReady || r.CompleteImportAccepted {
		t.Fatal("pinned native source audit changed")
	}
	if len(r.MissingReferences) != 2 || r.MissingReferences["device.source_id"] != 44 || r.MissingReferences["device.location_id"] != 11 || r.MissingOnDeletedRows["device.source_id"] != 44 || r.MissingOnDeletedRows["device.location_id"] != 11 || len(r.CompiledHistoryPolicyMissing) != 1 || r.CompiledHistoryPolicyMissing["device.source_id"] != 44 {
		t.Fatal("unresolved device policy must stay explicit")
	}
	encoded, _ := json.Marshal(r)
	t.Logf("DEVICE_HISTORY_SOURCE_AUDIT %s", encoded)
	if _, e = AuditDeviceHistory(t.Context(), f, os.Getenv("NATIVE_HISTORY_SHA256"), 1, 64<<20); e == nil {
		t.Fatal("source row budget ignored")
	}
	if _, e = AuditDeviceHistory(t.Context(), f, os.Getenv("NATIVE_HISTORY_SHA256"), 100000, 1); e == nil {
		t.Fatal("source payload budget ignored")
	}
}

func TestDeviceAuditKeepsHistoricalOrderMembership(t *testing.T) {
	cell := func(v string) rowimport.Cell { raw, _ := json.Marshal(v); return rowimport.Cell{Raw: raw} }
	assignment := rowimport.SourceRow{"id": cell("assignment"), "order_id": cell("order"), "item_id": cell("item"), "deleted_at": {SQLNull: true}, "replacement_of": {SQLNull: true}, "after_sales_request_id": {SQLNull: true}}
	membership := rowimport.SourceRow{"order_id": cell("order"), "item_id": cell("item"), "version": {Raw: []byte(`1`)}}
	tables := map[string][]rowimport.SourceRow{"device_assignment": {assignment}, "order_item": {membership}}
	index := map[string]map[string]rowimport.SourceRow{"device_assignment": {"assignment": assignment}, "after_sales_request": {}}
	var r DeviceAudit
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil || r.InvalidAssignmentRelations != 0 {
		t.Fatal("historical order membership ignored")
	}
	tables["order_item"] = nil
	r = DeviceAudit{}
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil || r.InvalidAssignmentRelations != 1 {
		t.Fatal("foreign order line accepted")
	}
	tables["order_item"] = []rowimport.SourceRow{membership}
	assignment["replacement_of"] = cell("assignment")
	r = DeviceAudit{}
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil || r.InvalidAssignmentRelations != 1 {
		t.Fatal("self replacement accepted")
	}
	assignment["replacement_of"] = rowimport.Cell{SQLNull: true}
	assignment["after_sales_request_id"] = cell("request")
	index["after_sales_request"]["request"] = rowimport.SourceRow{"order_id": cell("another-order")}
	r = DeviceAudit{}
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil || r.InvalidAssignmentRelations != 1 {
		t.Fatal("cross-order after-sales request accepted")
	}
	tables["device_assignment"] = nil
	request := rowimport.SourceRow{"order_id": cell("order"), "deleted_at": {SQLNull: true}, "items": {Raw: []byte(`{"lines":[{"item_id":"item"},{"item_id":"item"}]}`)}}
	tables["after_sales_request"] = []rowimport.SourceRow{request}
	r = DeviceAudit{}
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil || r.InvalidAfterSalesLines != 1 {
		t.Fatal("duplicate after-sales line accepted")
	}
	request["items"] = rowimport.Cell{Raw: []byte(`{"lines":[{"item_id":"foreign-item"}]}`)}
	r = DeviceAudit{}
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil || r.InvalidAfterSalesLines != 1 {
		t.Fatal("foreign after-sales line accepted")
	}
	request["items"] = rowimport.Cell{Raw: []byte(`{"lines":null}`)}
	r = DeviceAudit{}
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil || r.InvalidAfterSalesLines != 1 {
		t.Fatal("missing after-sales envelope accepted")
	}
}
