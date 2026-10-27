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
	if err != nil || result.Applied != 9 {
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
	// Draft payload is deliberately private. The list must use column projection.
	if db.Exec(`INSERT INTO catalog_products (id,handle,title,status,payload,created_at,updated_at) VALUES
	 ('fixture-a','fixture-a','Phone 50%_!','draft','{"private":"must-not-escape","title":"stale-title"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
	 ('fixture-b','fixture-b','Phone other','published','{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
	 ('fixture-deleted','fixture-deleted','Removed','draft','{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`).Error != nil {
		t.Fatal("fixture products failed")
	}
	if db.Exec(`UPDATE catalog_products SET deleted_at=UTC_TIMESTAMP(6) WHERE id='fixture-deleted'`).Error != nil {
		t.Fatal("fixture soft deletion failed")
	}
	if db.Exec(`INSERT INTO catalog_variants (id,product_id,title,payload,created_at,updated_at) VALUES
	 ('fixture-v','fixture-a','Variant','{"private":"variant-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`).Error != nil {
		t.Fatal("fixture variant failed")
	}
	t.Log("OWNED_BROWSER_FIXTURE_READY")
}
