package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/promotion"
	"gorm.io/gorm"
)

func checkPromotionProducts(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("artificial promotion product setup failed")
		}
	}
	exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,weight,created_at,updated_at,metadata) VALUES('promo-facts-product','Private draft','promo-facts-product',TRUE,'draft',FALSE,1.25,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"not-rule-context"}')`)
	exec("INSERT INTO catalog_categories(id,name,description,handle,mpath,is_active,is_internal,`rank`,created_at,updated_at) VALUES('promo-facts-category','Internal','','promo-facts-category','promo-facts-category',FALSE,TRUE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec(`INSERT INTO catalog_tags(id,value,created_at,updated_at) VALUES('promo-facts-tag','Tag',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_category_products(product_id,category_id) VALUES('promo-facts-product','promo-facts-category')`)
	exec(`INSERT INTO catalog_product_tags(product_id,tag_id) VALUES('promo-facts-product','promo-facts-tag')`)
	read := func(ids []string, wantErr bool) map[string]catalog.PromotionProduct {
		t.Helper()
		var out map[string]catalog.PromotionProduct
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; out, err = catalog.ReadPromotionProducts(tx, ids); return err }, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
		if wantErr {
			if err == nil || out != nil {
				t.Fatal("invalid product facts returned partial projection")
			}
		} else if err != nil {
			t.Fatalf("promotion product read: %v", err)
		}
		return out
	}
	if out, err := catalog.ReadPromotionProducts(db, []string{"promo-facts-product"}); err == nil || out != nil {
		t.Fatal("autocommit promotion relations accepted")
	}
	got := read([]string{"promo-facts-product", "promo-facts-product", "absent-product"}, false)
	p := got["promo-facts-product"]
	if len(got) != 1 || !p.IsGiftcard || p.Weight == nil || string(*p.Weight) != "1.25" || p.TypeID != nil || p.CollectionID != nil || !reflect.DeepEqual(p.Categories, []catalog.PromotionRelation{{ID: "promo-facts-category"}}) || !reflect.DeepEqual(p.Tags, []catalog.PromotionRelation{{ID: "promo-facts-tag"}}) {
		t.Fatal("current private product relations missing or fabricated")
	}
	encoded, _ := json.Marshal(map[string]any{"product": p})
	if strings.Contains(string(encoded), "not-rule-context") || strings.Contains(string(encoded), "metadata") {
		t.Fatal("product metadata escaped internal rule projection")
	}
	for _, attribute := range []string{"items.product.categories.id", "items.product.tags.id", "items.product.is_giftcard", "items.product.weight"} {
		value, operator := "promo-facts-category", "in"
		switch attribute {
		case "items.product.tags.id":
			value = "promo-facts-tag"
		case "items.product.is_giftcard":
			value = "true"
		case "items.product.weight":
			value = "1.2"
			operator = "gt"
		}
		raw, _ := json.Marshal(value)
		valid, err := promotion.MatchRules(encoded, []promotion.Rule{{Attribute: attribute, Operator: operator, Values: []promotion.RuleValue{{Value: raw}}}}, "items")
		if err != nil || !valid {
			t.Fatal("SQL product projection incompatible with original rule paths")
		}
	}
	exec("UPDATE catalog_tags SET deleted_at=UTC_TIMESTAMP(6) WHERE id='promo-facts-tag'")
	if len(read([]string{"promo-facts-product"}, false)["promo-facts-product"].Tags) != 0 {
		t.Fatal("deleted tag relation survived")
	}
	exec("UPDATE catalog_tags SET deleted_at=NULL WHERE id='promo-facts-tag'")
	var before, after map[string]catalog.PromotionProduct
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		before, err = catalog.ReadPromotionProducts(tx, []string{"promo-facts-product"})
		if err != nil {
			return err
		}
		exec("UPDATE catalog_products SET weight=2.5 WHERE id='promo-facts-product'")
		after, err = catalog.ReadPromotionProducts(tx, []string{"promo-facts-product"})
		return err
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("product relations escaped caller snapshot")
	}
	exec("UPDATE catalog_products SET deleted_at=UTC_TIMESTAMP(6) WHERE id='promo-facts-product'")
	if len(read([]string{"promo-facts-product"}, false)) != 0 {
		t.Fatal("deleted product relation survived")
	}
	read([]string{"bad\nidentifier"}, true)
	exec("UPDATE catalog_runtime_schema SET reads_enabled=FALSE")
	read(nil, true)
	exec("UPDATE catalog_runtime_schema SET reads_enabled=TRUE")
	exec("DELETE FROM catalog_products WHERE id='promo-facts-product'")
	exec("DELETE FROM catalog_categories WHERE id='promo-facts-category'")
	exec("DELETE FROM catalog_tags WHERE id='promo-facts-tag'")
	t.Log(`PROMOTION_PRODUCTS_REHEARSAL {"actual_mysql":true,"internal_draft_and_category_relations_preserved":true,"missing_deleted_and_null_semantics":true,"numeric_weight":true,"original_rule_paths":true,"metadata_excluded":true,"caller_snapshot":true,"transaction_and_schema_gate":true,"cart_context_or_checkout_integrated":false}`)
}
