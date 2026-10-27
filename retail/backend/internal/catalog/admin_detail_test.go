package catalog

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminDetailInputBeforeDependencies(t *testing.T) {
	for _, id := range []string{"", strings.Repeat("x", 192), "bad\x00id", "bad\nID", string([]byte{0xff})} {
		if validProductID(id) {
			t.Fatal("invalid product identity accepted")
		}
	}
	if !validProductID("原商品-123") {
		t.Fatal("valid source identity rejected")
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := Handler{}
	r.GET("/products/:id", h.AdminProduct)
	r.GET("/products/:id/variants", h.AdminVariants)
	for path, want := range map[string]int{"/products/fixture?fields=*": 400, "/products/fixture/variants?limit=101": 400, "/products/fixture/variants?limit=1&limit=2": 400, "/products/fixture/variants?sort=title_asc": 400, "/products/fixture/variants?limit=%zz": 400, "/products/fixture": 503, "/products/fixture/variants": 503} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Fatal("detail query boundary differs")
		}
	}
}
