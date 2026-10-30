package catalogimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkActualBuybackPrices(t *testing.T, ctx context.Context, db *sql.DB, gdb *gorm.DB, router *gin.Engine, sourceRows []rowimport.SourceRow, counts map[string]int64) {
	t.Helper()
	data, err := os.ReadFile("/tmp/buyback-expected.json")
	var oracle struct {
		SHA    string `json:"snapshot_sha256"`
		Rows   int    `json:"source_rows"`
		Active int    `json:"active_rows"`
		Prices []struct {
			Product string          `json:"product_id"`
			Prices  []buyback.Price `json:"prices"`
		} `json:"prices"`
		Estimates []struct {
			Input    json.RawMessage `json:"input"`
			Expected json.RawMessage `json:"expected"`
		} `json:"estimates"`
	}
	if err != nil || json.Unmarshal(data, &oracle) != nil || oracle.SHA != "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985" || oracle.Rows != 61 || len(oracle.Prices) == 0 || len(oracle.Estimates) != oracle.Active*4 {
		t.Fatal("private actual buyback oracle unavailable")
	}
	var store string
	if db.QueryRowContext(ctx, "SELECT id FROM commerce_stores WHERE deleted_at IS NULL").Scan(&store) != nil {
		t.Fatal("actual buyback store binding unavailable")
	}
	(buyback.Store{DB: gdb, StoreID: store}).Register(router.Group("/api/store"))
	var active int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM buyback_prices WHERE deleted_at IS NULL").Scan(&active) != nil || active != oracle.Active {
		t.Fatal("actual buyback soft deletion differs")
	}
	for _, expected := range oracle.Prices {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/api/store/buyback/prices?product_id="+url.QueryEscape(expected.Product), nil))
		var reply struct {
			Prices []buyback.Price `json:"prices"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &reply) != nil || !reflect.DeepEqual(reply.Prices, expected.Prices) {
			t.Fatal("actual imported buyback public prices differ from source")
		}
		counts["http_actual_buyback_product_price_lists"]++
	}
	for _, expected := range oracle.Estimates {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/store/buyback/estimate", bytes.NewReader(expected.Input))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		var actual, want any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &actual) != nil || json.Unmarshal(expected.Expected, &want) != nil || !reflect.DeepEqual(actual, want) {
			t.Fatal("actual imported buyback estimate differs from original function")
		}
		counts["http_actual_buyback_original_estimates"]++
	}
	counts["buyback_imported_rows"] = int64(oracle.Rows)
	counts["buyback_active_rows"] = int64(oracle.Active)
	if len(sourceRows) != oracle.Rows {
		t.Fatal("actual buyback source row coverage differs")
	}
	for _, mode := range []string{"target_drift", "source_drift", "snapshot_drift"} {
		func() {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal("buyback conflict transaction unavailable")
			}
			defer tx.Rollback()
			row := rowimport.SourceRow{}
			for k, v := range sourceRows[0] {
				row[k] = v
			}
			sha := oracle.SHA
			switch mode {
			case "target_drift":
				id, err := importvalue.Text(row["id"].Raw, 191)
				if err != nil {
					t.Fatal("buyback conflict identity invalid")
				}
				if _, err = tx.ExecContext(ctx, "UPDATE buyback_prices SET currency_code='conflict' WHERE id=?", id); err != nil {
					t.Fatal("buyback drift setup failed")
				}
			case "source_drift":
				cell := row["currency_code"]
				cell.Raw = json.RawMessage(`"conflict"`)
				row["currency_code"] = cell
			case "snapshot_drift":
				sha = "a58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
			}
			if !errors.Is(Import(ctx, tx, sha, "buyback_price", row), ErrConflict) {
				t.Fatal("buyback import drift was not rejected")
			}
			if tx.Rollback() != nil {
				t.Fatal("buyback conflict rollback failed")
			}
			counts["buyback_import_rejected_"+mode]++
		}()
	}
}
