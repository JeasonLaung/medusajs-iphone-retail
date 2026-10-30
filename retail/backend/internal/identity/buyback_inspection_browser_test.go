package identity_test

import (
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
	"time"
)

func TestPrepareInspectionBrowserDevice(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned browser fixture required")
	}
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("fixture SQL")
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()

	for _, q := range []string{
		`INSERT INTO commerce_channel_locations(id,sales_channel_id,stock_location_id,created_at,updated_at) VALUES('inspection-browser-location','fixture-channel','fixture-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_variants(id,product_id,title,variant_rank,manage_inventory,allow_backorder,created_at,updated_at) VALUES('inspection-browser-variant','fixture-b','Inspection SKU',0,TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES('inspection-browser-variant','fixture-256')`,
		`INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES('inspection-browser-source','Owned inspection fixture','self','fixture-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('inspection-browser-device','SN-JOINT','000000000000001','inspection-browser-variant','inspection-browser-source','fixture-location',0,'eur','','','inspection','recycled','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("inspection browser fixture")
		}
	}
	t.Log("OWNED_INSPECTION_BROWSER_READY")
}
