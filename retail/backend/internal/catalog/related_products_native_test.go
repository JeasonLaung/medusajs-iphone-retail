package catalog

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func verifyRelatedProductCandidates(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("related fixture", e)
		}
	}
	exec("INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('related-channel','Related',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('related-private','Private',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES('related-store','Related','related-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_collections(id,title,handle,created_at,updated_at) VALUES('related-col','Related','related-col',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('related-other-col','Other','related-other-col',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	for _, id := range []string{"related-a", "related-b", "related-c"} {
		exec("INSERT INTO catalog_tags(id,value,created_at,updated_at) VALUES(?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, id)
	}
	add := func(id, col, tag, status, channel string, gift, deleted bool) {
		exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,collection_id,created_at,updated_at,deleted_at) VALUES(?,?,?, ?,?,TRUE,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),IF(?,UTC_TIMESTAMP(6),NULL))", id, "iPhone "+id, id, status, gift, col, deleted)
		exec("INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "channel-"+id, id, channel)
		exec("INSERT INTO catalog_product_tags(product_id,tag_id) VALUES(?,?)", id, tag)
	}
	add("related-source", "related-col", "related-a", "published", "related-channel", false, false)
	exec("INSERT INTO catalog_product_tags(product_id,tag_id) VALUES('related-source','related-b')")
	for i := 0; i < 52; i++ {
		tag := "related-a"
		if i%2 == 1 {
			tag = "related-b"
		}
		add(fmt.Sprintf("related-%02d", i), "related-col", tag, "published", "related-channel", false, false)
	}
	for _, item := range []struct {
		id, col, tag, status, channel string
		gift, deleted                 bool
	}{
		{"related-wrong-col", "related-other-col", "related-a", "published", "related-channel", false, false},
		{"related-wrong-tag", "related-col", "related-c", "published", "related-channel", false, false},
		{"related-gift", "related-col", "related-a", "published", "related-channel", true, false},
		{"related-draft", "related-col", "related-a", "draft", "related-channel", false, false},
		{"related-deleted", "related-col", "related-a", "published", "related-channel", false, true},
		{"related-foreign", "related-col", "related-a", "published", "related-private", false, false},
	} {
		add(item.id, item.col, item.tag, item.status, item.channel, item.gift, item.deleted)
	}
	h := StoreHandler{DB: db, StoreID: "related-store"}
	r := gin.New()
	r.GET("/products", h.Products)
	call := func(query string, status int, wantCount int64) []StoreProduct {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/products?"+query, nil))
		if w.Code != status {
			t.Fatalf("related HTTP %d: %s", w.Code, w.Body.String())
		}
		if status != 200 {
			return nil
		}
		var out struct {
			Products      []StoreProduct
			Count         int64
			Limit, Offset int
		}
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Count != wantCount || out.Limit != 50 || out.Offset != 0 {
			t.Fatal("related candidate count", w.Body.String())
		}
		return out.Products
	}
	q := "related_to=related-source&limit=50"
	rows := call(q, 200, 53)
	if len(rows) != 50 || rows[0].ID != "related-00" || rows[49].ID != "related-49" {
		t.Fatal("related first 50 candidate window")
	}
	exec("DELETE FROM catalog_product_tags WHERE product_id='related-source'")
	call(q, 200, 54)
	exec("INSERT INTO catalog_product_tags(product_id,tag_id) VALUES('related-source','related-a'),('related-source','related-b')")
	exec("UPDATE catalog_products SET collection_id=NULL WHERE id='related-source'")
	call(q, 200, 54)
	exec("UPDATE catalog_products SET collection_id='related-col' WHERE id='related-source'")
	exec("UPDATE catalog_tags SET deleted_at=UTC_TIMESTAMP(6) WHERE id='related-a'")
	call(q, 200, 27)
	exec("UPDATE catalog_products SET status='draft' WHERE id='related-source'")
	call(q, 404, 0)
	for _, invalid := range []string{"related_to=related-foreign&limit=50", "related_to=missing&limit=50"} {
		call(invalid, 404, 0)
	}
	call(q+"&sales_channel_id=related-private", 400, 0)
	t.Log(`RELATED_PRODUCTS_NATIVE {"collection_and_any_tag":true,"empty_tags_unconstrained":true,"null_collection_unconstrained":true,"gift_draft_deleted_foreign_excluded":true,"active_tags_only":true,"first_50_before_client_filter":true,"private_source_404":true}`)
}
