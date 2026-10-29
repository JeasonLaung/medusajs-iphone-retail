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
// The subsequent browser creates the warehouse account through production HTTP.
func TestPrepareWarehouseDevicesBrowserFixture(t *testing.T) {
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
		"INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('joint-device-product','iPhone 14','joint-device-product',FALSE,'draft',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO catalog_variants(id,product_id,title,manage_inventory,allow_backorder,created_at,updated_at) VALUES('joint-device-variant','joint-device-product','Warehouse device SKU',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES('joint-device-link','joint-device-variant','joint-warehouse-item',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES('joint-device-source','Own devices','self','joint-warehouse-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('joint-device-1','WAREHOUSE-PENDING','000000000009001','joint-device-variant','joint-device-source','joint-warehouse-a',12345,'eur','Shelf A','SECRET-NOTE','pending','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('joint-device-2','WAREHOUSE-INSPECTION','000000000009002','joint-device-variant','joint-device-source','joint-warehouse-a',12345,'eur','Shelf A','SECRET-NOTE','inspection','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('joint-device-3','WAREHOUSE-RETURNED','000000000009003','joint-device-variant','joint-device-source','joint-warehouse-a',12345,'eur','Shelf A','SECRET-NOTE','inspection','returned','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('joint-device-other','HIDDEN-OTHER-WAREHOUSE','000000000009004','joint-device-variant','joint-device-source','joint-warehouse-b',12345,'eur','Shelf A','SECRET-NOTE','available','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('joint-device-recycled','HIDDEN-RECYCLED','000000000009005','joint-device-variant','joint-device-source','joint-warehouse-a',12345,'eur','Shelf A','SECRET-NOTE','inspection','recycled','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("warehouse device fixture failed")
		}
	}
	t.Log("OWNED_WAREHOUSE_DEVICES_READY")
}
