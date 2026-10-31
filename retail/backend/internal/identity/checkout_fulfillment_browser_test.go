package identity_test

import (
	"context"
	"fmt"
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/refunds"
	"os"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Only prepares stock/configuration and a newly created operator in the owned
// database. Orders, payment captures, assignments and parcels must be produced
// by the actual storefront and Admin commands during the browser rehearsal.
func mountCheckoutFulfillment(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, router *gin.Engine, options cart.InitialPaymentOptions) func() {
	t.Helper()
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()
	addCleanup := func(f func()) { cleanups = append(cleanups, f) }
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("checkout fulfillment fixture", e)
		}
	}
	exec("UPDATE catalog_variant_inventory_items SET required_quantity=1 WHERE id='add-v-link'")
	addCleanup(func() { exec("UPDATE catalog_variant_inventory_items SET required_quantity=2.5 WHERE id='add-v-link'") })
	// Select the existing supported manual delivery provider before checkout;
	// never rewrite the completed order's original shipping method.
	exec(`INSERT INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('manual_manual',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_location_providers(id,stock_location_id,fulfillment_provider_id,created_at,updated_at) VALUES('checkout-manual-location','add-location','manual_manual',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("UPDATE fulfillment_shipping_options SET provider_id='manual_manual' WHERE id='delivery-standard'")
	addCleanup(func() {
		exec("UPDATE fulfillment_shipping_options SET provider_id='delivery-provider' WHERE id='delivery-standard'")
	})
	exec(`INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES('checkout-source','Checkout warehouse stock','self','add-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	deviceCount := 4
	if os.Getenv("CHECKOUT_EXCHANGE") == "1" {
		deviceCount = 8
	}
	for i := 1; i <= deviceCount; i++ {
		exec(`INSERT INTO devices(id,serial_number,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,?,'add-v','checkout-source','add-location',0,'eur','','','available','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, fmt.Sprintf("checkout-device-%d", i), fmt.Sprintf("CHECKOUT-DEVICE-%d", i))
	}
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	addCleanup(func() { exec("UPDATE device_runtime_schema SET commands_enabled=FALSE WHERE id=1") })
	for _, table := range []string{"order_device_allocation_runtime", "order_device_packing_runtime", "order_device_parcel_runtime", "warehouse_user_runtime", "warehouse_scope_runtime"} {
		exec("UPDATE " + table + " SET enabled=TRUE WHERE id=1")
		addCleanup(func() { exec("UPDATE " + table + " SET enabled=FALSE WHERE id=1") })
	}
	if os.Getenv("CHECKOUT_REFUND") == "1" || os.Getenv("CHECKOUT_REPAIR") == "1" || os.Getenv("CHECKOUT_EXCHANGE") == "1" {
		// The ordinary cart fixture deliberately shares stock between two SKUs.
		// This owned serialized SKU must satisfy the real enrollment contract.
		exec("UPDATE catalog_variant_inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='add-shared-link'")
		addCleanup(func() { exec("UPDATE catalog_variant_inventory_items SET deleted_at=NULL WHERE id='add-shared-link'") })
		addCleanup(func() {
			exec("UPDATE device_variant_tracking SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id='add-v' AND deleted_at IS NULL")
		})
		for _, table := range []string{"customer_after_sales_runtime", "after_sales_review_runtime", "device_return_runtime", "device_service_runtime"} {
			exec("UPDATE " + table + " SET enabled=TRUE WHERE id=1")
			addCleanup(func() { exec("UPDATE " + table + " SET enabled=FALSE WHERE id=1") })
		}
		router.GET("/api/store/device-photos/:photo_id", (devices.Handler{DB: db}).PhotoHTTP)
	}
	hash, e := bcrypt.GenerateFromPassword([]byte("artificial-browser-password"), bcrypt.DefaultCost)
	if e != nil {
		t.Fatal("checkout operator KDF")
	}
	actor := identity.User{ID: "checkout-fulfillment-admin", Email: "browser@example.invalid", Name: "Checkout fulfillment operator", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: string(hash), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Create(&actor).Error != nil || db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, actor.ID) }) != nil {
		t.Fatal("checkout operator explicit grants")
	}
	if os.Getenv("CHECKOUT_REFUND") == "1" {
		exec("UPDATE order_refund_runtime SET enabled=TRUE WHERE id=1")
		addCleanup(func() { exec("UPDATE order_refund_runtime SET enabled=FALSE WHERE id=1") })
	}
	var admin *gin.Engine
	if os.Getenv("CHECKOUT_REFUND") == "1" {
		admin, e = server.RouterWithAuthAndRefunds(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19383"}, payments.MollieWebhookOptions{}, refunds.Options{Enabled: true, StoreID: "shop", Ring: options.Ring, Adapters: options.Adapters})
	} else {
		admin, e = server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19383"})
	}
	if e != nil {
		t.Fatal("checkout Admin router", e)
	}
	// Reuse the complete production Admin router, including authentication,
	// origin/CSRF, warehouse scope and command guards. The outer fixture owns
	// only the store/provider controls and routes no business request to Medusa.
	router.Any("/api/admin/*path", gin.WrapH(admin))
	success = true
	return cleanup
}

func checkCheckoutFulfillment(t *testing.T, db *gorm.DB, orders []string) {
	t.Helper()
	if os.Getenv("CHECKOUT_EXCHANGE") == "1" {
		checkCheckoutExchange(t, db, orders)
		return
	}
	trackingPrefix := "CHECKOUT-TRACK-%"
	if os.Getenv("CHECKOUT_REPAIR") == "1" {
		trackingPrefix = "000-CHECKOUT-REPAIR-%"
	}
	for _, order := range orders {
		checks := []struct {
			query string
			want  int64
		}{
			{"SELECT COUNT(*) FROM order_fulfillments l JOIN fulfillments f ON f.id=l.fulfillment_id WHERE l.order_id=? AND f.shipped_at IS NOT NULL AND f.canceled_at IS NULL", 1},
			{"SELECT COUNT(*) FROM order_fulfillments l JOIN fulfillments f ON f.id=l.fulfillment_id WHERE l.order_id=? AND f.canceled_at IS NOT NULL", 1},
			{"SELECT COUNT(*) FROM device_assignments a JOIN devices d ON d.id=a.device_id WHERE a.order_id=? AND a.status='shipped' AND d.state='shipped' AND a.tracking_number LIKE '" + trackingPrefix + "'", 1},
			{"SELECT COUNT(*) FROM device_assignments WHERE order_id=? AND status='canceled'", 1},
			{"SELECT COUNT(*) FROM inventory_reservations r JOIN order_items oi ON oi.item_id=r.line_item_id WHERE oi.order_id=? AND r.deleted_at IS NULL", 0},
			{"SELECT COUNT(*) FROM order_items WHERE order_id=? AND quantity=1 AND fulfilled_quantity=1 AND shipped_quantity=1", 1},
			{"SELECT COUNT(*) FROM customer_account_notices n JOIN customer_order_ownership o ON o.order_id=n.resource_id AND o.customer_id=n.customer_id WHERE n.resource_id=? AND n.kind='shipment.created' AND n.read_at IS NOT NULL", 1},
			{"SELECT COUNT(*) FROM order_device_parcel_operations WHERE order_id=? AND JSON_UNQUOTE(JSON_EXTRACT(request_json,'$.action'))='ship'", 1},
		}
		for _, check := range checks {
			var n int64
			if e := db.Raw(check.query, order).Scan(&n).Error; e != nil || n != check.want {
				t.Fatalf("checkout fulfillment durable facts differ: %s: %d, %v", check.query, n, e)
			}
		}
	}
	if os.Getenv("CHECKOUT_REPAIR") == "1" {
		checkCheckoutRepair(t, db, orders)
	}
	t.Log(`CHECKOUT_FULFILLMENT_JOINT_SQL {"same_four_new_paid_orders":true,"four_canceled_and_four_shipped_parcels":true,"one_shipment_receipt_per_order":true,"one_canceled_and_one_shipped_assignment_per_order":true,"reservations_consumed":true,"fulfilled_and_shipped_quantity_per_order":"1","one_own_read_shipment_notice_per_order":true,"final_reserved_delta":"0","final_stock_delta_from_added_baseline":"-4"}`)
}
