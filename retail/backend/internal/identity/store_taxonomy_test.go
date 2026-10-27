package identity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkStoreTaxonomy(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if db.Exec(sql, args...).Error != nil {
			t.Fatal("taxonomy fixture mutation failed")
		}
	}
	exec(`INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('public-main','Public',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-other','Private',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES('public-shop','Shop','public-main',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-shop','Other','public-other',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_collections(id,title,handle,metadata,created_at,updated_at) VALUES('col-public','Collection','public','{"private":"must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('col-foreign','Foreign','foreign',NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('col-empty','Empty','empty',NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,collection_id,created_at,updated_at,deleted_at) VALUES('public-a','Visible','visible',FALSE,'published',TRUE,'col-public',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('public-b','Foreign','foreign',FALSE,'published',TRUE,'col-foreign',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('public-draft','Draft','draft',FALSE,'draft',TRUE,'col-public',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('public-deleted','Deleted','deleted',FALSE,'published',TRUE,'col-public',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_channels(product_id,sales_channel_id,id,created_at,updated_at) VALUES('public-a','public-main','pc-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-a','public-other','pc-a2',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-b','public-other','pc-b',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-draft','public-main','pc-d',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-deleted','public-main','pc-x',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_categories(id,name,description,handle,mpath,is_active,is_internal,` + "`rank`" + `,parent_category_id,metadata,created_at,updated_at,deleted_at) VALUES('cat-root','Root','Description','phones','cat-root',TRUE,FALSE,2,NULL,'{"private":"must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('cat-child','Child','','phones/child','cat-root.cat-child',TRUE,FALSE,1,'cat-root',NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('cat-grand','Grand','','phones/child/grand','cat-root.cat-child.cat-grand',TRUE,FALSE,1,'cat-child',NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('cat-hidden','Private','','private','cat-hidden',TRUE,TRUE,0,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('cat-public-orphan','Independent','','independent','cat-hidden.cat-public-orphan',TRUE,FALSE,3,'cat-hidden',NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('cat-inactive','Inactive','','inactive','cat-inactive',FALSE,FALSE,0,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('cat-deleted','Deleted','','deleted','cat-deleted',TRUE,FALSE,0,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_category_products(product_id,category_id) VALUES('public-a','cat-root'),('public-b','cat-root'),('public-draft','cat-root'),('public-deleted','cat-root'),('public-a','cat-child')`)
	call := func(path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Cookie", "retail_admin_session=invalid")
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("taxonomy HTTP status %d expected %d", w.Code, status)
		}
		for _, s := range []string{"must-not-escape", "metadata", "payload", "external_id", "is_internal", "is_active", "sales_channel_id", "public-main", "public-other", "cat-hidden"} {
			if strings.Contains(w.Body.String(), s) {
				t.Fatal("taxonomy private fields escaped")
			}
		}
		if w.Header().Get("Set-Cookie") != "" || status == 200 && w.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" || status != 200 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("taxonomy identity/cache boundary failed")
		}
		return w.Body.Bytes()
	}
	fingerprint := func() []byte {
		t.Helper()
		out := []json.RawMessage{}
		for _, table := range []string{"catalog_products", "catalog_product_channels", "catalog_categories", "catalog_category_products", "catalog_collections", "commerce_stores", "commerce_sales_channels"} {
			var rows []map[string]any
			if db.Table(table).Order("1,2").Find(&rows).Error != nil {
				t.Fatal("taxonomy fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			out = append(out, b)
		}
		b, _ := json.Marshal(out)
		return b
	}
	checkStoreProducts(t, db, router)
	before := fingerprint()
	for _, binding := range []string{"", "missing-shop"} {
		isolated := gin.New()
		h := catalog.StoreHandler{DB: db, StoreID: binding}
		isolated.GET("/categories", h.Categories)
		reply := httptest.NewRecorder()
		isolated.ServeHTTP(reply, httptest.NewRequest("GET", "/categories", nil))
		if reply.Code != 503 {
			t.Fatal("missing explicit shop binding did not fail closed")
		}
	}
	var categories struct {
		Categories []catalog.StoreCategory `json:"product_categories"`
		Count      int
	}
	if json.Unmarshal(call("/api/store/product-categories", 200), &categories) != nil || categories.Count != 4 || len(categories.Categories) != 4 {
		t.Fatal("public category count incorrect")
	}
	var root, grand, orphan catalog.StoreCategory
	for _, c := range categories.Categories {
		switch c.ID {
		case "cat-root":
			root = c
		case "cat-grand":
			grand = c
		case "cat-public-orphan":
			orphan = c
		}
	}
	if root.ProductCount != 1 || len(root.Children) != 1 || root.Children[0].ID != "cat-child" || grand.ParentCategory == nil || grand.ParentCategory.ParentCategory == nil || grand.ParentCategory.ParentCategory.ID != "cat-root" || orphan.ParentCategory != nil || orphan.ParentCategoryID != nil {
		t.Fatal("category hierarchy or current-channel count incorrect")
	}
	var filtered struct {
		Categories []catalog.StoreCategory `json:"product_categories"`
		Count      int
	}
	if json.Unmarshal(call("/api/store/product-categories?handle=phones%2Fchild", 200), &filtered) != nil || filtered.Count != 1 || filtered.Categories[0].ID != "cat-child" {
		t.Fatal("slash handle lookup failed")
	}
	if json.Unmarshal(call("/api/store/product-categories?parent_category_id=null", 200), &filtered) != nil || filtered.Count != 2 {
		t.Fatal("public roots filter failed")
	}
	call("/api/store/product-categories?parent_category_id=cat-root", 200)
	if json.Unmarshal(call("/api/store/product-categories?limit=1&offset=2", 200), &filtered) != nil || filtered.Count != 4 || len(filtered.Categories) != 1 || filtered.Categories[0].ID != "cat-root" {
		t.Fatal("stable category pagination failed")
	}
	if json.Unmarshal(call("/api/store/product-categories?offset=99", 200), &filtered) != nil || len(filtered.Categories) != 0 || filtered.Categories == nil {
		t.Fatal("category empty page not an array")
	}
	var detail struct {
		Category catalog.StoreCategory `json:"product_category"`
	}
	if json.Unmarshal(call("/api/store/product-categories/cat-root", 200), &detail) != nil {
		t.Fatal("category detail failed")
	}
	a, _ := json.Marshal(root)
	b, _ := json.Marshal(detail.Category)
	if !bytes.Equal(a, b) {
		t.Fatal("category detail differs from list")
	}
	for _, id := range []string{"cat-hidden", "cat-inactive", "cat-deleted", "missing"} {
		call("/api/store/product-categories/"+id, 404)
	}
	var collections struct {
		Collections []catalog.StoreCollection
		Count       int
	}
	if json.Unmarshal(call("/api/store/collections", 200), &collections) != nil || collections.Count != 1 || collections.Collections[0].ID != "col-public" || collections.Collections[0].ProductCount != 1 {
		t.Fatal("collection current-channel count incorrect")
	}
	call("/api/store/collections/col-public", 200)
	for _, id := range []string{"col-foreign", "col-empty", "missing"} {
		call("/api/store/collections/"+id, 404)
	}
	for _, path := range []string{"/api/store/product-categories?fields=*", "/api/store/product-categories?sales_channel_id=public-other", "/api/store/product-categories?limit=0", "/api/store/product-categories?offset=01", "/api/store/product-categories?limit=1&limit=2", "/api/store/product-categories?bad%zz", "/api/store/product-categories/cat-root?limit=1", "/api/store/collections?parent_category_id=null"} {
		call(path, 400)
	}
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("taxonomy reads changed evidence")
	}
	exec("UPDATE catalog_product_channels SET deleted_at=UTC_TIMESTAMP(6) WHERE product_id='public-a' AND sales_channel_id='public-main'")
	call("/api/store/collections/col-public", 404)
	exec("UPDATE catalog_product_channels SET deleted_at=NULL WHERE product_id='public-a' AND sales_channel_id='public-main'")
	exec("UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='public-main'")
	call("/api/store/collections", 503)
	call("/api/store/product-categories", 503)
	exec("UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='public-main'")
	exec("UPDATE commerce_stores SET deleted_at=UTC_TIMESTAMP(6) WHERE id='public-shop'")
	call("/api/store/product-categories", 503)
	exec("UPDATE commerce_stores SET deleted_at=NULL WHERE id='public-shop'")
	exec("UPDATE catalog_categories SET parent_category_id='cat-grand' WHERE id='cat-root'")
	call("/api/store/product-categories", 503)
	exec("UPDATE catalog_categories SET parent_category_id=NULL WHERE id='cat-root'")
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("taxonomy failure checks changed evidence")
	}
	// The complete tree is bounded independently of page size. Never return a
	// plausible partial hierarchy or recurse through an unbounded ancestor path.
	stamp := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	rows := []map[string]any{}
	for i := 0; i < 997; i++ {
		id := fmt.Sprintf("overflow-category-%04d", i)
		rows = append(rows, map[string]any{"id": id, "name": "Overflow", "description": "", "handle": id, "mpath": id, "is_active": true, "is_internal": false, "rank": 0, "created_at": stamp, "updated_at": stamp})
	}
	if db.Table("catalog_categories").Create(rows).Error != nil {
		t.Fatal("taxonomy overflow seed failed")
	}
	call("/api/store/product-categories?limit=1", 503)
	exec("DELETE FROM catalog_categories WHERE id LIKE 'overflow-category-%'")
	for i := 0; i < 18; i++ {
		id := fmt.Sprintf("deep-category-%02d", i)
		var parent any
		if i > 0 {
			parent = fmt.Sprintf("deep-category-%02d", i-1)
		}
		exec("INSERT INTO catalog_categories(id,name,description,handle,mpath,is_active,is_internal,`rank`,parent_category_id,created_at,updated_at) VALUES(?,'Deep','',?,?,TRUE,FALSE,0,?,?,?)", id, id, id, parent, stamp, stamp)
	}
	call("/api/store/product-categories", 503)
	for i := 17; i >= 0; i-- {
		exec("DELETE FROM catalog_categories WHERE id=?", fmt.Sprintf("deep-category-%02d", i))
	}
	exec("UPDATE catalog_runtime_schema SET reads_enabled=FALSE WHERE id=1")
	call("/api/store/collections", 503)
	exec("UPDATE catalog_runtime_schema SET reads_enabled=TRUE WHERE id=1")
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("taxonomy bound checks changed evidence")
	}
	t.Log(`STORE_TAXONOMY_REHEARSAL {"explicit_store_binding":true,"unconfigured_store_503":true,"tree_size_and_depth_limits":true,"current_channel_only":true,"published_live_product_counts":true,"category_ancestry_and_children":true,"hidden_ancestor_omitted":true,"slash_handle_lookup":true,"stable_pagination":true,"exact_detail_projection":true,"unavailable_collection_404":true,"unknown_scope_filter_rejected":true,"disabled_channel_and_deleted_store_503":true,"cycle_rejected":true,"public_revalidation":true,"read_fingerprint_unchanged":true}`)
}
