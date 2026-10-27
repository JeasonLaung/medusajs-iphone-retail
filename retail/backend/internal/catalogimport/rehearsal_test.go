package catalogimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	cataloghttp "github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type rehearsalLayout struct {
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	Primary []string `json:"primary_key"`
	Fields  []struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Type   string `json:"target_type"`
	} `json:"fields"`
	References []struct {
		Column       string `json:"column"`
		Parent       string `json:"parent_table"`
		ParentColumn string `json:"parent_column"`
	} `json:"references"`
}

// Fixed source, private tmpfs, no app DSN or source connection. Driver errors,
// row values, keys and fingerprints must never appear in test output.
func TestActualCatalogMySQL(t *testing.T) {
	if os.Getenv("CATALOG_MYSQL_REHEARSAL") != "1" {
		t.Skip("owned isolated real-data rehearsal required")
	}
	const snapshotHash = "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
	catalog, err := plans()
	if err != nil {
		t.Fatal("fixed catalog program unavailable")
	}
	data, err := os.ReadFile("/tmp/field-plans.json")
	if err != nil {
		t.Fatal("rehearsal field layout unavailable")
	}
	var manifest struct {
		Hashes map[string]string `json:"input_sha256"`
	}
	if json.Unmarshal(program, &manifest) != nil {
		t.Fatal("fixed catalog input hashes invalid")
	}
	digest := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
	if hex.EncodeToString(digest[:]) != manifest.Hashes["retail/backend/internal/rowimport/field_plans.json"] {
		t.Fatal("rehearsal layout differs from reviewed program")
	}
	var layouts struct {
		Tables []rehearsalLayout `json:"tables"`
	}
	if json.Unmarshal(data, &layouts) != nil || len(layouts.Tables) != 159 {
		t.Fatal("rehearsal layout invalid")
	}
	all := map[string]rehearsalLayout{}
	for _, p := range layouts.Tables {
		all[p.Source] = p
	}
	scope := map[string]bool{}
	for table := range catalog {
		scope[table] = true
	}
	for changed := true; changed; {
		changed = false
		for table := range scope {
			for _, r := range all[table].References {
				if !scope[r.Parent] {
					scope[r.Parent] = true
					changed = true
				}
			}
		}
	}
	if len(scope) != 45 {
		t.Fatal("catalog parent closure differs")
	}
	selected := map[string][]rowimport.SourceRow{}
	file, err := os.Open("/tmp/catalog-source.ndjson")
	if err != nil {
		t.Fatal("private catalog snapshot unavailable")
	}
	defer file.Close()
	verified, err := rowimport.VerifyStream(file, snapshotHash, func(table string, row rowimport.SourceRow) error {
		if scope[table] {
			selected[table] = append(selected[table], row)
		}
		return nil
	})
	if err != nil || len(verified.Tables) != 159 {
		t.Fatal("complete catalog source stream invalid")
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal("catalog relationship source unavailable")
	}
	relations, err := rowimport.VerifyRelationships(file, snapshotHash, rowimport.RelationLimits{MaxRows: 100000, KeyBytes: 64 << 20})
	missing := int64(0)
	for name, n := range relations.Relations.Missing {
		missing += n
		for table := range scope {
			if strings.HasPrefix(name, table+".") {
				t.Fatal("catalog dependency closure contains unresolved references")
			}
		}
	}
	if err != rowimport.ErrReferences || relations.Snapshot.SHA256 != snapshotHash || missing != 375 || len(relations.Relations.PhysicalMissing) != 0 {
		t.Fatal("global catalog reference evidence differs")
	}
	// Topologically order tables without disabling actual physical constraints.
	var order []string
	pending := map[string]bool{}
	for table := range scope {
		pending[table] = true
	}
	for len(pending) > 0 {
		var ready []string
		for table := range pending {
			blocked := false
			for _, r := range all[table].References {
				if r.Parent != table && pending[r.Parent] {
					blocked = true
				}
			}
			if !blocked {
				ready = append(ready, table)
			}
		}
		if len(ready) == 0 {
			t.Fatal("catalog table dependency cycle requires review")
		}
		sort.Strings(ready)
		for _, table := range ready {
			order = append(order, table)
			delete(pending, table)
		}
	}
	// Categories have self references: order actual parents, not source file order.
	for _, table := range order {
		for _, ref := range all[table].References {
			if ref.Parent != table {
				continue
			}
			index := map[string]rowimport.SourceRow{}
			for _, row := range selected[table] {
				key, err := importvalue.Text(row[ref.ParentColumn].Raw, 191)
				if err != nil {
					t.Fatal("self-reference parent invalid")
				}
				index[key] = row
			}
			state := map[string]byte{}
			var sorted []rowimport.SourceRow
			var visit func(string)
			visit = func(key string) {
				if state[key] == 2 {
					return
				}
				if state[key] == 1 {
					t.Fatal("catalog self-reference cycle")
				}
				row, ok := index[key]
				if !ok {
					t.Fatal("catalog self-reference parent absent")
				}
				state[key] = 1
				if !row[ref.Column].SQLNull {
					parent, err := importvalue.Text(row[ref.Column].Raw, 191)
					if err != nil {
						t.Fatal("catalog self-reference invalid")
					}
					visit(parent)
				}
				state[key] = 2
				sorted = append(sorted, row)
			}
			keys := make([]string, 0, len(index))
			for key := range index {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				visit(key)
			}
			selected[table] = sorted
		}
	}
	counts := map[string]int64{"source_verified_tables": 159, "source_verified_rows": relations.Relations.Rows, "unresolved_global_references": missing, "catalog_tables": 36, "dependency_tables": 9, "closure_reference_missing": 0}
	for _, table := range order {
		if _, ok := catalog[table]; ok {
			counts["catalog_rows"] += int64(len(selected[table]))
		} else {
			counts["dependency_rows"] += int64(len(selected[table]))
		}
	}
	if counts["catalog_rows"] != 21837 || counts["dependency_rows"] != 1722 {
		t.Fatal("catalog scope row counts differ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "catalog_rehearsal"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'", "sql_mode": "'STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("isolated catalog connection invalid")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("catalog embedded schema plan invalid")
	}
	created, err := schema.Apply(ctx, db, plan)
	if err != nil || created.Applied != 11 || schema.CheckApplied(ctx, db, plan) != nil {
		t.Fatal("catalog embedded migration failed")
	}
	repeated, err := schema.Apply(ctx, db, plan)
	if err != nil || repeated.Skipped != 11 {
		t.Fatal("catalog embedded migration replay failed")
	}
	counts["explicit_schema_migrations"] = 11
	var enabled int
	if db.QueryRowContext(ctx, "SELECT @@foreign_key_checks").Scan(&enabled) != nil || enabled != 1 {
		t.Fatal("catalog physical constraints must remain enabled")
	}
	count := func(table string) int64 {
		var n int64
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n) != nil {
			t.Fatal("catalog target count unavailable")
		}
		return n
	}
	aux := []string{"migration_catalog_rows", "unique_key_owners", "unique_key_buckets", "unique_key_claims", "unique_key_secret_scopes"}
	for _, table := range order {
		if count(all[table].Target) != 0 {
			t.Fatal("catalog target must be empty")
		}
	}
	for _, table := range aux {
		if count(table) != 0 {
			t.Fatal("catalog evidence target must be empty")
		}
	}
	parentSQL := func(tx *sql.Tx, layout rehearsalLayout, row rowimport.SourceRow, insert bool) error {
		bound, err := rowimport.Convert(layout.Source, row)
		if err != nil {
			return ErrPolicy
		}
		var names, markers, comparisons, where []string
		var pk []any
		for j, c := range layout.Fields {
			if bound.Columns[j] != c.Target {
				return ErrPolicy
			}
			quoted := "`" + c.Target + "`"
			names = append(names, quoted)
			markers = append(markers, "?")
			param := "?"
			if strings.HasPrefix(c.Type, "DECIMAL(38,18)") {
				param = "CAST(? AS DECIMAL(38,18))"
			}
			comparisons = append(comparisons, "("+quoted+" <=> "+param+")")
		}
		for _, name := range layout.Primary {
			for j, c := range layout.Fields {
				if c.Source == name {
					where = append(where, "`"+c.Target+"` = ?")
					pk = append(pk, bound.Values[j])
				}
			}
		}
		if insert {
			if _, err := tx.ExecContext(ctx, "INSERT INTO `"+layout.Target+"` ("+strings.Join(names, ",")+") VALUES("+strings.Join(markers, ",")+")", bound.Values...); err != nil {
				return ErrStorage
			}
		}
		var equal bool
		args := append(append([]any{}, bound.Values...), pk...)
		if tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM `"+layout.Target+"` WHERE "+strings.Join(where, " AND ")+" FOR UPDATE", args...).Scan(&equal) != nil || !equal {
			return ErrConflict
		}
		return nil
	}
	stage := func(insertParents, commit bool) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("catalog transaction unavailable")
		}
		defer tx.Rollback()
		for _, table := range order {
			for _, row := range selected[table] {
				if _, ok := catalog[table]; ok {
					err = Import(ctx, tx, snapshotHash, table, row)
				} else {
					err = parentSQL(tx, all[table], row, insertParents)
				}
				if err != nil {
					t.Fatal("actual catalog atomic import or readback failed: " + table)
				}
			}
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal("catalog commit or rollback failed")
		}
	}
	stage(true, false)
	for _, table := range order {
		if count(all[table].Target) != 0 {
			t.Fatal("actual catalog rollback left business rows")
		}
	}
	for _, table := range aux {
		if count(table) != 0 {
			t.Fatal("actual catalog rollback left evidence")
		}
	}
	counts["rollback_business_rows"] = 23559
	counts["rollback_provenance_rows"] = 21837
	stage(true, true)
	for _, table := range order {
		n := count(all[table].Target)
		if n != int64(len(selected[table])) {
			t.Fatal("catalog committed counts differ")
		}
		counts["committed_"+table] = n
	}
	if count("migration_catalog_rows") != 21837 {
		t.Fatal("catalog provenance count differs")
	}
	counts["committed_provenance_rows"] = 21837
	counts["committed_unique_claims"] = count("unique_key_claims")
	fingerprint := func() []byte {
		h := sha256.New()
		targets := map[string]string{}
		for _, table := range order {
			layout := all[table]
			var columns []string
			for _, primary := range layout.Primary {
				for _, f := range layout.Fields {
					if f.Source == primary {
						columns = append(columns, "`"+f.Target+"`")
					}
				}
			}
			targets[layout.Target] = strings.Join(columns, ",")
		}
		for table, sort := range map[string]string{"migration_catalog_rows": "source_table,identity_sha256", "unique_key_owners": "scope,owner", "unique_key_buckets": "scope,digest", "unique_key_claims": "scope,owner", "unique_key_secret_scopes": "scope"} {
			targets[table] = sort
		}
		var names []string
		for name := range targets {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, table := range names {
			write := func(v []byte) { h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(v)))); h.Write(v) }
			write([]byte(table))
			rows, err := db.QueryContext(ctx, "SELECT * FROM `"+table+"` ORDER BY "+targets[table])
			if err != nil {
				t.Fatal("private catalog fingerprint unavailable")
			}
			cols, err := rows.Columns()
			if err != nil {
				t.Fatal("private catalog columns unavailable")
			}
			for rows.Next() {
				values := make([][]byte, len(cols))
				dest := make([]any, len(cols))
				for j := range values {
					dest[j] = &values[j]
				}
				if rows.Scan(dest...) != nil {
					t.Fatal("private catalog fingerprint invalid")
				}
				h.Write([]byte{2})
				for _, v := range values {
					marker := byte(0)
					if v != nil {
						marker = 1
					}
					h.Write([]byte{marker})
					write(v)
				}
			}
			if rows.Err() != nil || rows.Close() != nil {
				t.Fatal("private catalog fingerprint incomplete")
			}
			h.Write([]byte{3})
		}
		return h.Sum(nil)
	}
	before := fingerprint()
	// Exercise the actual HTTP projection against all imported real products in
	// the reviewed schema. Responses remain inside this isolated test process.
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		t.Fatal("catalog HTTP connection unavailable")
	}
	if cataloghttp.Ready(ctx, gdb) != nil {
		t.Fatal("catalog read schema readiness failed")
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/products", (cataloghttp.Handler{DB: gdb}).AdminProducts)
	router.GET("/products/:id", (cataloghttp.Handler{DB: gdb}).AdminProduct)
	router.GET("/products/:id/variants", (cataloghttp.Handler{DB: gdb}).AdminVariants)
	router.GET("/products/:id/variants/:variant_id/prices", (cataloghttp.Handler{DB: gdb}).AdminVariantPrices)
	var expected int64
	if db.QueryRow("SELECT COUNT(*) FROM catalog_products WHERE deleted_at IS NULL").Scan(&expected) != nil {
		t.Fatal("catalog HTTP expected count unavailable")
	}
	seen := map[string]bool{}
	var first *cataloghttp.AdminProductSummary
	for offset := 0; int64(offset) < expected; offset += 20 {
		reply := httptest.NewRecorder()
		router.ServeHTTP(reply, httptest.NewRequest("GET", "/products?limit=20&offset="+strconv.Itoa(offset), nil))
		var response struct {
			Products []cataloghttp.AdminProductSummary `json:"products"`
			Count    int64                             `json:"count"`
		}
		if reply.Code != 200 || json.Unmarshal(reply.Body.Bytes(), &response) != nil || response.Count != expected || len(response.Products) == 0 {
			t.Fatal("reviewed catalog HTTP list failed")
		}
		for _, p := range response.Products {
			if seen[p.ID] {
				t.Fatal("catalog HTTP unstable pagination")
			}
			seen[p.ID] = true
			if first == nil {
				copy := p
				first = &copy
			}
			var title, handle, status string
			var variants int64
			if db.QueryRow("SELECT title,handle,status,(SELECT COUNT(*) FROM catalog_variants v WHERE v.product_id=p.id AND v.deleted_at IS NULL) FROM catalog_products p WHERE id=? AND deleted_at IS NULL", p.ID).Scan(&title, &handle, &status, &variants) != nil || p.Title != title || p.Handle != handle || p.Status != status || p.VariantCount != variants || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
				t.Fatal("catalog HTTP projection differs from real SQL")
			}
		}
	}
	if int64(len(seen)) != expected || first == nil {
		t.Fatal("catalog HTTP did not cover actual products")
	}
	reply := httptest.NewRecorder()
	router.ServeHTTP(reply, httptest.NewRequest("GET", "/products?q="+url.QueryEscape(first.Handle)+"&status="+url.QueryEscape(first.Status), nil))
	var filtered struct {
		Products []cataloghttp.AdminProductSummary `json:"products"`
	}
	if reply.Code != 200 || json.Unmarshal(reply.Body.Bytes(), &filtered) != nil {
		t.Fatal("catalog HTTP actual filters failed")
	}
	found := false
	for _, p := range filtered.Products {
		if p.ID == first.ID {
			found = true
		}
	}
	if !found || !bytes.Equal(before, fingerprint()) {
		t.Fatal("catalog HTTP query changed data or lost filter match")
	}
	counts["http_projected_actual_products"] = expected
	for id := range seen {
		detailReply := httptest.NewRecorder()
		router.ServeHTTP(detailReply, httptest.NewRequest("GET", "/products/"+url.PathEscape(id), nil))
		var detail struct {
			Product cataloghttp.AdminProductDetail `json:"product"`
		}
		if detailReply.Code != 200 || json.Unmarshal(detailReply.Body.Bytes(), &detail) != nil || detail.Product.ID != id || detail.Product.Options == nil || detail.Product.Images == nil {
			t.Fatal("actual catalog detail HTTP failed")
		}
		var imageCount int64
		if db.QueryRow("SELECT COUNT(*) FROM catalog_images WHERE product_id=? AND deleted_at IS NULL", id).Scan(&imageCount) != nil || int64(len(detail.Product.Images)) != imageCount {
			t.Fatal("actual catalog detail images differ")
		}
		for _, o := range detail.Product.Options {
			for _, v := range o.Values {
				var member int64
				if db.QueryRow("SELECT COUNT(*) FROM catalog_product_options po JOIN catalog_product_option_values pv ON pv.product_product_option_id=po.id AND pv.deleted_at IS NULL JOIN catalog_option_values v ON v.id=pv.product_option_value_id AND v.deleted_at IS NULL AND v.option_id=po.product_option_id WHERE po.product_id=? AND po.deleted_at IS NULL AND po.product_option_id=? AND v.id=?", id, o.ID, v.ID).Scan(&member) != nil || member == 0 {
					t.Fatal("actual catalog detail option membership differs")
				}
			}
		}
		variantIDs := map[string]bool{}
		for offset := 0; int64(offset) < detail.Product.VariantCount; offset += 100 {
			vr := httptest.NewRecorder()
			router.ServeHTTP(vr, httptest.NewRequest("GET", "/products/"+url.PathEscape(id)+"/variants?limit=100&offset="+strconv.Itoa(offset), nil))
			var response struct {
				Variants []cataloghttp.AdminVariant `json:"variants"`
				Count    int64                      `json:"count"`
			}
			if vr.Code != 200 || json.Unmarshal(vr.Body.Bytes(), &response) != nil || response.Count != detail.Product.VariantCount || len(response.Variants) == 0 {
				t.Fatal("actual catalog variant HTTP failed")
			}
			for _, v := range response.Variants {
				if v.ProductID != id || variantIDs[v.ID] || v.Options == nil || v.Images == nil || v.UnassignedValueIDs == nil {
					t.Fatal("actual catalog variant projection failed")
				}
				variantIDs[v.ID] = true
				for priceOffset := 0; ; priceOffset += 100 {
					w := httptest.NewRecorder()
					router.ServeHTTP(w, httptest.NewRequest("GET", "/products/"+url.PathEscape(id)+"/variants/"+url.PathEscape(v.ID)+"/prices?limit=100&offset="+strconv.Itoa(priceOffset), nil))
					var prices struct {
						Prices      []cataloghttp.AdminConfiguredPrice
						Count       int64
						PriceSetIDs []string `json:"price_set_ids"`
					}
					if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &prices) != nil || prices.PriceSetIDs == nil || prices.Prices == nil {
						t.Fatal("actual configured price HTTP failed")
					}
					var expectedPrices int64
					if db.QueryRow(`SELECT COUNT(*) FROM pricing_prices p WHERE p.deleted_at IS NULL AND p.price_set_id IN (SELECT vp.price_set_id FROM catalog_variant_price_sets vp JOIN pricing_price_sets s ON s.id=vp.price_set_id AND s.deleted_at IS NULL WHERE vp.variant_id=? AND vp.deleted_at IS NULL)`, v.ID).Scan(&expectedPrices) != nil || expectedPrices != prices.Count {
						t.Fatal("actual price membership count differs")
					}
					for _, p := range prices.Prices {
						var amount, currency, set string
						var min, max, list sql.NullString
						if db.QueryRow("SELECT amount,currency_code,price_set_id,min_quantity,max_quantity,price_list_id FROM pricing_prices WHERE id=? AND deleted_at IS NULL", p.ID).Scan(&amount, &currency, &set, &min, &max, &list) != nil || amount != p.Amount || currency != p.CurrencyCode || set != p.PriceSetID || min.Valid != (p.MinQuantity != nil) || max.Valid != (p.MaxQuantity != nil) || list.Valid != (p.PriceListID != nil) || (min.Valid && min.String != *p.MinQuantity) || (max.Valid && max.String != *p.MaxQuantity) || (list.Valid && list.String != *p.PriceListID) {
							t.Fatal("actual exact price SQL differs")
						}
						var rules int
						if db.QueryRow("SELECT COUNT(*) FROM pricing_price_rules WHERE price_id=? AND deleted_at IS NULL", p.ID).Scan(&rules) != nil || rules != len(p.Rules) {
							t.Fatal("actual price rule count differs")
						}
						counts["http_projected_actual_price_memberships"]++
					}
					if int64(priceOffset+len(prices.Prices)) >= prices.Count {
						break
					}
					if len(prices.Prices) == 0 {
						t.Fatal("actual price pagination stopped early")
					}
				}
				counts["http_priced_actual_variants"]++
				var title string
				var sku sql.NullString
				if db.QueryRow("SELECT title,sku FROM catalog_variants WHERE id=? AND product_id=? AND deleted_at IS NULL", v.ID, id).Scan(&title, &sku) != nil || title != v.Title || (sku.Valid != (v.SKU != nil)) || (sku.Valid && sku.String != *v.SKU) {
					t.Fatal("actual catalog variant SQL differs")
				}
				if v.ConfigurationComplete {
					counts["http_complete_actual_variants"]++
				} else {
					counts["http_incomplete_actual_variants"]++
				}
			}
		}
		if int64(len(variantIDs)) != detail.Product.VariantCount {
			t.Fatal("actual catalog variant pagination incomplete")
		}
		counts["http_detailed_actual_products"]++
		counts["http_projected_actual_variants"] += int64(len(variantIDs))
	}
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("actual catalog detail HTTP changed data")
	}
	counts["http_queries_preserved_full_fingerprint"] = 1
	stage(false, true)
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("actual catalog replay changed evidence")
	}
	counts["exact_replay_catalog_rows"] = 21837
	counts["exact_replay_dependency_rows"] = 1722
	counts["physical_foreign_key_checks_enabled"] = 1
	report, _ := json.Marshal(counts)
	t.Log("CATALOG_MYSQL_REHEARSAL " + string(report))
}
