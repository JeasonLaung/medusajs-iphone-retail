// Explicit local preview configuration; never performs schema migration or
// enables external payments, refunds, stock reservation or notification jobs.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/eurorefurb/retail/internal/platform"
	driver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	enable := flag.Bool("enable", false, "Enable draft-cart operations in the dedicated Windows preview database")
	flag.Parse()
	_ = godotenv.Load()
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil || os.Getenv("APP_ENV") != "development" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "eurorefurb_local_trial" {
		log.Fatal("Requires development environment and the dedicated loopback Windows preview database")
	}
	db, err := platform.Open()
	if err != nil {
		log.Fatal("Preview database unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	err = db.Transaction(func(tx *gorm.DB) error {
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
}
