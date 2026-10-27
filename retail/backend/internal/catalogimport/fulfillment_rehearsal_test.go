package catalogimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/gin-gonic/gin"
)

// Compare every projected fulfillment field to independent, fixed SQL reads.
// Private values and identifiers must never enter test failure output.
func checkActualFulfillments(t *testing.T, ctx context.Context, db *sql.DB, router *gin.Engine, summary orders.Summary, counts map[string]int64) {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/orders/"+url.PathEscape(summary.ID)+"/fulfillments", nil))
	var data orders.FulfillmentEvidence
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &data) != nil || data.OrderID != summary.ID || data.Version != summary.Version || data.OperationsEnabled {
		t.Fatal("actual fulfillment HTTP envelope differs")
	}
	links, err := db.QueryContext(ctx, "SELECT fulfillment_id FROM order_fulfillments WHERE order_id=? AND deleted_at IS NULL ORDER BY fulfillment_id", summary.ID)
	if err != nil {
		t.Fatal("actual fulfillment link check unavailable")
	}
	ids := []string{}
	for links.Next() {
		var id string
		if links.Scan(&id) != nil {
			links.Close()
			t.Fatal("actual fulfillment link invalid")
		}
		ids = append(ids, id)
	}
	err = links.Err()
	links.Close()
	if err != nil || len(ids) != len(data.Fulfillments) {
		t.Fatal("actual fulfillment link scope differs")
	}
	for n, id := range ids {
		link := data.Fulfillments[n]
		if link.ID != id {
			t.Fatal("actual fulfillment link order differs")
		}
		var deleted sql.NullTime
		err := db.QueryRowContext(ctx, "SELECT deleted_at FROM fulfillments WHERE id=?", id).Scan(&deleted)
		if err == sql.ErrNoRows {
			if link.Availability != "missing" || link.Record != nil {
				t.Fatal("actual missing fulfillment fabricated")
			}
			counts["http_actual_missing_fulfillment_links"]++
			continue
		}
		if err != nil {
			t.Fatal("actual fulfillment availability check failed")
		}
		if deleted.Valid {
			if link.Availability != "deleted" || link.Record != nil {
				t.Fatal("actual deleted fulfillment exposed")
			}
			counts["http_actual_deleted_fulfillment_links"]++
			continue
		}
		if link.Availability != "available" || link.Record == nil {
			t.Fatal("actual active fulfillment omitted")
		}
		var expected orders.FulfillmentRecord
		err = db.QueryRowContext(ctx, "SELECT location_id,provider_id,shipping_option_id,delivery_address_id,requires_shipping,created_at,packed_at,shipped_at,delivered_at,canceled_at FROM fulfillments WHERE id=?", id).Scan(&expected.LocationID, &expected.ProviderID, &expected.ShippingOptionID, &expected.DeliveryAddressID, &expected.RequiresShipping, &expected.CreatedAt, &expected.PackedAt, &expected.ShippedAt, &expected.DeliveredAt, &expected.CanceledAt)
		if err != nil {
			t.Fatal("actual fulfillment facts unavailable")
		}
		if expected.DeliveryAddressID != nil {
			var a orders.FulfillmentAddress
			err = db.QueryRowContext(ctx, "SELECT id,company,first_name,last_name,address_1,address_2,city,country_code,province,postal_code,phone FROM fulfillment_addresses WHERE id=? AND deleted_at IS NULL", *expected.DeliveryAddressID).Scan(&a.ID, &a.Company, &a.FirstName, &a.LastName, &a.Address1, &a.Address2, &a.City, &a.CountryCode, &a.Province, &a.PostalCode, &a.Phone)
			if err != nil && err != sql.ErrNoRows {
				t.Fatal("actual fulfillment address check failed")
			}
			if err == nil {
				expected.Address = &a
				counts["http_actual_fulfillment_addresses_verified"]++
			}
		}
		expected.Items = []orders.FulfillmentItem{}
		items, err := db.QueryContext(ctx, "SELECT id,title,sku,barcode,raw_quantity,line_item_id,inventory_item_id FROM fulfillment_items WHERE fulfillment_id=? AND deleted_at IS NULL ORDER BY id", id)
		if err != nil {
			t.Fatal("actual fulfillment item check failed")
		}
		for items.Next() {
			var item orders.FulfillmentItem
			var raw string
			if items.Scan(&item.ID, &item.Title, &item.SKU, &item.Barcode, &raw, &item.LineItemID, &item.InventoryItemID) != nil {
				items.Close()
				t.Fatal("actual fulfillment item facts invalid")
			}
			position := len(expected.Items)
			if position >= len(link.Record.Items) {
				items.Close()
				t.Fatal("actual fulfillment item omitted")
			}
			var number struct{ Value json.RawMessage }
			if json.Unmarshal([]byte(raw), &number) != nil || number.Value == nil {
				items.Close()
				t.Fatal("actual fulfillment quantity unavailable")
			}
			var value string
			if json.Unmarshal(number.Value, &value) != nil {
				value = string(number.Value)
			}
			want, ok := new(big.Rat).SetString(value)
			got, valid := new(big.Rat).SetString(link.Record.Items[position].Quantity)
			if !ok || !valid || want.Cmp(got) != 0 {
				items.Close()
				t.Fatal("actual fulfillment quantity precision differs")
			}
			item.Quantity = link.Record.Items[position].Quantity
			expected.Items = append(expected.Items, item)
			counts["http_actual_fulfillment_items_verified"]++
		}
		err = items.Err()
		items.Close()
		if err != nil {
			t.Fatal("actual fulfillment item stream incomplete")
		}
		expected.Labels = []orders.FulfillmentLabel{}
		labels, err := db.QueryContext(ctx, "SELECT id,tracking_number,tracking_url,label_url FROM fulfillment_labels WHERE fulfillment_id=? AND deleted_at IS NULL ORDER BY id", id)
		if err != nil {
			t.Fatal("actual fulfillment label check failed")
		}
		for labels.Next() {
			var label orders.FulfillmentLabel
			if labels.Scan(&label.ID, &label.TrackingNumber, &label.TrackingURL, &label.LabelURL) != nil {
				labels.Close()
				t.Fatal("actual fulfillment label facts invalid")
			}
			expected.Labels = append(expected.Labels, label)
			counts["http_actual_fulfillment_labels_verified"]++
		}
		err = labels.Err()
		labels.Close()
		if err != nil {
			t.Fatal("actual fulfillment label stream incomplete")
		}
		want, _ := json.Marshal(expected)
		got, _ := json.Marshal(link.Record)
		if !bytes.Equal(want, got) {
			t.Fatal("actual fulfillment projected fields differ")
		}
		counts["http_actual_fulfillments_verified"]++
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(w.Body.Bytes(), &fields) != nil || len(fields) != 4 || strings.Contains(w.Body.String(), `"metadata"`) || strings.Contains(w.Body.String(), `"data"`) || strings.Contains(w.Body.String(), `"created_by"`) || strings.Contains(w.Body.String(), `"marked_shipped_by"`) {
		t.Fatal("actual fulfillment private projection differs")
	}
	counts["http_actual_order_fulfillments_verified"]++
}
