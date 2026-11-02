package identity_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/seo"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedSEOPublication(t *testing.T) {
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
		t.Fatal("owned SEO server unavailable")
	}
	defer root.Close()
	open := func(name string) (*sql.DB, *gorm.DB) {
		t.Helper()
		if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
			t.Fatal("owned SEO database unavailable")
		}
		c := *cfg
		c.DBName = name
		db, err := sql.Open("mysql", c.FormatDSN())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
		if err != nil {
			t.Fatal("SEO SQL adapter unavailable")
		}
		return db, gdb
	}

	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	pool, gdb := open("seo_publication_rehearsal")
	// Upgrade an actual 99-schema target; the production preview is untouched.
	old := fstest.MapFS{}
	names, _ := fs.ReadDir(migrations.Files, ".")
	for _, name := range names {
		if strings.HasSuffix(name.Name(), ".sql") && name.Name() < "0100" {
			body, _ := fs.ReadFile(migrations.Files, name.Name())
			old[name.Name()] = &fstest.MapFile{Data: body}
		}
	}
	prior, e := schema.Load(old)
	if e != nil {
		t.Fatal(e)
	}
	if r, e := schema.Apply(ctx, pool, prior); e != nil || r.Applied != 99 {
		t.Fatal("99 baseline failed")
	}
	if r, e := schema.Apply(ctx, pool, plan); e != nil || r.Applied != 4 || r.Skipped != 99 {
		t.Fatal("99 to102 upgrade failed")
	}
	if r, e := schema.Apply(ctx, pool, plan); e != nil || r.Skipped != 103 || r.Applied != 0 {
		t.Fatal("SEO migration replay changed schema")
	}
	ring, _ := protected.New("test", map[string][]byte{"test": bytes.Repeat([]byte{71}, 32)})
	store := seo.Store{DB: gdb, Ring: ring, Origin: "http://127.0.0.1:9300"}
	config, e := store.Config(ctx, "/dk-en/about///")
	if e != nil || config.Page != nil || len(config.Settings) != 1 || config.Settings["seo.site_url"] != store.Origin {
		t.Fatal("empty configuration fallback")
	}
	withoutOrigin := store
	withoutOrigin.Origin = ""
	config, e = withoutOrigin.Config(ctx, "/")
	if e != nil || config.Settings["seo.site_url"] != "http://localhost:9300" || config.Page != nil {
		t.Fatal("local preview without commerce origin must publish default SEO origin")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := pool.ExecContext(ctx, query, args...); e != nil {
			t.Fatal("artificial SEO fixture failed")
		}
	}
	setting := func(id, key, value string, secret bool) {
		t.Helper()
		sealed, e := ring.Seal(protected.Context{Purpose: "merchant-config", Table: "seo_settings", Column: "value", Row: id}, []byte(value))
		if e != nil {
			t.Fatal(e)
		}
		exec("INSERT INTO seo_settings(id,`key`,value,is_secret,`group`,created_at,updated_at) VALUES(?,?,?,?,'fixture',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, key, sealed, secret)
	}
	setting("native-name", "seo.site_name", "原文 EuroRefurb", false)
	setting("native-origin", "seo.site_url", "https://seo.example/path", false)
	setting("secret", "seo.default_title", "must-not-escape-secret", true)
	setting("private-key", "google.client_secret", "must-not-escape-private", false)
	setting("held", "seo.default_description", "must-not-escape-held", false)
	exec("INSERT INTO migration_merchant_dispositions VALUES('seo_setting','held',UNHEX(REPEAT('a',64)),UNHEX(REPEAT('b',64)),UNHEX(REPEAT('c',64)),UNHEX(REPEAT('d',32)),1,'hold','legacy-merchant-reconciliation')")
	setting("deleted", "seo.robots_txt", "must-not-escape-deleted", false)
	exec("UPDATE seo_settings SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted'")
	structured := `[{"@type":"Thing","name":"</script>原文"}]`
	exec("INSERT INTO seo_pages(id,path,title,description,canonical,og_image,hreflang,structured_data,robots,created_at,updated_at) VALUES('page','/dk-en/about','Override','Description','https://seo.example/dk-en/about','/image.png',?,?, 'noindex,follow',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", `{"en":"https://seo.example/dk-en/about"}`, structured)
	exec("INSERT INTO seo_pages(id,path,title,description,canonical,og_image,robots,created_at,updated_at,deleted_at) VALUES('deleted-page','/deleted','Private','','','','',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	config, e = store.Config(ctx, "/dk-en/about/")
	if e != nil || len(config.Settings) != 2 || config.Settings["seo.site_name"] != "原文 EuroRefurb" || config.Settings["seo.site_url"] != "https://seo.example" || config.Page == nil || config.Page.Title != "Override" || string(config.Page.StructuredData) != structured || config.Page.StructuredDataJSON != structured {
		t.Fatal("public configuration projection")
	}
	body, _ := json.Marshal(config)
	if bytes.Contains(body, []byte("must-not-escape")) || bytes.Contains(body, []byte("is_secret")) || bytes.Contains(body, []byte("native-name")) {
		t.Fatal("private SEO fields exposed")
	}
	pages, e := store.Pages(ctx)
	if e != nil || len(pages) != 1 || pages[0].Path != "/dk-en/about" {
		t.Fatal("discovery projection")
	}
	discovery, _ := json.Marshal(pages)
	if bytes.Contains(discovery, []byte("Override")) || bytes.Contains(discovery, []byte("structured_data")) {
		t.Fatal("discovery leaked page content")
	}
	// A moved ciphertext must fail under its row-bound merchant context.
	setting("duplicate", "seo.site_name", "Other", false)
	if _, e = store.Config(ctx, "/"); !errors.Is(e, seo.ErrUnavailable) {
		t.Fatal("ambiguous full key accepted")
	}
	exec("DELETE FROM seo_settings WHERE id='duplicate'")
	exec("UPDATE seo_settings SET value=(SELECT v FROM (SELECT value v FROM seo_settings WHERE id='native-origin') x) WHERE id='native-name'")
	if _, e = store.Config(ctx, "/"); !errors.Is(e, seo.ErrUnavailable) {
		t.Fatal("moved ciphertext accepted")
	}
	exec("DELETE FROM seo_settings WHERE id='native-name'")
	exec("INSERT INTO seo_pages(id,path,title,description,canonical,og_image,robots,created_at,updated_at) VALUES('duplicate-page','/dk-en/about','Other','','','','',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	if _, e = store.Config(ctx, "/dk-en/about"); !errors.Is(e, seo.ErrUnavailable) {
		t.Fatal("ambiguous page accepted")
	}
	if _, e = store.Pages(ctx); !errors.Is(e, seo.ErrUnavailable) {
		t.Fatal("ambiguous discovery accepted")
	}
	exec("DELETE FROM seo_pages WHERE id='duplicate-page'")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	store.Register(router.Group("/api/store"))
	for path, status := range map[string]int{"/api/store/seo?path=/dk-en/about": 200, "/api/store/seo/pages": 200, "/api/store/seo?path=//external": 400} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("SEO HTTP boundary %d", w.Code)
		}
	}
	exec("UPDATE seo_settings SET value='invalid-envelope' WHERE id='native-origin'")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/store/seo", nil))
	if w.Code != 503 || bytes.Contains(w.Body.Bytes(), []byte("invalid-envelope")) {
		t.Fatal("SEO failure boundary")
	}
	// Fresh100 is independently applied and validated, not just upgrade.
	fresh, _ := open("seo_publication_schema_fresh")
	if r, e := schema.Apply(ctx, fresh, plan); e != nil || r.Applied != 103 || schema.CheckApplied(ctx, fresh, plan) != nil {
		t.Fatal("fresh100 SEO schema")
	}
}
