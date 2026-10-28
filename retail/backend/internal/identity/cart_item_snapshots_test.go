package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCartItemSnapshots(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("snapshot artificial fixture failed")
		}
	}
	exec(`UPDATE catalog_products SET title='New title',subtitle='New subtitle',description='New description',thumbnail='new-image',is_giftcard=TRUE WHERE id='product'`)
	exec(`UPDATE catalog_variants SET title='New variant',sku='000123',barcode='000456' WHERE id='v'`)
	exec(`UPDATE cart_line_items SET product_id='old-product',product_title=NULL,product_description=NULL,product_subtitle='',thumbnail='',variant_title=NULL,variant_sku=NULL,variant_barcode='',variant_option_values='{"Saved":"Option"}',metadata=NULL WHERE id='a'`)
	exec(`UPDATE cart_line_items SET product_title='Historical title',product_description='Historical description',product_handle='historical-handle',metadata='null' WHERE id='b'`)
	exec(`UPDATE cart_line_items SET is_giftcard=TRUE,metadata='{"manual":true}' WHERE id='manual'`)
	fingerprint := func() string {
		t.Helper()
		var rows []map[string]any
		if db.Table("cart_line_items").Order("id").Find(&rows).Error != nil {
			t.Fatal("snapshot evidence read failed")
		}
		b, _ := json.Marshal(rows)
		return string(b)
	}
	run := func(rollback bool) (int, error) {
		count := 0
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			count, err = cart.RefreshItemSnapshots(ctx, tx, "shop", ref, "1")
			if err != nil {
				return err
			}
			if rollback {
				return errors.New("artificial later domain failure")
			}
			return nil
		})
		return count, err
	}
	before := fingerprint()
	if _, err := cart.RefreshItemSnapshots(ctx, db, "shop", ref, "1"); err != cart.ErrInput {
		t.Fatal("autocommit snapshot refresh accepted")
	}
	if count, err := run(true); err == nil || count != 5 || before != fingerprint() {
		t.Fatal("later failure retained item snapshot changes")
	}
	exec(`CREATE TRIGGER snapshot_late_failure BEFORE UPDATE ON cart_line_items FOR EACH ROW BEGIN IF NEW.id='b' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial late failure'; END IF; END`)
	if _, err := run(false); err != cart.ErrStorage || before != fingerprint() {
		t.Fatal("snapshot late failure left partial fields")
	}
	exec(`DROP TRIGGER snapshot_late_failure`)
	if count, err := run(false); err != nil || count != 5 {
		t.Fatal("snapshot phase unavailable")
	}
	var a struct {
		Title, ProductID, ProductTitle, ProductDescription, ProductSubtitle, Thumbnail, VariantTitle, VariantSKU, VariantBarcode, VariantOptionValues, Metadata, RawUnitPrice string
		IsGiftcard, RequiresShipping, IsDiscountable                                                                                                                          bool
	}
	if db.Table("cart_line_items").Where("id='a'").Take(&a).Error != nil || a.Title != "Saved title" || a.ProductID != "product" || a.ProductTitle != "New title" || a.ProductDescription != "New description" || a.ProductSubtitle != "" || a.Thumbnail != "" || a.VariantTitle != "New variant" || a.VariantSKU != "000123" || a.VariantBarcode != "" || a.VariantOptionValues != "{\"Saved\":\"Option\"}" || a.Metadata != "{}" || !a.IsGiftcard || !a.RequiresShipping || a.IsDiscountable {
		t.Fatal("snapshot null fallback or saved explicit fields changed")
	}
	var b struct {
		ProductTitle, ProductDescription, ProductHandle, Metadata string
		IsGiftcard                                                bool
	}
	if db.Table("cart_line_items").Where("id='b'").Take(&b).Error != nil || b.ProductTitle != "Historical title" || b.ProductDescription != "Historical description" || b.ProductHandle != "historical-handle" || b.Metadata != "{}" || !b.IsGiftcard {
		t.Fatal("snapshot historical content overwritten")
	}
	var manual struct {
		Metadata   string
		IsGiftcard bool
	}
	if db.Table("cart_line_items").Where("id='manual'").Take(&manual).Error != nil || manual.IsGiftcard || manual.Metadata != "{\"manual\":true}" {
		t.Fatal("manual line defaults differ")
	}
	var removed struct {
		IsGiftcard   bool
		ProductTitle *string
	}
	if db.Table("cart_line_items").Where("id='removed'").Take(&removed).Error != nil || removed.IsGiftcard || removed.ProductTitle != nil {
		t.Fatal("deleted snapshot modified")
	}
	// Snapshot preparation must not overwrite the exact price phase output.
	var raw struct{ Value string }
	if json.Unmarshal([]byte(a.RawUnitPrice), &raw) != nil || raw.Value != "30" {
		t.Fatal("snapshot phase overwrote precise price")
	}
	before = fingerprint()
	exec(`UPDATE catalog_products SET status='draft' WHERE id='product'`)
	if _, err := run(false); err != cart.ErrItemUnavailable || before != fingerprint() {
		t.Fatal("unpublished snapshot changed saved fields")
	}
	exec(`UPDATE catalog_products SET status='published' WHERE id='product'`)
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := cart.RefreshItemSnapshots(ctx, tx, "shop", ref, "2"); return err }); err != cart.ErrConflict || before != fingerprint() {
		t.Fatal("stale snapshot version accepted")
	}
	t.Log(`CART_ITEM_SNAPSHOT_REHEARSAL {"null_fallback":true,"explicit_empty_preserved":true,"historical_content_preserved":true,"current_product_and_giftcard":true,"manual_defaults":true,"metadata_null_normalized":true,"price_and_options_preserved":true,"removed_rows_preserved":true,"whole_transaction_rollback":true,"late_failure_rollback":true,"unpublished_and_stale_rejected":true,"full_refresh":false}`)
}
