package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed testdata/context_payment_seed.sql
var contextPaymentSeed string

// Independent subflow fixture: no dependency on the long CartRuntime regression,
// order creation, fulfillment, repair, refund, customer or mail test suites.
func TestOwnedContextPaymentBrowser(t *testing.T) {
	if os.Getenv("CONTEXT_PAYMENT_BROWSER") != "1" {
		t.Skip("owned context browser resources required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err = root.ExecContext(ctx, "CREATE DATABASE context_payment_browser"); err != nil {
		t.Fatal("owned database already exists or unavailable")
	}
	cfg.DBName = "context_payment_browser"
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(12)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	result, err := schema.Apply(ctx, pool, plan)
	files, fileErr := fs.Glob(migrations.Files, "*.sql")
	if err != nil || fileErr != nil || len(files) == 0 || result.Applied != len(files) {
		t.Fatal("dedicated context fixture migrations", err)
	}
	t.Logf("CONTEXT_PAYMENT_MIGRATIONS=%d", result.Applied)
	for _, statement := range strings.Split(contextPaymentSeed, ";\n") {
		if strings.TrimSpace(statement) != "" {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatal("fixed context fixture seed", err)
			}
		}
	}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Protocol: 2, DisableIdentity: true, MaxRetries: -1})
	defer client.Close()
	sessions, err := sessionstore.New(client, "eurorefurb:context-browser:v1:")
	if err != nil {
		t.Fatal(err)
	}
	ring, err := protected.New("artificial", map[string][]byte{"artificial": bytes.Repeat([]byte{0x71}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	options := cart.InitialPaymentOptions{Ring: ring, ManualProviders: []string{"delivery-provider"}}
	makeCart := func() (sessionstore.CartReference, cart.InitialPaymentCommand) {
		t.Helper()
		id := uuid.NewString()
		created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: id, ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: "en-GB"})
		if err != nil {
			t.Fatal("fixture cart", err)
		}
		ref := created.Reference
		if _, err = cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"}); err != nil {
			t.Fatal("fixture line", err)
		}
		address := cart.ContactAddress{FirstName: "Owned", LastName: "Buyer", Address1: "Test street 1", City: "Copenhagen", PostalCode: "0012", Country: "dk", Phone: "+45 111"}
		if _, err = cart.SaveContacts(ctx, db, "shop", ref, cart.ContactCommand{OperationID: uuid.NewString(), Version: "2", Email: "owned@example.invalid", Shipping: address, Billing: address}, options.ManualProviders); err != nil {
			t.Fatal("fixture contact", err)
		}
		if _, err = cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: "3", OptionID: "delivery-standard"}, options.ManualProviders); err != nil {
			t.Fatal("fixture shipping", err)
		}
		return ref, cart.InitialPaymentCommand{OperationID: uuid.NewString(), Version: "4"}
	}
	if os.Getenv("PAYPAL_CHECKOUT_REHEARSAL") == "1" {
		checkPayPalCheckout(t, ctx, db, sessions, makeCart, options)
	} else if os.Getenv("PAYPAL_WALLET_BROWSER") == "1" {
		checkPayPalWalletBrowser(t, ctx, db, sessions, makeCart, options)
	} else {
		checkContextPaymentBrowser(t, ctx, db, sessions, makeCart, options)
	}
}
