package catalogimport

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	cataloghttp "github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/gin-gonic/gin"
)

func checkActualStorePrices(t *testing.T, router *gin.Engine, counts map[string]int64) {
	t.Helper()
	data, err := os.ReadFile("/tmp/store-prices-expected.json")
	var oracle struct {
		SnapshotSHA string `json:"snapshot_sha256"`
		Contexts    []struct {
			RegionID string `json:"region_id"`
			Quantity string
			Variants map[string]*pricing.Quote
		}
	}
	if err != nil || json.Unmarshal(data, &oracle) != nil || oracle.SnapshotSHA != "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985" || len(oracle.Contexts) != 2 {
		t.Fatal("private actual store price oracle unavailable")
	}
	for _, ctx := range oracle.Contexts {
		var count int64 = -1
		for offset := 0; count < 0 || int64(offset) < count; offset += 5 {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/public-products?limit=5&offset=%d&region_id=%s&quantity=%s", offset, url.QueryEscape(ctx.RegionID), ctx.Quantity), nil))
			var page struct {
				Products     []cataloghttp.StoreProduct
				Count        int64
				Capabilities map[string]bool
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || !page.Capabilities["pricing"] || page.Capabilities["availability"] || page.Capabilities["checkout"] {
				t.Fatal("actual regional product price page failed")
			}
			if count >= 0 && count != page.Count {
				t.Fatal("actual regional product count changed")
			}
			count = page.Count
			for _, p := range page.Products {
				for _, v := range p.Variants {
					var expected *pricing.Quote
					if v.ConfigurationComplete {
						expected = ctx.Variants[v.ID]
					} else {
						counts["http_public_unquoted_incomplete_variant_contexts"]++
					}
					a, _ := json.Marshal(expected)
					b, _ := json.Marshal(v.CalculatedPrice)
					if string(a) != string(b) {
						t.Fatal("actual regional quote differs from installed legacy service")
					}
					if expected != nil {
						counts["http_public_actual_quoted_variant_contexts"]++
					} else {
						counts["http_public_actual_unquoted_variant_contexts"]++
					}
				}
			}
		}
		counts["http_public_actual_pricing_contexts"]++
	}
}
