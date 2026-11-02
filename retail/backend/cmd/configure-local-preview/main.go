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
	driver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	enable := flag.Bool("enable", false, "Enable draft-cart operations in the dedicated Windows preview database")
	payments := flag.Bool("payments", false, "Explicitly prepare test-only checkout and local order numbering")
	fulfillment := flag.Bool("fulfillment", false, "Enable existing device allocation, packing and parcel commands in the dedicated local preview")
	buyback := flag.Bool("buyback", false, "Enable existing customer buyback and Admin review commands in the dedicated local preview")
	flag.Parse()
	_ = godotenv.Load()
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil || os.Getenv("APP_ENV") != "development" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "eurorefurb_local_trial" {
		log.Fatal("Requires development environment and the dedicated loopback Windows preview database")
	}
	if *payments {
		options, e := checkoutconfig.Load(os.Getenv)
		if e != nil || options == nil || !strings.HasPrefix(os.Getenv("MOLLIE_API_KEY"), "test_") || os.Getenv("MOLLIE_LOCAL_TEST_WITHOUT_WEBHOOK") != "true" {
			log.Fatal("Valid local Mollie test checkout configuration required")
		}
	}
	db, err := platform.Open()
	if err != nil {
		log.Fatal("Preview database unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	err = db.Transaction(func(tx *gorm.DB) error {
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
}
