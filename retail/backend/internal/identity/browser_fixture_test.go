package identity_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Only prepares an owned throwaway database. The real cmd/api subsequently
// serves the browser; this fixture adds no test HTTP routes or auth bypass.
func TestPrepareOwnedBrowserAPI(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned browser database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("fixture SQL unavailable")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal("fixture pool unavailable")
	}
	defer pool.Close()
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("fixture plan invalid")
	}
	result, err := schema.Apply(ctx, pool, plan)
	if err != nil || result.Applied != 10 {
		t.Fatal("fixture migrations failed")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("artificial-browser-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal("fixture KDF failed")
	}
	u := identity.User{ID: "new-browser-fixture", Email: "browser@example.invalid", Name: "Browser Fixture", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: string(hash), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Create(&u).Error != nil {
		t.Fatal("fixture new account failed")
	}
	t.Log("OWNED_BROWSER_FIXTURE_READY")
}

// Runtime fixture for the full reviewed schema; never used by the application.
func TestPrepareReviewedCatalogFixture(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned browser database required")
	}
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("reviewed fixture SQL unavailable")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal("reviewed fixture pool unavailable")
	}
	defer pool.Close()
	statements := []string{
		`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at,metadata) VALUES
   ('fixture-a','Phone 50%_!','fixture-a',0,'draft',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"must-not-escape"}'),
   ('fixture-b','Phone other','fixture-b',0,'published',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),
   ('fixture-deleted','Removed','fixture-deleted',0,'draft',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`UPDATE catalog_products SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-deleted'`,
		`INSERT INTO catalog_variants(id,product_id,title,sku,allow_backorder,manage_inventory,variant_rank,created_at,updated_at,metadata) VALUES
   ('fixture-v','fixture-a','Complete variant','0000123456789012345',0,1,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"variant-secret"}'),
   ('fixture-incomplete','fixture-b','Incomplete variant',NULL,1,1,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES ('fixture-storage','Storage',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES
   ('fixture-128','128 GB','fixture-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
   ('fixture-256','256 GB','fixture-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_product_options(id,product_id,product_option_id,created_at,updated_at) VALUES
   ('fixture-membership-a','fixture-a','fixture-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
   ('fixture-membership-b','fixture-b','fixture-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES
   ('fixture-value-a','fixture-membership-a','fixture-128',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
   ('fixture-value-b','fixture-membership-b','fixture-256',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES ('fixture-v','fixture-128'),('fixture-incomplete','fixture-128')`,
		`INSERT INTO catalog_images(id,url,` + "`rank`" + `,product_id,created_at,updated_at) VALUES
   ('fixture-image','/fixture-product.svg',0,'fixture-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
   ('fixture-removed-image','/removed.svg',1,'fixture-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE catalog_images SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-removed-image'`,
		`INSERT INTO catalog_variant_images(id,variant_id,image_id,created_at,updated_at) VALUES ('fixture-image-link','fixture-v','fixture-image',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	}
	for _, statement := range statements {
		if db.Exec(statement).Error != nil {
			t.Fatal("reviewed fixture rows failed")
		}
	}
	t.Log("OWNED_REVIEWED_CATALOG_READY")
}
