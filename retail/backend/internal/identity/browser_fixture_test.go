package identity_test

import (
	"context"
	"fmt"
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
	if err != nil || result.Applied != 16 {
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
	seedReviewedCatalogFixture(t, db)
}

func seedReviewedCatalogFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
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
		`INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES ('fixture-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-set-unlinked',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-set-deleted',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE pricing_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-set-deleted'`,
		`INSERT INTO catalog_variant_price_sets(variant_id,price_set_id,id,created_at,updated_at) VALUES ('fixture-v','fixture-set','fixture-price-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-incomplete','fixture-set','fixture-shared-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-v','fixture-set-unlinked','fixture-deleted-price-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-v','fixture-set-deleted','fixture-deleted-set-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE catalog_variant_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-deleted-price-link'`,
		`INSERT INTO pricing_price_lists(id,status,title,description,type,rules_count,starts_at,ends_at,created_at,updated_at,metadata) VALUES ('fixture-list','active','Future sale','Artificial','sale',1,'2030-01-01','2030-02-01',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"list-secret"}'),('fixture-list-draft','draft','Draft offer','Artificial','override',0,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('fixture-list-deleted','active','Deleted offer','Artificial','sale',0,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`UPDATE pricing_price_lists SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-list-deleted'`,
		`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,min_quantity,max_quantity,price_list_id,created_at,updated_at) VALUES ('fixture-price-00','fixture-set','eur',12345678901234567890.123456789012345678,'{"value":"must-not-escape-raw"}',2,1,10,'fixture-list',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-price-01','fixture-set','eur',0,'{}',0,NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-price-02','fixture-set','eur',9,'{}',0,1.5,2.5,'fixture-list-draft',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-price-03','fixture-set','eur',17,'{}',0,NULL,NULL,'fixture-list-deleted',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-price-usd','fixture-set','usd',123.45,'{}',0,NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-price-deleted','fixture-set','eur',999,'{}',0,NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-price-unlinked','fixture-set-unlinked','eur',888,'{}',0,NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-price-deleted-set','fixture-set-deleted','eur',777,'{}',0,NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE pricing_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-price-deleted'`,
		`INSERT INTO pricing_price_rules(id,price_id,attribute,operator,value,priority,created_at,updated_at) VALUES ('fixture-rule','fixture-price-00','region_id','eq','region-fixture',10,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-rule-unknown','fixture-price-00','custom_condition','eq','0000123456789012345',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-rule-deleted','fixture-price-00','hidden','eq','must-not-escape-rule',20,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE pricing_price_rules SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-rule-deleted'`,
		`INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES ('fixture-list-rule','fixture-list','customer.groups','["0000123456789012345"]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-list-rule-deleted','fixture-list','hidden','"must-not-escape-list-rule"',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE pricing_price_list_rules SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-list-rule-deleted'`,
		`INSERT INTO inventory_items(id,sku,title,requires_shipping,created_at,updated_at,metadata) VALUES ('fixture-stock','0000123456789012345','Tracked item',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"inventory-secret"}'),('fixture-stock-retired','retired','Retired item',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('fixture-stock-unlinked','unlinked','Unlinked item',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`UPDATE inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-stock-retired'`,
		`INSERT INTO inventory_locations(id,name,created_at,updated_at,metadata) VALUES ('fixture-location','Fixture warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"warehouse-secret"}'),('fixture-location-retired','Removed warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-location-retired'`,
		`INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES ('fixture-stock-link','fixture-v','fixture-stock',2.5,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-stock-shared','fixture-incomplete','fixture-stock',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-stock-retired-link','fixture-v','fixture-stock-retired',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fixture-stock-deleted-link','fixture-v','fixture-stock-unlinked',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE catalog_variant_inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-stock-deleted-link'`,
		`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at,metadata) VALUES ('fixture-level','fixture-stock','fixture-location',9007199254740993.000000000000000001,9007199254740993,0.25,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"level-secret"}'),('fixture-level-retired-location','fixture-stock','fixture-location-retired',2,3,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at,deleted_at) VALUES ('fixture-level-deleted','fixture-stock','fixture-location',999,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,created_at,updated_at,metadata) VALUES ('fixture-reservation','fixture-stock','fixture-location',9007199254740992,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"reservation-secret"}'),('fixture-reservation-retired-location','fixture-stock','fixture-location-retired',3,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('fixture-reservation-no-level','fixture-stock','fixture-location-missing',0.5,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,created_at,updated_at,deleted_at) VALUES ('fixture-reservation-deleted','fixture-stock','fixture-location',999,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	}
	for i := 4; i < 25; i++ {
		statements = append(statements, fmt.Sprintf("INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES ('fixture-price-%02d','fixture-set','eur',%d,'{}',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", i, i))
	}
	for _, statement := range statements {
		if db.Exec(statement).Error != nil {
			t.Fatal("reviewed fixture rows failed")
		}
	}
	t.Log("OWNED_REVIEWED_CATALOG_READY")
}
