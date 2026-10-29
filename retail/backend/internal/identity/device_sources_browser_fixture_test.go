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
// The subsequent browser creates device sources through production HTTP.
func TestPrepareDeviceSourcesBrowserFixture(t *testing.T) {
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
		"UPDATE warehouse_management_runtime SET enabled=TRUE WHERE id=1",
		"INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('source-joint-channel','Source default',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES('fixture-store','Source store','source-joint-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
		"INSERT INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('manual_manual',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("source browser preparation failed")
		}
	}
	t.Log("OWNED_DEVICE_SOURCES_BROWSER_READY")
}
