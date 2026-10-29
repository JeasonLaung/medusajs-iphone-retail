package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedProductEdits(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned MySQL required")
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
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("owned fixture connection failed")
	}
	base, _ := db.DB()
	defer base.Close()
	if db.Exec("CREATE DATABASE product_edit_rehearsal").Error != nil {
		t.Fatal("owned fixture schema failed")
	}
	cfg.DBName = "product_edit_rehearsal"
	db, err = gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("owned fixture connection failed")
	}
	pool, _ := db.DB()
	defer pool.Close()
	pool.SetMaxOpenConns(10)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("fixture plan failed")
	}
	applied, err := schema.Apply(ctx, pool, plan)
	if err != nil || applied.Applied != 38 {
		t.Fatal("eleven migrations failed")
	}
	user := identity.User{ID: "edit-fixture-admin", Email: "edit@example.invalid", Name: "Edit", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: "artificial-hash", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Create(&user).Error != nil {
		t.Fatal("fixture account failed")
	}
	if db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, user.ID) }) != nil {
		t.Fatal("explicit fixture grants failed")
	}
	if db.Exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at,metadata) VALUES ('edit-fixture','Original','edit-fixture',0,'draft',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"unchanged"}')`).Error != nil {
		t.Fatal("fixture product failed")
	}
	h := catalog.Handler{DB: db}
	revision := func() string {
		var p catalog.EditableProduct
		if db.Table("catalog_products").Select("title,subtitle,description,status,updated_at").Where("id=?", "edit-fixture").Take(&p).Error != nil {
			t.Fatal("fixture read failed")
		}
		b, _ := json.Marshal(p)
		s := sha256.Sum256(b)
		return hex.EncodeToString(s[:])
	}
	auditCount := func() int64 {
		var n int64
		if db.Table("catalog_product_edits").Count(&n).Error != nil {
			t.Fatal("audit read failed")
		}
		return n
	}
	initial := revision()
	in := catalog.EditInput{OperationID: uuid.NewString(), ExpectedRevision: initial, Title: "Edited 中文", Status: "published"}
	result, err := h.EditBasics(ctx, "edit-fixture", user, in)
	if err != nil || result.Replayed || result.Revision == initial || result.Revision != revision() || auditCount() != 1 {
		t.Fatal("atomic edit/revision failed")
	}

	if db.Exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code=?", user.ID, "product.write").Error != nil {
		t.Fatal("permission revocation failed")
	}
	if _, err := h.EditBasics(ctx, "edit-fixture", user, in); !errors.Is(err, catalog.ErrEditDenied) || auditCount() != 1 {
		t.Fatal("revoked permission replay disclosed prior success")
	}
	if db.Exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT role_id,? FROM authorization_memberships WHERE user_id=?", "product.write", user.ID).Error != nil {
		t.Fatal("permission restoration failed")
	}
	replay, err := h.EditBasics(ctx, "edit-fixture", user, in)
	if err != nil || !replay.Replayed || replay.Revision != result.Revision || auditCount() != 1 {
		t.Fatal("same operation not replayed")
	}
	// Concurrent copies of one new operation must all observe one committed result.
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = result.Revision
	duplicateResults := make(chan catalog.EditResult, 8)
	duplicateErrors := make(chan error, 8)
	var duplicateWG sync.WaitGroup
	for i := 0; i < 8; i++ {
		duplicateWG.Add(1)
		go func() {
			defer duplicateWG.Done()
			r, err := h.EditBasics(ctx, "edit-fixture", user, duplicate)
			duplicateResults <- r
			duplicateErrors <- err
		}()
	}
	duplicateWG.Wait()
	close(duplicateResults)
	close(duplicateErrors)
	for err := range duplicateErrors {
		if err != nil {
			t.Fatal("concurrent duplicate failed")
		}
	}
	commits := 0
	for r := range duplicateResults {
		if !r.Replayed {
			commits++
		}
		if r.Revision != revision() {
			t.Fatal("duplicate result diverged")
		}
	}
	if commits != 1 || auditCount() != 2 {
		t.Fatal("concurrent duplicate committed more than once")
	}
	result.Revision = revision()
	collision := in
	collision.Title = "Collision"
	if _, err = h.EditBasics(ctx, "edit-fixture", user, collision); !errors.Is(err, catalog.ErrEditConflict) {
		t.Fatal("changed operation payload accepted")
	}
	stale := in
	stale.OperationID = uuid.NewString()
	if _, err = h.EditBasics(ctx, "edit-fixture", user, stale); !errors.Is(err, catalog.ErrEditConflict) {
		t.Fatal("stale revision accepted")
	}
	start := make(chan struct{})
	out := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			next := in
			next.OperationID = uuid.NewString()
			next.ExpectedRevision = result.Revision
			_, err := h.EditBasics(ctx, "edit-fixture", user, next)
			out <- err
		}()
	}
	close(start)
	wg.Wait()
	close(out)
	wins, conflicts := 0, 0
	for err := range out {
		if err == nil {
			wins++
		} else if errors.Is(err, catalog.ErrEditConflict) {
			conflicts++
		} else {
			t.Fatal("concurrent edit failed unexpectedly")
		}
	}
	if wins != 1 || conflicts != 7 || auditCount() != 3 {
		t.Fatal("concurrent edits overwrote each other")
	}
	// A real MySQL trigger forces audit failure after UPDATE; both must roll back.
	before := revision()
	if db.Exec(`CREATE TRIGGER reject_fixture_audit BEFORE INSERT ON catalog_product_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial audit failure'`).Error != nil {
		t.Fatal("audit failure setup failed")
	}
	next := in
	next.OperationID = uuid.NewString()
	next.ExpectedRevision = before
	next.Title = "Must roll back"
	if _, err = h.EditBasics(ctx, "edit-fixture", user, next); err == nil || revision() != before || auditCount() != 3 {
		t.Fatal("audit failure committed partial update")
	}
	if db.Exec("DROP TRIGGER reject_fixture_audit").Error != nil {
		t.Fatal("audit failure cleanup failed")
	}
	// Revocation/generation and command capability are checked inside the write transaction.
	if db.Model(&identity.User{}).Where("id=?", user.ID).Update("auth_version", 2).Error != nil {
		t.Fatal("fixture generation failed")
	}
	if _, err = h.EditBasics(ctx, "edit-fixture", user, next); !errors.Is(err, catalog.ErrEditDenied) {
		t.Fatal("stale account authority accepted")
	}
	user.AuthVersion = 2
	if db.Model(&identity.User{}).Where("id=?", user.ID).Update("enabled", false).Error != nil {
		t.Fatal("fixture revocation failed")
	}
	if _, err = h.EditBasics(ctx, "edit-fixture", user, next); !errors.Is(err, catalog.ErrEditDenied) {
		t.Fatal("disabled actor accepted")
	}
	if db.Model(&identity.User{}).Where("id=?", user.ID).Update("enabled", true).Error != nil {
		t.Fatal("fixture enable failed")
	}
	if db.Exec("UPDATE catalog_command_capabilities SET enabled=FALSE").Error != nil {
		t.Fatal("capability setup failed")
	}
	if _, err = h.EditBasics(ctx, "edit-fixture", user, next); !errors.Is(err, catalog.ErrSchema) {
		t.Fatal("disabled command accepted")
	}
	if revision() != before || auditCount() != 3 {
		t.Fatal("rejected edits changed product")
	}
	var metadata string
	if db.Raw("SELECT metadata FROM catalog_products WHERE id='edit-fixture'").Scan(&metadata).Error != nil || metadata != `{"private":"unchanged"}` {
		t.Fatal("unrelated field changed")
	}
	var audit struct{ BeforeJSON, AfterJSON, RequestJSON string }
	if db.Table("catalog_product_edits").Where("operation_id=?", in.OperationID).Take(&audit).Error != nil {
		t.Fatal("audit evidence missing")
	}
	var a, b catalog.EditableProduct
	if json.Unmarshal([]byte(audit.BeforeJSON), &a) != nil || json.Unmarshal([]byte(audit.AfterJSON), &b) != nil || a.Title != "Original" || b.Title != in.Title || b.Status != "published" || b.Subtitle != nil || b.Description != nil {
		t.Fatal("audit evidence incorrect")
	}
	t.Log(`PRODUCT_EDIT_REHEARSAL {"migrations":38,"concurrent_attempts":8,"concurrent_wins":1,"conflicts":7,"concurrent_duplicate_attempts":8,"concurrent_duplicate_commits":1,"audit_rows":3,"replay_single_commit":true,"audit_failure_rollback":true,"stale_authority_rejected":true,"revoked_action_replay_denied":true,"disabled_actor_rejected":true,"disabled_command_rejected":true,"unrelated_fields_preserved":true}`)
}
