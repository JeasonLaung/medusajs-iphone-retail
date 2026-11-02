package historyimport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
)

// DeviceAudit contains aggregates only, never SN/IMEI, addresses or actor IDs.
// This read-only audit neither relaxes compiled reference policy nor grants
// permission to restore a device or claim its identity.
type DeviceAudit struct {
	SnapshotSHA                  string           `json:"snapshot_sha256"`
	Devices                      int              `json:"devices"`
	Active                       int              `json:"active_devices"`
	Deleted                      int              `json:"deleted_devices"`
	Assignments                  int              `json:"assignments"`
	Events                       int              `json:"events"`
	AfterSalesRequests           int              `json:"after_sales_requests"`
	InvalidActiveIdentities      int              `json:"invalid_active_identities"`
	NonCanonicalActiveIdentities int              `json:"noncanonical_active_identities"`
	DuplicateActiveIdentities    int              `json:"duplicate_active_identities"`
	MissingProductFacts          int              `json:"missing_product_facts"`
	InvalidAssignmentRelations   int              `json:"invalid_assignment_relations"`
	InvalidAfterSalesLines       int              `json:"invalid_after_sales_lines"`
	PhotoReferences              int              `json:"photo_references"`
	MissingReferences            map[string]int64 `json:"missing_references"`
	MissingOnDeletedRows         map[string]int64 `json:"missing_on_deleted_rows"`
	CompiledHistoryPolicyMissing map[string]int64 `json:"compiled_history_policy_missing"`
	IdentityImportReady          bool             `json:"identity_import_ready"`
	CompleteImportAccepted       bool             `json:"complete_import_accepted"`
}

// AuditDeviceHistory verifies the entire pinned stream and relationship audit
// before examining device identities against verified product metadata. Limits
// bound source row count and retained cell payload, not total Go heap. No SQL,
// environment, network, private file reads or source normalization is performed.
func AuditDeviceHistory(ctx context.Context, input io.ReadSeeker, sha string, maxRows, maxBytes int64) (DeviceAudit, error) {
	if ctx.Err() != nil || input == nil || maxRows < 1 || maxBytes < 1 {
		return DeviceAudit{}, ErrPolicy
	}
	tables := map[string][]rowimport.SourceRow{"device": nil, "device_assignment": nil, "device_event": nil, "after_sales_request": nil, "product": nil, "product_variant": nil, "product_type": nil, "order_item": nil}
	if _, e := input.Seek(0, io.SeekStart); e != nil {
		return DeviceAudit{}, ErrPolicy
	}
	var total, retained int64
	_, err := rowimport.VerifyStream(input, sha, func(name string, row rowimport.SourceRow) error {
		total++
		if ctx.Err() != nil || total > maxRows {
			return ErrPolicy
		}
		if _, keep := tables[name]; !keep {
			return nil
		}
		for k, c := range row {
			retained += int64(len(k) + len(c.Raw) + 1)
			if retained > maxBytes {
				return ErrPolicy
			}
		}
		if _, e := rowimport.Convert(name, row); e != nil {
			return ErrPolicy
		}
		tables[name] = append(tables[name], row)
		return nil
	})
	if err != nil {
		return DeviceAudit{}, ErrPolicy
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return DeviceAudit{}, ErrPolicy
	}
	relations, err := rowimport.VerifyRelationships(input, sha, rowimport.RelationLimits{MaxRows: maxRows, KeyBytes: maxBytes})
	if (err != nil && !errors.Is(err, rowimport.ErrReferences)) || len(relations.Relations.PhysicalMissing) != 0 || ctx.Err() != nil {
		return DeviceAudit{}, ErrPolicy
	}
	report, err := auditDeviceRows(tables)
	if err != nil {
		return DeviceAudit{}, err
	}
	report.SnapshotSHA = sha
	for name, n := range relations.Relations.Missing {
		source := strings.SplitN(name, ".", 2)[0]
		if source != "device" && source != "device_assignment" && source != "device_event" && source != "after_sales_request" {
			continue
		}
		report.MissingReferences[name] = n
		if n := relations.Relations.MissingOnDeletedRows[name]; n != 0 {
			report.MissingOnDeletedRows[name] = n
		}
		if n := relations.Relations.HistoryPolicyMissing[name]; n != 0 {
			report.CompiledHistoryPolicyMissing[name] = n
		}
	}
	report.IdentityImportReady = report.InvalidActiveIdentities == 0 && report.NonCanonicalActiveIdentities == 0 && report.DuplicateActiveIdentities == 0 && report.MissingProductFacts == 0 && report.PhotoReferences == 0 && report.InvalidAssignmentRelations == 0 && report.InvalidAfterSalesLines == 0 && len(report.MissingReferences) == 0
	return report, nil
}

func sourceText(row rowimport.SourceRow, key string) (string, error) {
	c, ok := row[key]
	if !ok {
		return "", ErrPolicy
	}
	if c.SQLNull {
		return "", nil
	}
	v, e := importvalue.Text(c.Raw, 0)
	if e != nil {
		return "", ErrPolicy
	}
	return v, nil
}

func auditDeviceRows(tables map[string][]rowimport.SourceRow) (DeviceAudit, error) {
	r := DeviceAudit{Devices: len(tables["device"]), Assignments: len(tables["device_assignment"]), Events: len(tables["device_event"]), AfterSalesRequests: len(tables["after_sales_request"]), MissingReferences: map[string]int64{}, MissingOnDeletedRows: map[string]int64{}, CompiledHistoryPolicyMissing: map[string]int64{}}
	index := map[string]map[string]rowimport.SourceRow{}
	for _, name := range []string{"product", "product_variant", "product_type", "device_assignment", "after_sales_request"} {
		index[name] = map[string]rowimport.SourceRow{}
		for _, row := range tables[name] {
			id, e := sourceText(row, "id")
			if e != nil || id == "" {
				return DeviceAudit{}, ErrPolicy
			}
			if _, exists := index[name][id]; exists {
				return DeviceAudit{}, ErrPolicy
			}
			index[name][id] = row
		}
	}
	serials, radios := map[string]bool{}, map[string]bool{}
	for _, row := range tables["device"] {
		var photos []json.RawMessage
		cell, ok := row["photos"]
		if !ok || cell.SQLNull || json.Unmarshal(cell.Raw, &photos) != nil || len(bytes.TrimSpace(cell.Raw)) == 0 || bytes.TrimSpace(cell.Raw)[0] != '[' {
			return DeviceAudit{}, ErrPolicy
		}
		r.PhotoReferences += len(photos)
		if !row["deleted_at"].SQLNull {
			r.Deleted++
			continue
		}
		r.Active++
		var identity devices.Identity
		for name, dest := range map[string]*string{"serial_number": &identity.SerialNumber, "imei": &identity.IMEI, "imei2": &identity.IMEI2, "eid": &identity.EID} {
			v, e := sourceText(row, name)
			if e != nil {
				return DeviceAudit{}, e
			}
			*dest = v
		}
		variantID, e := sourceText(row, "variant_id")
		if e != nil {
			return DeviceAudit{}, e
		}
		variant := index["product_variant"][variantID]
		productID, e := sourceText(variant, "product_id")
		if variant == nil || e != nil {
			r.MissingProductFacts++
			continue
		}
		product := index["product"][productID]
		if product == nil {
			r.MissingProductFacts++
			continue
		}
		title, e := sourceText(product, "title")
		if e != nil {
			return DeviceAudit{}, e
		}
		handle, e := sourceText(product, "handle")
		if e != nil {
			return DeviceAudit{}, e
		}
		typeID, e := sourceText(product, "type_id")
		if e != nil {
			return DeviceAudit{}, e
		}
		productType := ""
		if typeID != "" {
			typ := index["product_type"][typeID]
			if typ == nil {
				r.MissingProductFacts++
				continue
			}
			productType, e = sourceText(typ, "value")
			if e != nil {
				return DeviceAudit{}, e
			}
		}
		metadata := map[string]any{}
		if c := product["metadata"]; !c.SQLNull && json.Unmarshal(c.Raw, &metadata) != nil {
			return DeviceAudit{}, ErrPolicy
		}
		canonical, e := devices.NormalizeIdentity(identity, devices.RequiresRadio(metadata, title, handle, productType))
		if e != nil {
			r.InvalidActiveIdentities++
			continue
		}
		if canonical != identity {
			r.NonCanonicalActiveIdentities++
		}
		if serials[canonical.SerialNumber] {
			r.DuplicateActiveIdentities++
		}
		serials[canonical.SerialNumber] = true
		for _, value := range []string{canonical.IMEI, canonical.IMEI2, canonical.EID} {
			if value == "" {
				continue
			}
			if radios[value] {
				r.DuplicateActiveIdentities++
			}
			radios[value] = true
		}
	}
	if e := auditDeviceBusinessLinks(tables, index, &r); e != nil {
		return DeviceAudit{}, e
	}
	return r, nil
}

// Preserve the original source semantic audit: membership comes from order_item
// across versions, not from guessing the latest order-line snapshot.
func auditDeviceBusinessLinks(tables map[string][]rowimport.SourceRow, index map[string]map[string]rowimport.SourceRow, r *DeviceAudit) error {
	members := map[[2]string]bool{}
	for _, row := range tables["order_item"] {
		order, e := sourceText(row, "order_id")
		if e != nil {
			return e
		}
		item, e := sourceText(row, "item_id")
		if e != nil {
			return e
		}
		members[[2]string{order, item}] = true
	}
	for _, row := range tables["device_assignment"] {
		if !row["deleted_at"].SQLNull {
			continue
		}
		id, e := sourceText(row, "id")
		if e != nil {
			return e
		}
		order, e := sourceText(row, "order_id")
		if e != nil {
			return e
		}
		item, e := sourceText(row, "item_id")
		if e != nil {
			return e
		}
		if !members[[2]string{order, item}] {
			r.InvalidAssignmentRelations++
		}
		previous, e := sourceText(row, "replacement_of")
		if e != nil {
			return e
		}
		if previous != "" {
			parent := index["device_assignment"][previous]
			parentOrder, e := sourceText(parent, "order_id")
			if parent == nil || e != nil || parentOrder != order || previous == id {
				r.InvalidAssignmentRelations++
			}
		}
		requestID, e := sourceText(row, "after_sales_request_id")
		if e != nil {
			return e
		}
		if requestID != "" {
			parent := index["after_sales_request"][requestID]
			parentOrder, e := sourceText(parent, "order_id")
			if parent == nil || e != nil || parentOrder != order {
				r.InvalidAssignmentRelations++
			}
		}
	}
	for _, row := range tables["after_sales_request"] {
		if !row["deleted_at"].SQLNull {
			continue
		}
		order, e := sourceText(row, "order_id")
		if e != nil {
			return e
		}
		var envelope struct {
			Lines *[]struct {
				ItemID string `json:"item_id"`
			} `json:"lines"`
		}
		if json.Unmarshal(row["items"].Raw, &envelope) != nil || envelope.Lines == nil {
			r.InvalidAfterSalesLines++
			continue
		}
		seen := map[string]bool{}
		for _, line := range *envelope.Lines {
			if line.ItemID == "" || seen[line.ItemID] || !members[[2]string{order, line.ItemID}] {
				r.InvalidAfterSalesLines++
			}
			seen[line.ItemID] = true
		}
	}
	return nil
}
