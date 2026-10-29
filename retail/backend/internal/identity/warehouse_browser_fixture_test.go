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
func TestPrepareWarehouseBrowserFixture(t *testing.T) {
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
		"UPDATE warehouse_user_runtime SET enabled=TRUE WHERE id=1",
		"UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1",
		"UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.edit_stock'",
		"INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('joint-warehouse-a','Warehouse A',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('joint-warehouse-b','Warehouse B',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO inventory_items(id,title,sku,requires_shipping,metadata,created_at,updated_at) VALUES('joint-warehouse-item','Scoped joint stock','WAREHOUSE-JOINT',TRUE,'{\"private\":\"must-not-escape\"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES('joint-warehouse-level','joint-warehouse-item','joint-warehouse-a',100.000000000000000001,0,1.25,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('joint-other-level','joint-warehouse-item','joint-warehouse-b',999,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("warehouse browser preparation failed")
		}
	}
	t.Log("OWNED_WAREHOUSE_BROWSER_READY")
}
