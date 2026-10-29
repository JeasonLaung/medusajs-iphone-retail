package identity_test

import (
	"os"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This preparation runs only in the runner's newly owned artificial database.
// The subsequent browser enrolls a SKU through production HTTP.
func TestPrepareDeviceTrackingBrowserFixture(t *testing.T) {
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
		t.Fatal("fixture unavailable")
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal("fixture pool unavailable")
	}
	defer pool.Close()

	for _, q := range []string{
		"UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1",
		"INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('joint-device-product','Joint device','joint-device-product',FALSE,'draft',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO catalog_variants(id,product_id,title,sku,manage_inventory,allow_backorder,created_at,updated_at) VALUES('joint-device-variant','joint-device-product','Joint SKU','DEVICE-JOINT',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES('joint-device-link','joint-device-variant','joint-warehouse-item',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("device browser preparation failed")
		}
	}
	t.Log("OWNED_DEVICE_BROWSER_READY")
}
