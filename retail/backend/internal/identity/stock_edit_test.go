package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedInventoryStockEdits(t *testing.T) {
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
		t.Fatal("stock fixture connection failed")
	}
	base, _ := db.DB()
	defer base.Close()
	if db.Exec("CREATE DATABASE inventory_stock_edit_rehearsal").Error != nil {
		t.Fatal("stock fixture schema failed")
	}
	cfg.DBName = "inventory_stock_edit_rehearsal"
	db, err = gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("stock fixture connection failed")
	}
	pool, _ := db.DB()
	defer pool.Close()
	pool.SetMaxOpenConns(10)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("stock fixture plan failed")
	}
	applied, err := schema.Apply(ctx, pool, plan)
	if err != nil || applied.Applied != 42 {
		t.Fatal("stock fixture migrations failed")
	}
	seedReviewedCatalogFixture(t, db)
	user := identity.User{ID: "stock-fixture-admin", Email: "stock@example.invalid", Name: "Stock", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: "artificial-hash", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Create(&user).Error != nil {
		t.Fatal("stock fixture account failed")
	}
	if db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, user.ID) }) != nil {
		t.Fatal("explicit fixture grants failed")
	}
	for _, q := range []string{
		`INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES ('stock-edit-location','Stock edit location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at,metadata) VALUES ('stock-edit-level','fixture-stock','stock-edit-location',100,0,1.25,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"unchanged"}')`,
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("stock level fixture failed")
		}
	}
	h := inventory.Handler{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/products/:id/variants/:variant_id/inventory", h.AdminVariant)
	level := func(id string) inventory.Level {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/products/fixture-a/variants/fixture-v/inventory", nil))
		var response struct {
			Items            []inventory.Item
			StockEditEnabled bool `json:"stock_edit_enabled"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || !response.StockEditEnabled {
			t.Fatal("stock edit read model failed")
		}
		for _, item := range response.Items {
			for _, l := range item.Levels {
				if l.ID == id {
					return l
				}
			}
		}
		t.Fatal("stock fixture level absent")
		return inventory.Level{}
	}
	auditCount := func() int64 {
		var n int64
		if db.Table("inventory_stock_edits").Count(&n).Error != nil {
			t.Fatal("stock audit unavailable")
		}
		return n
	}
	in := inventory.StockInput{OperationID: uuid.NewString(), ExpectedRevision: level("stock-edit-level").Revision, StockedQuantity: "99.000000000000000001", IncomingQuantity: "2.000000000000000000"}
	result, err := h.EditStock(ctx, "stock-edit-level", user, in)
	if err != nil || result.Replayed || result.Revision != level("stock-edit-level").Revision || auditCount() != 1 {
		t.Fatal("exact stock edit failed")
	}

	if db.Exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code=?", user.ID, "inventory.stock.write").Error != nil {
		t.Fatal("permission revocation failed")
	}
	if _, err := h.EditStock(ctx, "stock-edit-level", user, in); !errors.Is(err, inventory.ErrStockDenied) || auditCount() != 1 {
		t.Fatal("revoked permission replay disclosed prior success")
	}
	if db.Exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT role_id,? FROM authorization_memberships WHERE user_id=?", "inventory.stock.write", user.ID).Error != nil {
		t.Fatal("permission restoration failed")
	}
	if again, err := h.EditStock(ctx, "stock-edit-level", user, in); err != nil || !again.Replayed || again.Revision != result.Revision || auditCount() != 1 {
		t.Fatal("stock replay duplicated edit")
	}
	stale := in
	stale.OperationID = uuid.NewString()
	if _, err = h.EditStock(ctx, "stock-edit-level", user, stale); !errors.Is(err, inventory.ErrStockConflict) {
		t.Fatal("stale stock version accepted")
	}
	reused := in
	reused.IncomingQuantity = "3.000000000000000000"
	if _, err = h.EditStock(ctx, "stock-edit-level", user, reused); !errors.Is(err, inventory.ErrStockConflict) {
		t.Fatal("stock operation reused with changed contents")
	}
	// Same revision, eight independent operations: exactly one commits.
	revision := level("stock-edit-level").Revision
	var wg sync.WaitGroup
	out := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			next := in
			next.OperationID = uuid.NewString()
			next.ExpectedRevision = revision
			next.StockedQuantity = "77.000000000000000001"
			_, err := h.EditStock(ctx, "stock-edit-level", user, next)
			out <- err
		}()
	}
	wg.Wait()
	close(out)
	wins, conflicts := 0, 0
	for err := range out {
		if err == nil {
			wins++
		} else if errors.Is(err, inventory.ErrStockConflict) {
			conflicts++
		} else {
			t.Fatal("stock concurrent dependency failure")
		}
	}
	if wins != 1 || conflicts != 7 || auditCount() != 2 {
		t.Fatal("stock concurrent edits lost updates")
	}
	// Eight deliveries of ONE operation: one audit, seven replays.
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = level("stock-edit-level").Revision
	duplicate.StockedQuantity = "55.000000000000000001"
	type duplicateOutcome struct {
		replay bool
		err    error
	}
	duplicateOut := make(chan duplicateOutcome, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := h.EditStock(ctx, "stock-edit-level", user, duplicate)
			duplicateOut <- duplicateOutcome{res.Replayed, err}
		}()
	}
	wg.Wait()
	close(duplicateOut)
	replays := 0
	for outcome := range duplicateOut {
		if outcome.err != nil {
			t.Fatal("duplicate stock dependency failure")
		}
		if outcome.replay {
			replays++
		}
	}
	if replays != 7 || auditCount() != 3 {
		t.Fatal("stock duplicate concurrency failed")
	}
	before := level("stock-edit-level")
	next := duplicate
	next.OperationID = uuid.NewString()
	next.ExpectedRevision = before.Revision
	next.StockedQuantity = "54.000000000000000001"
	if db.Exec(`CREATE TRIGGER reject_stock_audit BEFORE INSERT ON inventory_stock_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial audit failure'`).Error != nil {
		t.Fatal("stock audit failure setup failed")
	}
	if _, err = h.EditStock(ctx, "stock-edit-level", user, next); err == nil || level("stock-edit-level").Revision != before.Revision || auditCount() != 3 {
		t.Fatal("stock audit failure committed partial data")
	}
	if db.Exec("DROP TRIGGER reject_stock_audit").Error != nil {
		t.Fatal("stock audit failure cleanup failed")
	}
	if db.Exec(`INSERT INTO inventory_migration_holds(inventory_item_id,location_id,reason,snapshot_sha256,evidence_sha256,evidence_json,policy_version) VALUES ('fixture-stock','stock-edit-location','reservation_mismatch',REPEAT('a',32),REPEAT('b',32),'{}',1)`).Error != nil {
		t.Fatal("stock hold fixture failed")
	}
	if _, err = h.EditStock(ctx, "stock-edit-level", user, next); !errors.Is(err, inventory.ErrStockHeld) || level("stock-edit-level").Revision != before.Revision {
		t.Fatal("historical stock hold bypassed")
	}
	next.StockedQuantity = "56.000000000000000001"
	if _, err = h.EditStock(ctx, "stock-edit-level", user, next); err != nil || auditCount() != 4 {
		t.Fatal("held location stock increase rejected")
	}
	var holds int64
	if db.Table("inventory_migration_holds").Count(&holds).Error != nil || holds != 1 {
		t.Fatal("stock increase cleared historical hold")
	}
	mismatch := in
	mismatch.OperationID = uuid.NewString()
	mismatch.ExpectedRevision = level("fixture-level").Revision
	mismatch.StockedQuantity = "9007199254740992.000000000000000001"
	if _, err = h.EditStock(ctx, "fixture-level", user, mismatch); !errors.Is(err, inventory.ErrStockHeld) {
		t.Fatal("live reservation mismatch bypassed")
	}
	if _, err = h.EditStock(ctx, "fixture-level-retired-location", user, in); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("deleted stock parent accepted")
	}
	if _, err = h.EditStock(ctx, "fixture-level-deleted", user, in); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("deleted stock level accepted")
	}
	var stored struct{ StockedQuantity, ReservedQuantity, IncomingQuantity, RawStockedQuantity, RawIncomingQuantity, Metadata string }
	if db.Table("inventory_levels").Where("id=?", "stock-edit-level").Take(&stored).Error != nil || stored.StockedQuantity != "56.000000000000000001" || stored.ReservedQuantity != "0.000000000000000000" || stored.IncomingQuantity != "2.000000000000000000" || stored.Metadata != `{"private":"unchanged"}` {
		t.Fatal("stock exact or unrelated fields changed")
	}
	for _, raw := range []string{stored.RawStockedQuantity, stored.RawIncomingQuantity} {
		var n struct {
			Value     string
			Precision int
		}
		if json.Unmarshal([]byte(raw), &n) != nil || n.Precision != 38 {
			t.Fatal("stock raw quantity pair invalid")
		}
	}
	if db.Model(&identity.User{}).Where("id=?", user.ID).Update("auth_version", 2).Error != nil {
		t.Fatal("stock authority fixture failed")
	}
	if _, err = h.EditStock(ctx, "stock-edit-level", user, in); !errors.Is(err, inventory.ErrStockDenied) {
		t.Fatal("stale stock authority accepted")
	}
	user.AuthVersion = 2
	if db.Model(&identity.User{}).Where("id=?", user.ID).Update("enabled", false).Error != nil {
		t.Fatal("stock actor disable failed")
	}
	if _, err = h.EditStock(ctx, "stock-edit-level", user, in); !errors.Is(err, inventory.ErrStockDenied) {
		t.Fatal("disabled stock actor accepted")
	}
	if db.Model(&identity.User{}).Where("id=?", user.ID).Update("enabled", true).Error != nil || db.Exec("UPDATE inventory_command_capabilities SET enabled=FALSE").Error != nil {
		t.Fatal("stock command fixture failed")
	}
	if _, err = h.EditStock(ctx, "stock-edit-level", user, in); !errors.Is(err, inventory.ErrSchema) {
		t.Fatal("disabled stock command accepted")
	}
	if auditCount() != 4 {
		t.Fatal("denied stock edit created audit")
	}
	t.Log(`STOCK_EDIT_REHEARSAL {"migrations":42,"exact_decimal":true,"concurrent_attempts":8,"concurrent_wins":1,"conflicts":7,"duplicate_attempts":8,"duplicate_replays":7,"audit_rows":4,"audit_failure_rollback":true,"historical_hold_enforced":true,"live_mismatch_enforced":true,"held_increase_preserves_hold":true,"reservation_and_metadata_preserved":true,"deleted_parent_rejected":true,"stale_authority_rejected":true,"revoked_action_replay_denied":true,"disabled_command_rejected":true}`)
}
