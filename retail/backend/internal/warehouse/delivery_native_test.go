package warehouse

import (
	"context"
	"errors"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestOwnedWindowsDeliveryRecovery(t *testing.T) {
	if os.Getenv("WINDOWS_DELIVERY_RECOVERY") != "1" {
		t.Skip("explicit owned Windows recovery only")
	}
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if e != nil || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || !regexp.MustCompile(`^retail_account_[0-9a-f]{12}$`).MatchString(cfg.DBName) || os.Getenv("APP_ENV") != "development" {
		t.Fatal("owned recovery database required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	location, set := os.Getenv("WINDOWS_DELIVERY_LOCATION"), "fuset_01M4FBP3P1T4ZAR854Y7NQ7ZZT"
	var actor identity.User
	if e = db.Table("admin_users").Where("id IN (SELECT actor_id FROM warehouse_changes WHERE location_id=?)", location).First(&actor).Error; e != nil {
		t.Fatal(e)
	}
	h := Handler{DB: db}
	ctx := context.Background()
	directory, e := h.ReadDelivery(ctx, actor, location)
	if e != nil {
		t.Fatal(e)
	}
	command := DeliveryCommand{uuid.NewString(), directory.Revision, location, set}
	var fresh, replay atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.LinkDelivery(ctx, actor, command)
			if e != nil {
				t.Error(e)
				return
			}
			if r.Replayed {
				replay.Add(1)
			} else {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if fresh.Load() != 1 || replay.Load() != 5 {
		t.Fatal("concurrent replay duplicated")
	}
	stale := command
	stale.OperationID = uuid.NewString()
	if _, e = h.LinkDelivery(ctx, actor, stale); !errors.Is(e, ErrConflict) {
		t.Fatal("stale revision accepted")
	}
	changed := command
	changed.SetID = "other"
	if _, e = h.LinkDelivery(ctx, actor, changed); !errors.Is(e, ErrConflict) {
		t.Fatal("changed receipt accepted")
	}
	var worker identity.User
	if e = db.Table("admin_users").Where("email LIKE 'local-warehouse-%@example.invalid'").First(&worker).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = h.ReadDelivery(ctx, worker, location); e == nil {
		t.Fatal("warehouse principal reached full-store settings")
	}
	// Restore a soft-deleted link in this disposable checkpoint. A failure after
	// its write must roll back both link and warehouse revision.
	if e = db.Exec("UPDATE fulfillment_location_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE stock_location_id=? AND fulfillment_set_id=?", location, set).Error; e != nil {
		t.Fatal(e)
	}
	directory, e = h.ReadDelivery(ctx, actor, location)
	if e != nil {
		t.Fatal(e)
	}
	next := DeliveryCommand{uuid.NewString(), directory.Revision, location, set}
	name := "owned:fail-delivery-receipt"
	if e = db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "warehouse_changes" {
			tx.AddError(errors.New("owned injected receipt failure"))
		}
	}); e != nil {
		t.Fatal(e)
	}
	if _, e = h.LinkDelivery(ctx, actor, next); e == nil {
		t.Fatal("injected failure accepted")
	}
	db.Callback().Create().Remove(name)
	after, e := h.ReadDelivery(ctx, actor, location)
	if e != nil || after.Revision != directory.Revision {
		t.Fatal("failed receipt advanced revision")
	}
	for _, s := range after.Sets {
		if s.ID == set && s.Linked {
			t.Fatal("failed receipt did not roll back link")
		}
	}
	if _, e = h.LinkDelivery(ctx, actor, next); e != nil {
		t.Fatal(e)
	}
	var count int64
	db.Table("fulfillment_location_sets").Where("stock_location_id=? AND fulfillment_set_id=? AND deleted_at IS NULL", location, set).Count(&count)
	if count != 1 {
		t.Fatal("soft-deleted relation not restored uniquely")
	}
	t.Log("DELIVERY_RECOVERY concurrent6/fresh1/replay5 stale/conflict/warehouse-denied rollback/soft-delete-restore passed")
}
