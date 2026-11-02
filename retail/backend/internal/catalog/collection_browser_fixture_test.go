package catalog

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

// Owned native-browser fixture only. No application test route or main data.
func seedCollectionBrowser(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("collection fixture", e)
		}
	}
	exec("INSERT INTO catalog_collections(id,title,handle,created_at,updated_at) VALUES('joint-collection','Native phone collection','native-phones',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	for _, group := range []struct{ id, title string }{{"joint-storage", "Collection storage"}, {"joint-colour", "Collection colour"}} {
		exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES(?,?,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", group.id, group.title)
	}
	for _, v := range []struct{ id, group, value string }{{"joint-128", "joint-storage", "128 GB"}, {"joint-256", "joint-storage", "256 GB"}, {"joint-red", "joint-colour", "Red"}, {"joint-blue", "joint-colour", "Blue"}} {
		exec("INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", v.id, v.group, v.value)
	}
	for i := 0; i < 105; i++ {
		id := fmt.Sprintf("zz-collection-%03d", i)
		stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,collection_id,thumbnail,created_at,updated_at) VALUES(?,?,?,'published',FALSE,TRUE,'joint-collection','/iphone-thumbnails/iphone-16e/white.webp',?,?)", id, fmt.Sprintf("iPhone collection %03d", i), id, stamp, stamp)
		exec("INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES(?,?,'option-channel',?,?)", id+"-channel", id, stamp, stamp)
		for _, group := range []string{"joint-storage", "joint-colour"} {
			member := id + "-" + group
			exec("INSERT INTO catalog_product_options(id,product_id,product_option_id,created_at,updated_at) VALUES(?,?,?,?,?)", member, id, group, stamp, stamp)
			values := []string{"joint-128", "joint-256"}
			if group == "joint-colour" {
				values = []string{"joint-red", "joint-blue"}
			}
			for _, v := range values {
				exec("INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES(?,?,?,?,?)", member+"-"+v, member, v, stamp, stamp)
			}
		}
		choices := [][]string{{"joint-128", "joint-red"}, {"joint-256", "joint-blue"}}
		if i%2 == 0 {
			choices = append(choices, []string{"joint-128", "joint-blue"})
		}
		for j, values := range choices {
			variant := fmt.Sprintf("%s-v%d", id, j)
			amount := fmt.Sprintf("%d", 105-i+j)
			exec("INSERT INTO catalog_variants(id,product_id,title,sku,manage_inventory,allow_backorder,variant_rank,created_at,updated_at) VALUES(?,?,?,?,FALSE,FALSE,?,?,?)", variant, id, fmt.Sprintf("Collection SKU %d", j), variant, j, stamp, stamp)
			for _, v := range values {
				exec("INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES(?,?)", variant, v)
			}
			exec("INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES(?,?,?)", variant+"-set", stamp, stamp)
			exec("INSERT INTO catalog_variant_price_sets(id,variant_id,price_set_id,created_at,updated_at) VALUES(?,?,?,?,?)", variant+"-link", variant, variant+"-set", stamp, stamp)
			exec("INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES(?,?,'eur',?,?,0,?,?)", variant+"-price", variant+"-set", amount, fmt.Sprintf(`{"value":"%s","precision":20}`, amount), stamp, stamp)
		}
	}
	t.Log(`COLLECTION_BROWSER_FIXTURE {"products":105,"variants":263,"same_sku_blue_128_products":53,"working_set_limit":100,"real_sql":true}`)
}
