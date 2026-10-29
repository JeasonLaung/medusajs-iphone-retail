package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedProductInventory(t *testing.T) {
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
	base, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("fixture connection failed")
	}
	defer base.Close()
	if _, err = base.ExecContext(ctx, "CREATE DATABASE product_inventory_rehearsal"); err != nil {
		t.Fatal("fixture schema failed")
	}
	cfg.DBName = "product_inventory_rehearsal"
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
	if err != nil || result.Applied != 48 {
		t.Fatal("twelve migrations failed")
	}
	if result, err = schema.Apply(ctx, pool, plan); err != nil || result.Skipped != 48 {
		t.Fatal("twelve migration replay failed")
	}
	if inventory.Ready(ctx, db) != nil {
		t.Fatal("inventory model not ready")
	}
	seedReviewedCatalogFixture(t, db)
	fingerprint := func() [32]byte {
		var evidence []json.RawMessage
		for _, table := range []string{"inventory_items", "inventory_locations", "catalog_variant_inventory_items", "inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("id ASC").Find(&rows).Error != nil {
				t.Fatal("inventory fingerprint unavailable")
			}
			b, _ := json.Marshal(rows)
			evidence = append(evidence, b)
		}
		b, _ := json.Marshal(evidence)
		return sha256.Sum256(b)
	}
	before := fingerprint()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/products/:id/variants/:variant_id/inventory", (inventory.Handler{DB: db}).AdminVariant)
	type response struct {
		Items           []inventory.Item
		WritesEnabled   bool `json:"writes_enabled"`
		ManageInventory bool `json:"manage_inventory"`
		AllowBackorder  bool `json:"allow_backorder"`
	}
	call := func(path string, status int) response {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatal("inventory HTTP status differs")
		}
		for _, secret := range []string{"inventory-secret", "warehouse-secret", "level-secret", "reservation-secret", "metadata", "raw_stocked_quantity", "created_by"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("inventory private fields escaped")
			}
		}
		var out response
		if status == 200 && json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("inventory response invalid")
		}
		return out
	}
	root := "/products/fixture-a/variants/fixture-v/inventory"
	out := call(root, 200)
	if len(out.Items) != 2 || out.WritesEnabled || !out.ManageInventory || out.AllowBackorder {
		t.Fatal("inventory mode or membership differs")
	}
	item := out.Items[0]
	if item.InventoryItemID != "fixture-stock" || item.RequiredQuantity != "2.500000000000000000" || !item.ItemAvailable || item.SKU == nil || *item.SKU != "0000123456789012345" || len(item.Levels) != 2 {
		t.Fatal("inventory requirement or SKU precision lost")
	}
	l := item.Levels[0]
	if l.StockedQuantity != "9007199254740993.000000000000000001" || l.ReservedQuantity != "9007199254740993.000000000000000000" || l.AvailableQuantity != "0.000000000000000001" || l.ReservationQuantity != "9007199254740992.000000000000000000" || l.Reconciled || l.ReservationCount != 1 {
		t.Fatal("inventory exact mismatch evidence changed")
	}
	if item.Levels[1].AvailableQuantity != "-1.000000000000000000" || !item.Levels[1].Reconciled || item.Levels[1].LocationName != nil {
		t.Fatal("negative balance or deleted location hidden")
	}
	if len(item.UnmatchedReservations) != 1 || item.UnmatchedReservations[0].Quantity != "0.500000000000000000" {
		t.Fatal("unmatched reservation lost")
	}
	if out.Items[1].ItemAvailable || out.Items[1].Title != nil || out.Items[1].Levels == nil {
		t.Fatal("deleted item relation hidden or leaked")
	}
	shared := call("/products/fixture-b/variants/fixture-incomplete/inventory", 200)
	if len(shared.Items) != 1 || shared.Items[0].InventoryItemID != item.InventoryItemID || shared.Items[0].RequiredQuantity != "1.000000000000000000" || !shared.AllowBackorder {
		t.Fatal("shared item or variant requirement lost")
	}
	for _, path := range []string{"/products/fixture-b/variants/fixture-v/inventory", "/products/fixture-a/variants/missing/inventory", "/products/fixture-deleted/variants/fixture-v/inventory"} {
		call(path, 404)
	}
	call(root+"?fields=*", 400)
	call(root+"?limit=1", 400)
	if fingerprint() != before {
		t.Fatal("inventory read changed evidence")
	}
	// Actual generated active uniqueness and physical FKs remain enabled.
	if db.Exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES ('fixture-duplicate','fixture-stock','fixture-location',0,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`).Error == nil {
		t.Fatal("duplicate active item/location accepted")
	}
	if db.Exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES ('fixture-orphan','missing','fixture-location',0,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`).Error == nil {
		t.Fatal("physical inventory orphan accepted")
	}
	if db.Exec("UPDATE inventory_runtime_schema SET writes_enabled=TRUE WHERE id=1").Error == nil {
		t.Fatal("generic inventory writes enabled")
	}
	verifyInventoryImport(t, ctx, pool)
	if db.Exec("UPDATE inventory_runtime_schema SET reads_enabled=FALSE WHERE id=1").Error != nil {
		t.Fatal("fixture read disable failed")
	}
	call(root, 503)
	if db.Exec("UPDATE inventory_runtime_schema SET reads_enabled=TRUE,model_sha256=REPEAT('0',64) WHERE id=1").Error != nil {
		t.Fatal("fixture drift setup failed")
	}
	call(root, 503)
	t.Log(`PRODUCT_INVENTORY_REHEARSAL {"migrations":48,"active_level_uniqueness":true,"physical_fk_enabled":true,"exact_quantity_difference":true,"reservation_mismatch_preserved":true,"negative_balance_retained":true,"shared_item_requirements":true,"soft_delete_filters":true,"unmatched_reservations_retained":true,"cross_product_rejected":true,"read_fingerprint_unchanged":true,"schema_drift_rejected":true,"inventory_writes_disabled":true}`)
}
