package identity_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/seo"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedSEOCommands(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned SQL fixture required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	root, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	if _, e = root.ExecContext(ctx, "CREATE DATABASE seo_commands_rehearsal"); e != nil {
		t.Fatal("owned SEO command database failed")
	}
	cfg.DBName = "seo_commands_rehearsal"
	pool, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	old := fstest.MapFS{}
	files, _ := fs.ReadDir(migrations.Files, ".")
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sql") && file.Name() < "0101" {
			b, _ := fs.ReadFile(migrations.Files, file.Name())
			old[file.Name()] = &fstest.MapFile{Data: b}
		}
	}
	prior, _ := schema.Load(old)
	if r, e := schema.Apply(ctx, pool, prior); e != nil || r.Applied != 100 {
		t.Fatal("actual100 command baseline failed")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.ExecContext(ctx, q, args...); e != nil {
			t.Fatal("SEO command artificial SQL", e)
		}
	}
	exec("INSERT INTO authorization_roles(id,name,enabled) VALUES('existing-role','Existing',TRUE)")
	exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT 'existing-role',code FROM authorization_actions")
	plan, _ := schema.Load(migrations.Files)
	if r, e := schema.Apply(ctx, pool, plan); e != nil || r.Applied != 3 || r.Skipped != 100 {
		t.Fatal("actual100 to103 command migration failed")
	}
	if r, e := schema.Apply(ctx, pool, plan); e != nil || r.Applied != 0 || r.Skipped != 103 {
		t.Fatal("command migration replay failed")
	}
	var count int
	if pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM authorization_role_actions WHERE role_id='existing-role'").Scan(&count) != nil || count != 33 {
		t.Fatal("migration silently granted existing role authority")
	}
	db, e := gorm.Open(mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if e != nil {
		t.Fatal(e)
	}
	actor := identity.User{ID: "seo-new-admin", Email: "seo@example.invalid", Name: "SEO Fixture", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: "artificial-not-a-login", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&actor).Error; e != nil {
			return e
		}
		return authorization.Bootstrap(ctx, tx, actor.ID)
	}) != nil {
		t.Fatal("explicit new account/grants failed")
	}
	ring, _ := protected.New("test", map[string][]byte{"test": bytes.Repeat([]byte{0x71}, 32)})
	s := seo.Store{DB: db, Ring: ring, Origin: "https://seo.example"}
	view := func(resource string) seo.Directory {
		t.Helper()
		v, e := s.Directory(ctx, actor, resource, 50, 0)
		if e != nil || len(v.Revision) != 64 {
			t.Fatal("SEO directory", e)
		}
		return v
	}
	v := view("settings")
	if v.Enabled || !v.EncryptionReady || len(v.Settings) != 0 {
		t.Fatal("SEO gate/default context")
	}
	in := seo.Command{OperationID: uuid.NewString(), ExpectedRevision: v.Revision, Resource: "settings", Action: "save", Setting: &seo.SettingInput{Key: "seo.site_name", Value: " 原文 翻新店 ", Group: "site"}}
	if _, e := s.Save(ctx, actor, in); !errors.Is(e, seo.ErrUnavailable) {
		t.Fatal("closed SEO gate accepted write", e)
	}
	exec("UPDATE seo_command_state SET enabled=TRUE WHERE id=1")
	results, errs := make([]seo.CommandResult, 6), make([]error, 6)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = s.Save(ctx, actor, in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, result := range results {
		if errs[i] != nil || result.ID == "" {
			t.Fatal("SEO concurrent command", errs[i])
		}
		if !result.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("SEO command submitted more than once")
	}
	if pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM seo_command_receipts").Scan(&count) != nil || count != 1 {
		t.Fatal("SEO receipt count")
	}
	var envelope []byte
	if pool.QueryRowContext(ctx, "SELECT value FROM seo_settings WHERE id=?", results[0].ID).Scan(&envelope) != nil || bytes.Contains(envelope, []byte("翻新店")) {
		t.Fatal("SEO setting stored plaintext")
	}
	plain, e := ring.Open(protected.Context{Purpose: "merchant-config", Table: "seo_settings", Column: "value", Row: results[0].ID}, envelope)
	if e != nil || string(plain) != "原文 翻新店" {
		t.Fatal("SEO row-bound envelope")
	}
	clear(plain)
	config, e := s.Config(ctx, "/")
	if e != nil || config.Settings["seo.site_name"] != "原文 翻新店" {
		t.Fatal("saved setting not published", e)
	}
	changed := in
	changed.Setting = &seo.SettingInput{Key: "seo.site_name", Value: "Changed", Group: "site"}
	if _, e := s.Save(ctx, actor, changed); !errors.Is(e, seo.ErrConflict) {
		t.Fatal("changed UUID payload accepted", e)
	}
	stale := in
	stale.OperationID = uuid.NewString()
	if _, e := s.Save(ctx, actor, stale); !errors.Is(e, seo.ErrConflict) {
		t.Fatal("stale revision accepted", e)
	}
	exec("UPDATE seo_command_state SET enabled=FALSE WHERE id=1")
	if r, e := s.Save(ctx, actor, in); e != nil || !r.Replayed {
		t.Fatal("closed-gate receipt recovery failed", e)
	}
	exec("UPDATE seo_command_state SET enabled=TRUE WHERE id=1")
	page := seo.Command{OperationID: uuid.NewString(), ExpectedRevision: view("pages").Revision, Resource: "pages", Action: "save", Page: &seo.PageInput{Path: " /dk-en/about/// ", Title: "原文标题", Description: "Description", Canonical: "https://seo.example/dk-en/about", OGImage: "https://seo.example/image.png", Robots: "noindex,follow", HreflangJSON: `{"en":"https://seo.example/dk-en/about","zh-CN":"https://seo.example/dk-zh/about"}`, StructuredDataJSON: `{"@type":"Thing","value":900719925474099312345,"name":"</script>原文"}`}}
	p, e := s.Save(ctx, actor, page)
	if e != nil {
		t.Fatal("SEO page create", e)
	}
	config, e = s.Config(ctx, "/dk-en/about")
	if e != nil || config.Page == nil || config.Page.Title != "原文标题" || !strings.Contains(string(config.Page.StructuredData), "900719925474099312345") || config.Page.StructuredDataJSON != page.Page.StructuredDataJSON {
		t.Fatal("actual public page projection", e)
	}
	duplicate := page
	duplicate.OperationID, duplicate.ExpectedRevision = uuid.NewString(), view("pages").Revision
	if _, e := s.Save(ctx, actor, duplicate); !errors.Is(e, seo.ErrConflict) {
		t.Fatal("duplicate full path accepted", e)
	}
	// Both long paths share more than a VARCHAR(191) index prefix.
	for _, ending := range []string{"one", "two"} {
		long := page
		cp := *page.Page
		cp.Path = "/dk-en/" + strings.Repeat("a", 220) + "/" + ending
		long.Page, long.OperationID, long.ExpectedRevision = &cp, uuid.NewString(), view("pages").Revision
		if _, e := s.Save(ctx, actor, long); e != nil {
			t.Fatal("full path was truncated or prefix-conflated", e)
		}
	}
	before := view("settings")
	fault := seo.Command{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Resource: "settings", Action: "save", ID: results[0].ID, Setting: &seo.SettingInput{Key: "seo.site_name", Value: "Must roll back", Group: "site"}}
	exec(`CREATE TRIGGER seo_receipt_fault BEFORE INSERT ON seo_command_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial last-write failure'`)
	if _, e := s.Save(ctx, actor, fault); e == nil {
		t.Fatal("receipt failure accepted")
	}
	exec("DROP TRIGGER seo_receipt_fault")
	if after := view("settings"); after.Revision != before.Revision || after.Settings[0].Value != before.Settings[0].Value {
		t.Fatal("failed command partially changed encrypted setting")
	}
	// Secret/held values are deliberately invalid envelopes. Reading them
	// would fail this test; they must be excluded before decryption.
	exec(`INSERT INTO seo_settings(id,` + "`key`" + `,value,is_secret,` + "`group`" + `,created_at,updated_at) VALUES('secret','seo.default_title','invalid-secret',TRUE,'site',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('held','seo.default_description','invalid-held',FALSE,'site',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('private','google.client_secret','invalid-private',FALSE,'site',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("INSERT INTO migration_merchant_dispositions VALUES('seo_setting','held',UNHEX(REPEAT('a',64)),UNHEX(REPEAT('b',64)),UNHEX(REPEAT('c',64)),UNHEX(REPEAT('d',32)),1,'hold','legacy-merchant-reconciliation')")
	v = view("settings")
	if len(v.Settings) != 3 {
		t.Fatal("private key appeared in SEO directory")
	}
	for _, row := range v.Settings {
		if row.ID == "secret" || row.ID == "held" {
			if !row.Protected || row.Value != "" {
				t.Fatal("protected setting opened or exposed")
			}
			c := seo.Command{OperationID: uuid.NewString(), ExpectedRevision: v.Revision, Resource: "settings", Action: "delete", ID: row.ID}
			if _, e := s.Save(ctx, actor, c); !errors.Is(e, seo.ErrProtected) {
				t.Fatal("protected setting deletion accepted", e)
			}
		}
	}
	config, e = s.Config(ctx, "/")
	if e != nil || config.Settings["seo.default_title"] != "" || config.Settings["seo.default_description"] != "" {
		t.Fatal("secret or held setting published", e)
	}
	deletePage := seo.Command{OperationID: uuid.NewString(), ExpectedRevision: view("pages").Revision, Resource: "pages", Action: "delete", ID: p.ID}
	if _, e := s.Save(ctx, actor, deletePage); e != nil {
		t.Fatal("page soft deletion", e)
	}
	config, e = s.Config(ctx, "/dk-en/about")
	if e != nil || config.Page != nil {
		t.Fatal("deleted page still published")
	}
	exec("UPDATE seo_command_state SET enabled=FALSE WHERE id=1")
	if r, e := s.Save(ctx, actor, deletePage); e != nil || !r.Replayed {
		t.Fatal("delete receipt recovery", e)
	}
	if _, e := s.Save(ctx, actor, page); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("deleted target save replay accepted", e)
	}
	// Revocation is checked again even for an existing successful receipt.
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='seo.write'", actor.ID)
	exec("UPDATE authorization_runtime_schema SET revision=revision+1 WHERE id=1")
	if _, e := s.Save(ctx, actor, in); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("revoked write permission replayed receipt", e)
	}
	if _, e := s.Directory(ctx, actor, "pages", 50, 0); e != nil {
		t.Fatal("read-only permission rejected", e)
	}
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	if _, e := s.Directory(ctx, actor, "pages", 50, 0); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("warehouse principal gained global SEO access", e)
	}
	t.Log("OWNED_SEO_COMMANDS_VERIFIED")
}
