package identity_test

import (
	"golang.org/x/crypto/bcrypt"
	"os"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPrepareBuybackBrowserFixture(t *testing.T) {
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
	hash, err := bcrypt.GenerateFromPassword([]byte("artificial-buyback-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal("fixture password")
	}
	for _, pair := range [][2]string{{"cust_buyback-browser", "buyback-browser@example.invalid"}, {"cust_buyback-other", "buyback-other@example.invalid"}} {
		if db.Exec(`INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,phone,enabled,auth_version,verified_at,created_at,updated_at) VALUES(?,?,?,'Browser','Customer','123456',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, pair[0], pair[1], string(hash)).Error != nil {
			t.Fatal("fixture customer")
		}
	}
	for _, q := range []string{
		`UPDATE catalog_products SET title='iPhone 13',handle='swappie-iphone-13' WHERE id='fixture-b'`,
		`INSERT INTO buyback_prices(id,product_id,capacity,amount,raw_amount,currency_code,created_at,updated_at) VALUES('buyback-browser-price','fixture-b','256GB',294,'{"value":"294","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE buyback_request_runtime SET enabled=TRUE WHERE id=1`,
		`UPDATE buyback_admin_runtime SET enabled=TRUE WHERE id=1`,
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("buyback browser fixture")
		}
	}
	t.Log("OWNED_BUYBACK_BROWSER_READY")
}
