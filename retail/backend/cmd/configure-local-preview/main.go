// Explicit dedicated local preview configuration. Draft setup is independent
// from opt-in test checkout; schema migration and mail/refund workers stay separate.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/checkoutconfig"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	driver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	enable := flag.Bool("enable", false, "Enable draft-cart operations in the dedicated Windows preview database")
	payments := flag.Bool("payments", false, "Explicitly prepare test-only checkout and local order numbering")
	contextPayment := flag.Bool("payment-context", false, "Enable payment-aware cart context save and recovery in the dedicated local preview")
	fulfillment := flag.Bool("fulfillment", false, "Enable existing device allocation, packing and parcel commands in the dedicated local preview")
	buyback := flag.Bool("buyback", false, "Enable existing customer buyback and Admin review commands in the dedicated local preview")
	afterSales := flag.Bool("after-sales", false, "Enable existing customer requests, Admin review and device receipt/repair return in the dedicated local preview")
	warehouses := flag.Bool("warehouse-users", false, "Enable warehouse account management and scoped access in the dedicated local preview")
	reviews := flag.Bool("reviews", false, "Enable product review reads, customer submissions and Admin moderation in the dedicated local preview")
	orderPayments := flag.Bool("order-payments", false, "Enable existing customer supplemental payments in the dedicated local Mollie test preview")
	refunds := flag.Bool("refunds", false, "Enable existing refund commands in the dedicated local Mollie test preview")
	notificationMail := flag.Bool("notification-mail", false, "Validate explicit notification mailbox management without starting mail workers")
	productEntry := flag.Bool("product-entry", false, "Enable existing product, SKU, price and initial inventory entry in the dedicated local preview")
	warehouseManagement := flag.Bool("warehouse-management", false, "Enable existing warehouse management and validate device entry in the dedicated local preview")
	flag.Parse()
	_ = godotenv.Load()
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil || os.Getenv("APP_ENV") != "development" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "eurorefurb_local_trial" {
		log.Fatal("Requires development environment and the dedicated loopback Windows preview database")
	}
	if *notificationMail {
		if os.Getenv("NOTIFICATION_MAIL_ENABLED") != "true" {
			log.Fatal("Explicit notification mailbox management configuration required")
		}
		if _, err := protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON")); err != nil {
			log.Fatal("Persistent merchant encryption keyring required; do not replace keys used by saved mailboxes")
		}
	}
	if *payments || *contextPayment || *orderPayments || *refunds {
		options, e := checkoutconfig.Load(os.Getenv)
		if e != nil || options == nil || !strings.HasPrefix(os.Getenv("MOLLIE_API_KEY"), "test_") || os.Getenv("MOLLIE_LOCAL_TEST_WITHOUT_WEBHOOK") != "true" {
			log.Fatal("Valid local Mollie test checkout configuration required")
		}
		if *contextPayment {
			deletes, err := checkoutconfig.ContextPayments(os.Getenv, options)
			if err != nil || deletes == nil {
				log.Fatal("Explicit local checkout context payment configuration required")
			}
		}
		if *orderPayments {
			service, err := checkoutconfig.OrderPayments(os.Getenv, options)
			if err != nil || service == nil {
				log.Fatal("Explicit local order payment and notification configuration required")
			}
		}
		if *refunds {
			options, err := checkoutconfig.Refunds(os.Getenv, options)
			if err != nil || !options.Enabled {
				log.Fatal("Explicit local refund configuration required")
			}
		}
	}
	db, err := platform.Open()
	if err != nil {
		log.Fatal("Preview database unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	err = db.Transaction(func(tx *gorm.DB) error {
		if *warehouseManagement {
			var row struct {
				Version  int
				Revision uint64
			}
			if e := tx.Table("warehouse_management_runtime").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; e != nil || row.Version != 1 || row.Revision == 0 {
				return gorm.ErrInvalidData
			}
			var device struct {
				Version                          int
				Revision                         uint64
				IdentityEnabled, CommandsEnabled bool
			}
			if e := tx.Table("device_runtime_schema").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&device).Error; e != nil || device.Version != 1 || device.Revision == 0 || !device.IdentityEnabled || !device.CommandsEnabled {
				return gorm.ErrInvalidData
			}
			if e := tx.Table("warehouse_management_runtime").Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
				return e
			}
		}
		if *productEntry {
			for table, commands := range map[string][]string{
				"catalog_command_capabilities":   {"product.create"},
				"inventory_command_capabilities": {"inventory.create_level"},
			} {
				for _, command := range commands {
					var row struct{ Version int }
					if e := tx.Table(table).Where("command=?", command).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; e != nil || row.Version != 1 {
						return gorm.ErrInvalidData
					}
					if e := tx.Table(table).Where("command=? AND version=1", command).Update("enabled", true).Error; e != nil {
						return e
					}
				}
			}
			for _, table := range []string{"catalog_variant_create_runtime", "catalog_price_edit_runtime"} {
				var row struct {
					Version  int
					Revision uint64
				}
				if e := tx.Table(table).Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; e != nil || row.Version != 1 || row.Revision == 0 {
					return gorm.ErrInvalidData
				}
				if e := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
					return e
				}
			}
		}
		if *notificationMail {
			var row struct{ Revision uint64 }
			if e := tx.Raw("SELECT revision FROM notification_mail_settings WHERE id=1 FOR SHARE").Take(&row).Error; e != nil {
				return e
			}
			if row.Revision == 0 {
				return gorm.ErrInvalidData
			}
		}
		if *refunds {
			var row struct{ Version int }
			if e := tx.Raw("SELECT version FROM order_refund_runtime WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
				return e
			}
			if row.Version != 4 {
				return gorm.ErrInvalidData
			}
			if e := tx.Table("order_refund_runtime").Where("id=1 AND version=4").Update("enabled", true).Error; e != nil {
				return e
			}
		}
		if *orderPayments {
			for _, table := range []string{"order_payment_runtime", "mollie_notification_runtime"} {
				var row struct{ Version int }
				if e := tx.Raw("SELECT version FROM " + table + " WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
					return e
				}
				if row.Version != 1 {
					return gorm.ErrInvalidData
				}
				if e := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
					return e
				}
			}
		}
		if *reviews {
			var row struct{ Version int }
			if e := tx.Raw("SELECT version FROM product_review_runtime WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
				return e
			}
			if row.Version != 1 {
				return gorm.ErrInvalidData
			}
			if e := tx.Table("product_review_runtime").Where("id=1 AND version=1").Updates(map[string]any{"reads_enabled": true, "writes_enabled": true}).Error; e != nil {
				return e
			}
		}
		if *contextPayment {
			var row struct{ Version int }
			if e := tx.Raw("SELECT version FROM cart_payment_refresh_runtime WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
				return e
			}
			if row.Version != 1 {
				return gorm.ErrInvalidData
			}
			if e := tx.Table("cart_payment_refresh_runtime").Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
				return e
			}
		}
		if *warehouses {
			for _, table := range []string{"warehouse_user_runtime", "warehouse_scope_runtime"} {
				var row struct{ Version int }
				if e := tx.Raw("SELECT version FROM " + table + " WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
					return e
				}
				if row.Version != 1 {
					return gorm.ErrInvalidData
				}
				if e := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
					return e
				}
			}
		}
		if *afterSales {
			for _, table := range []string{"customer_after_sales_runtime", "after_sales_review_runtime", "device_return_runtime", "device_service_runtime"} {
				var row struct{ Version int }
				if e := tx.Raw("SELECT version FROM " + table + " WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
					return e
				}
				if row.Version != 1 {
					return gorm.ErrInvalidData
				}
				if e := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
					return e
				}
			}
		}
		if *buyback {
			for _, table := range []string{"buyback_request_runtime", "buyback_admin_runtime"} {
				var row struct {
					Version int
				}
				if e := tx.Raw("SELECT version FROM " + table + " WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
					return e
				}
				if row.Version != 1 {
					return gorm.ErrInvalidData
				}
				if e := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
					return e
				}
			}
		}
		if *fulfillment {
			var revision uint64
			if e := tx.Raw("SELECT revision FROM device_runtime_schema WHERE id=1 FOR UPDATE").Scan(&revision).Error; e != nil {
				return e
			}
			if revision == 0 {
				return gorm.ErrInvalidData
			}
			for _, table := range []string{"order_device_allocation_runtime", "order_device_packing_runtime", "order_device_parcel_runtime"} {
				var count int64
				if e := tx.Table(table).Where("id=1 AND version=1").Count(&count).Error; e != nil {
					return e
				}
				if count != 1 {
					return gorm.ErrInvalidData
				}
				if e := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
					return e
				}
			}
			if e := tx.Table("device_runtime_schema").Where("id=1").Updates(map[string]any{"identity_enabled": true, "commands_enabled": true}).Error; e != nil {
				return e
			}
		}
		if *payments {
			// These development numbers deliberately occupy a separate range.
			// They are not a production source-sequence cutover or import seed.
			var allocator struct{ Initialized bool }
			if e := tx.Raw("SELECT initialized FROM order_number_allocator WHERE id=1 FOR UPDATE").Take(&allocator).Error; e != nil {
				return e
			}
			if !allocator.Initialized {
				var history []struct {
					ID              string
					DisplayID       int64
					CustomDisplayID *string
				}
				if e := tx.Table("orders").Select("id,display_id,custom_display_id").Order("id").Find(&history).Error; e != nil {
					return e
				}
				raw, e := json.Marshal(history)
				if e != nil {
					return e
				}
				seed := orders.OrderNumberSeed{LastDisplayID: 1000000000, LastTick: time.Now().UTC().UnixMicro() / 10, EvidenceSHA256: sha256.Sum256(raw)}
				if e := orders.SeedOrderNumbers(context.Background(), tx, seed); e != nil {
					return e
				}
			}
			for _, table := range []string{"cart_checkout_runtime", "cart_completion_runtime"} {
				var count int64
				if e := tx.Table(table).Where("id=1 AND version=1").Count(&count).Error; e != nil {
					return e
				}
				if count != 1 {
					return gorm.ErrInvalidData
				}
				r := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true)
				if r.Error != nil {
					return r.Error
				}
			}
			var count int64
			if e := tx.Table("inventory_command_capabilities").Where("command='inventory.reservations' AND version=1").Count(&count).Error; e != nil {
				return e
			}
			if count != 1 {
				return gorm.ErrInvalidData
			}
			r := tx.Table("inventory_command_capabilities").Where("command='inventory.reservations' AND version=1").Update("enabled", true)
			if r.Error != nil {
				return r.Error
			}
		}
		// Lock the known-version settings before changing any of them. Missing or
		// incompatible schema rolls back the entire configuration operation.
		for _, table := range []string{"cart_item_runtime", "cart_contact_runtime", "cart_shipping_command_runtime", "cart_context_runtime", "cart_promotion_command_runtime"} {
			var row struct {
				Version int
				Enabled bool
			}
			if e := tx.Raw("SELECT version,enabled FROM " + table + " WHERE id=1 FOR UPDATE").Take(&row).Error; e != nil {
				return e
			}
			if row.Version != 1 {
				return gorm.ErrInvalidData
			}
			if *enable {
				if e := tx.Table(table).Where("id=1 AND version=1").Update("enabled", true).Error; e != nil {
					return e
				}
			} else if !row.Enabled {
				return gorm.ErrInvalidData
			}
		}
		for _, command := range []string{"cart.create", "cart.access"} {
			var row struct {
				Version int
				Enabled bool
			}
			if e := tx.Raw("SELECT version,enabled FROM cart_command_capabilities WHERE command=? FOR UPDATE", command).Take(&row).Error; e != nil {
				return e
			}
			if row.Version != 1 {
				return gorm.ErrInvalidData
			}
			if *enable {
				if e := tx.Table("cart_command_capabilities").Where("command=? AND version=1", command).Update("enabled", true).Error; e != nil {
					return e
				}
			} else if !row.Enabled {
				return gorm.ErrInvalidData
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("Draft-cart preview configuration unavailable; apply explicit migrations and run configure-local-preview --enable")
	}
	log.Print("Draft-cart preview ready: create/access/items/address/delivery/context/promotions")
	if *payments {
		log.Print("Local Mollie test checkout, completion, numbering and stock reservations ready; mail/refund workers unchanged")
	}
	if *fulfillment {
		log.Print("Local device allocation, packing and shipment registration enabled; no devices, stock or orders changed")
	}
	if *buyback {
		log.Print("Local buyback applications and Admin processing enabled; no prices, devices, mail or transfers created")
	}
	if *afterSales {
		log.Print("Local customer after-sales, review, receipt and repair return enabled; no orders/devices changed, refund workers unchanged")
	}
	if *warehouses {
		log.Print("Local warehouse account management and scoped access enabled; no users, grants, devices or stock created")
	}
	if *orderPayments {
		log.Print("Local customer supplemental payment and observation gates enabled; no instructions, payments or workers created")
	}
	if *refunds {
		log.Print("Local refund command gate enabled; no instructions, refunds or workers created")
	}
	if *notificationMail {
		log.Print("Local notification mailbox management ready; sender credentials and mail workers remain independent")
	}
	if *productEntry {
		log.Print("Local product entry gates enabled; no products, prices, inventory or reservations created")
	}
	if *warehouseManagement {
		log.Print("Local warehouse management enabled and device entry ready; no warehouses, sources or devices created")
	}
}
