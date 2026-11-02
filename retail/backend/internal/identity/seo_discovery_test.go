package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedSEODiscovery(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned SQL fixture required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned discovery server unavailable")
	}
	defer root.Close()
	if _, err = root.ExecContext(ctx, "CREATE DATABASE seo_discovery_rehearsal"); err != nil {
		t.Fatal("owned discovery database unavailable")
	}
	cfg.DBName = "seo_discovery_rehearsal"
	pool, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if result, e := schema.Apply(ctx, pool, plan); e != nil || result.Applied != 104 {
		t.Fatal("discovery schema failed")
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := pool.ExecContext(ctx, query, args...); e != nil {
			t.Fatal("discovery fixture SQL failed", e)
		}
	}
	exec(`INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('public','Public',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other','Other',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES('shop','Shop','public',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_collections(id,title,handle,created_at,updated_at) VALUES('empty','Empty','empty-collection','2026-01-01','2026-02-03 04:05:06.123456'),('deleted','Deleted','deleted-collection','2026-01-01','2026-01-01')`)
	exec(`UPDATE catalog_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted'`)
	for i := 0; i < 106; i++ {
		id := fmt.Sprintf("product-%03d", i)
		status, channel := "published", "public"
		if i == 103 {
			status = "draft"
		}
		if i == 104 {
			channel = "other"
		}
		exec(`INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at,metadata) VALUES(?,?,?, ?,FALSE,TRUE,'2026-01-01','2026-02-03 04:05:06.123456','{"private":"must-not-escape-product"}')`, id, id, id, status)
		exec(`INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES(?,?,?,'2026-01-01','2026-01-01')`, "link-"+id, id, channel)
	}
	exec(`UPDATE catalog_products SET deleted_at=UTC_TIMESTAMP(6) WHERE id='product-105'`)
	for i, metadata := range []string{`{"slug":"iphone-15-128gb-blue-S85","private":"must-not-escape-variant"}`, `{"slug":"javascript:invalid"}`, `{"slug":"iphone-15-256gb-blue-S100"}`} {
		exec(`INSERT INTO catalog_variants(id,product_id,title,allow_backorder,manage_inventory,variant_rank,created_at,updated_at,metadata) VALUES(?,'product-000','SKU',FALSE,TRUE,?,'2026-01-01','2026-01-01',?)`, fmt.Sprintf("variant-%d", i), i, metadata)
	}
	exec(`UPDATE catalog_variants SET deleted_at=UTC_TIMESTAMP(6) WHERE id='variant-2'`)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := catalog.StoreHandler{DB: db, StoreID: "shop"}
	r.GET("/catalog", h.SEOCatalog)
	request := func(query string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/catalog"+query, nil))
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("discovery HTTP %d for %s", w.Code, query)
		}
		return w.Body.Bytes()
	}
	for _, query := range []string{"", "?kind=invalid", "?kind=products&kind=collections", "?kind=products&limit=101", "?kind=products&offset=-1", "?kind=products&handle=foreign", "?kind=products&store_id=other"} {
		request(query, 400)
	}
	var result struct {
		Entries []catalog.SEOCatalogEntry `json:"entries"`
		Count   int                       `json:"count"`
		Limit   int                       `json:"limit"`
		Offset  int                       `json:"offset"`
	}
	first := request("?kind=products&limit=100", 200)
	if json.Unmarshal(first, &result) != nil || result.Count != 103 || len(result.Entries) != 100 || result.Limit != 100 || result.Offset != 0 {
		t.Fatal("discovery first page")
	}
	if result.Entries[0].Handle != "product-000" || result.Entries[0].UpdatedAt.Format(time.RFC3339Nano) != "2026-02-03T04:05:06.123456Z" || len(result.Entries[0].Slugs) != 1 || result.Entries[0].Slugs[0] != "iphone-15-128gb-blue-S85" {
		t.Fatal("source SKU slug / timestamp projection")
	}
	for _, private := range []string{"must-not-escape", "metadata", `"id"`, "product-103", "product-104", "product-105", "javascript:"} {
		if strings.Contains(string(first), private) {
			t.Fatal("private or nonpublic discovery data exposed")
		}
	}
	if json.Unmarshal(request("?kind=products&limit=100&offset=100", 200), &result) != nil || result.Count != 103 || len(result.Entries) != 3 || result.Offset != 100 {
		t.Fatal("discovery second page")
	}
	if json.Unmarshal(request("?kind=collections", 200), &result) != nil || result.Count != 1 || len(result.Entries) != 1 || result.Entries[0].Handle != "empty-collection" || len(result.Entries[0].Slugs) != 0 {
		t.Fatal("source includes undeleted empty collections")
	}
	exec(`UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='public'`)
	request("?kind=products", 503)
	request("?kind=collections", 503)
	exec(`UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='public'`)
	exec(`RENAME TABLE catalog_variants TO seo_discovery_variants_missing`)
	request("?kind=products", 503)
	t.Log("OWNED_SEO_DISCOVERY_VERIFIED")
}
