package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedProductPrices(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned MySQL and Redis required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	base, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("fixture connection failed")
	}
	defer base.Close()
	if _, err = base.ExecContext(ctx, "CREATE DATABASE product_price_rehearsal"); err != nil {
		t.Fatal("fixture schema failed")
	}
	cfg.DBName = "product_price_rehearsal"
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("fixture SQL failed")
	}
	pool, _ := db.DB()
	defer pool.Close()
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("fixture plan invalid")
	}
	result, err := schema.Apply(ctx, pool, plan)
	if err != nil || result.Applied != 51 {
		t.Fatal("fixture migrations failed")
	}
	seedReviewedCatalogFixture(t, db)
	fingerprint := func() [32]byte {
		var evidence []json.RawMessage
		for _, table := range []string{"catalog_products", "catalog_variants", "catalog_variant_price_sets", "pricing_price_sets", "pricing_prices", "pricing_price_rules", "pricing_price_lists", "pricing_price_list_rules"} {
			var rows []map[string]any
			if db.Table(table).Order("id ASC").Find(&rows).Error != nil {
				t.Fatal("fixture fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			evidence = append(evidence, b)
		}
		b, _ := json.Marshal(evidence)
		return sha256.Sum256(b)
	}
	before := fingerprint()
	gin.SetMode(gin.TestMode)
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(old)
	r := gin.New()
	r.GET("/products/:id/variants/:variant_id/prices", (catalog.Handler{DB: db}).AdminVariantPrices)
	type response struct {
		Prices        []catalog.AdminConfiguredPrice
		PriceSetIDs   []string `json:"price_set_ids"`
		Count         int64
		Limit, Offset int
	}
	call := func(path string, status int) response {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatal("price HTTP status incorrect")
		}
		var out response
		if status == 200 && json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("price response invalid")
		}
		for _, secret := range []string{"must-not-escape", "list-secret", "raw_amount", "metadata", "payload"} {
			if containsBytes(w.Body.Bytes(), secret) {
				t.Fatal("private price fields escaped")
			}
		}
		return out
	}
	root := "/products/fixture-a/variants/fixture-v/prices"
	first := call(root, 200)
	if first.Count != 26 || len(first.Prices) != 20 || len(first.PriceSetIDs) != 1 || first.PriceSetIDs[0] != "fixture-set" {
		t.Fatal("price membership, count or pagination incorrect")
	}
	p := first.Prices[0]
	if p.ID != "fixture-price-00" || p.Amount != "12345678901234567890.123456789012345678" || p.MinQuantity == nil || *p.MinQuantity != "1.000000000000000000" || len(p.Rules) != 2 || p.Rules[0].Priority != 10 || p.Rules[1].Value != "0000123456789012345" {
		t.Fatal("exact price or rule evidence changed")
	}
	if p.PriceList == nil || p.PriceList.Status != "active" || p.PriceList.StartsAt == nil || p.PriceList.StartsAt.Year() != 2030 || len(p.PriceList.Rules) != 1 || p.PriceList.Rules[0].Value == nil || *p.PriceList.Rules[0].Value != `["0000123456789012345"]` {
		t.Fatal("future price list or rule changed")
	}
	if first.Prices[1].Amount != "0.000000000000000000" || first.Prices[1].PriceList != nil || first.Prices[1].Rules == nil {
		t.Fatal("true zero or empty rule distinction lost")
	}
	if first.Prices[2].PriceList == nil || first.Prices[2].PriceList.Status != "draft" || first.Prices[2].MinQuantity == nil || *first.Prices[2].MinQuantity != "1.500000000000000000" {
		t.Fatal("draft list or quantity precision lost")
	}
	if first.Prices[3].PriceListID == nil || first.Prices[3].PriceList != nil {
		t.Fatal("unavailable list reference not retained")
	}
	second := call(root+"?limit=20&offset=20", 200)
	if second.Count != 26 || len(second.Prices) != 6 || second.Prices[5].CurrencyCode != "usd" {
		t.Fatal("stable second price page incorrect")
	}
	shared := call("/products/fixture-b/variants/fixture-incomplete/prices?limit=1", 200)
	if shared.Count != 26 || shared.Prices[0].ID != p.ID {
		t.Fatal("shared price set lost")
	}
	if len(call(root+"?offset=100", 200).Prices) != 0 {
		t.Fatal("empty price page incorrect")
	}
	for _, path := range []string{"/products/fixture-b/variants/fixture-v/prices", "/products/fixture-deleted/variants/fixture-v/prices", "/products/fixture-a/variants/missing/prices"} {
		call(path, 404)
	}
	for _, q := range []string{"fields=*", "limit=0", "limit=1&limit=2", "currency_code=eur"} {
		call(root+"?"+q, 400)
	}
	if fingerprint() != before {
		t.Fatal("price reads modified source evidence")
	}
	// Exercise a real SQL rule overflow: fail closed instead of returning truncated rules.
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal("rule limit fixture failed")
	}
	for offset := 0; offset < 10000; offset += 500 {
		rows := make([]map[string]any, 0, 500)
		for j := offset; j < offset+500; j++ {
			rows = append(rows, map[string]any{"id": fmtPriceRule(j), "price_id": "fixture-price-00", "attribute": "artificial", "operator": "eq", "value": "x", "priority": 0, "created_at": time.Now().UTC(), "updated_at": time.Now().UTC()})
		}
		if tx.Table("pricing_price_rules").Create(rows).Error != nil {
			tx.Rollback()
			t.Fatal("rule limit fixture failed")
		}
	}
	if tx.Commit().Error != nil {
		t.Fatal("rule limit fixture commit failed")
	}
	call(root+"?limit=1", 503)
	if db.Exec("DELETE FROM pricing_price_rules WHERE id LIKE 'fixture-overflow-%'").Error != nil {
		t.Fatal("rule limit cleanup failed")
	}
	if fingerprint() != before {
		t.Fatal("rule limit fixture changed original evidence")
	}
	sessions, redis, err := sessionstore.Open(ctx, "redis://127.0.0.1:6379/0", "eurorefurb:price_fixture:v1:")
	if err != nil {
		t.Fatal("fixture Redis unavailable")
	}
	defer redis.Close()
	t.Setenv("STOREFRONT_STORE_ID", "public-shop")
	guarded, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "https://admin.example", Secure: true})
	if err != nil {
		t.Fatal("guarded price router unavailable")
	}
	checkStoreRegions(t, db, guarded)
	checkStoreTaxonomy(t, db, guarded)
	checkStoreLocales(t, db, guarded)
	checkServiceContacts(t, db, guarded)
	checkStorePolicies(t, db, guarded)
	checkStoreSearch(t, db, guarded)
	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, httptest.NewRequest("GET", "https://admin.example/api/admin"+root, nil))
	if w.Code != 401 {
		t.Fatal("anonymous price read accepted")
	}
	t.Log(`PRODUCT_PRICE_REHEARSAL {"migrations":51,"configured_prices":26,"page_size":20,"exact_decimal":true,"shared_sets":true,"soft_delete_filters":true,"future_and_draft_lists_retained":true,"unavailable_list_reference_retained":true,"unknown_rule_retained":true,"cross_product_rejected":true,"anonymous_rejected":true,"rule_limit_503":true,"read_fingerprint_unchanged":true}`)
}

func containsBytes(b []byte, s string) bool { return strings.Contains(string(b), s) }
func fmtPriceRule(i int) string             { return fmt.Sprintf("fixture-overflow-%05d", i) }
