package importer

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Row map[string]json.RawMessage
type Bundle struct {
	FormatVersion int              `json:"format_version"`
	SourceCommit  string           `json:"source_commit"`
	Tables        map[string][]Row `json:"tables"`
}

func str(r Row, k string) string { var s string; _ = json.Unmarshal(r[k], &s); return s }
func text(r Row, k string) *string {
	if len(r[k]) == 0 || string(r[k]) == "null" {
		return nil
	}
	s := str(r, k)
	return &s
}
func boolean(r Row, k string) bool { var b bool; _ = json.Unmarshal(r[k], &b); return b }
func timestamp(r Row, k string) *time.Time {
	s := str(r, k)
	if s == "" {
		return nil
	}
	v, e := time.Parse(time.RFC3339Nano, s)
	if e != nil {
		return nil
	}
	return &v
}
func created(r Row, k string) time.Time {
	t := timestamp(r, k)
	if t == nil {
		return time.Unix(0, 0).UTC()
	}
	return *t
}
func numeric(r Row, k string) *string {
	v := strings.TrimSpace(string(r[k]))
	if v == "" || v == "null" {
		return nil
	}
	if strings.HasPrefix(v, "\"") {
		s := str(r, k)
		return &s
	}
	return &v
}
func encode(r any) json.RawMessage { data, _ := json.Marshal(r); return data }
func appendRows(r Row, key string, rows []Row) {
	if rows == nil {
		rows = []Row{}
	}
	r[key] = encode(rows)
}

// ImportCatalog preserves the complete source bundle on disk, but imports only
// the catalog/region domain. Other domains must not be marked migrated yet.
func ImportCatalog(db *gorm.DB, path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Bundle
	if err = json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	if b.FormatVersion != 1 || len(b.SourceCommit) != 40 {
		return nil, errors.New("unsupported source bundle")
	}
	for _, table := range []string{"product", "product_variant", "price", "product_category", "region"} {
		if _, ok := b.Tables[table]; !ok {
			return nil, fmt.Errorf("missing source table %s", table)
		}
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	counts := map[string]int{}
	err = db.Transaction(func(tx *gorm.DB) error {
		// Migration is deliberately restricted to an empty target catalog. A
		// repeated identical bundle is a no-op; it cannot overwrite later edits.
		var existing int64
		if err := tx.Table("migration_runs").Where("bundle_sha256=?", hash).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			counts["already_imported"] = 1
			return nil
		}
		if err := tx.Model(&catalog.Product{}).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return errors.New("target catalog is not empty; refusing to overwrite data")
		}
		images := group(b.Tables["image"], "product_id")
		optionValues := group(b.Tables["product_option_value"], "option_id")
		optionIndex := index(b.Tables["product_option"])
		optionsByProduct := map[string][]Row{}
		// Medusa stores product/option membership in a separate join table.
		for _, join := range b.Tables["product_product_option"] {
			if timestamp(join, "deleted_at") != nil {
				continue
			}
			pid := str(join, "product_id")
			option := clone(optionIndex[str(join, "product_option_id")])
			if len(option) > 0 {
				appendRows(option, "values", optionValues[str(option, "id")])
				optionsByProduct[pid] = append(optionsByProduct[pid], option)
			}
		}
		for _, r := range b.Tables["product"] {
			payload := clone(r)
			appendRows(payload, "images", images[str(r, "id")])
			appendRows(payload, "options", optionsByProduct[str(r, "id")])
			p := catalog.Product{ID: str(r, "id"), Handle: str(r, "handle"), Title: str(r, "title"), Status: str(r, "status"), Thumbnail: text(r, "thumbnail"), CollectionID: text(r, "collection_id"), Payload: encode(payload), CreatedAt: created(r, "created_at"), UpdatedAt: created(r, "updated_at"), DeletedAt: timestamp(r, "deleted_at")}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
			counts["products"]++
		}
		values := index(b.Tables["product_option_value"])
		variantOptions := group(b.Tables["product_variant_option"], "variant_id")
		for _, r := range b.Tables["product_variant"] {
			payload := clone(r)
			opts := []Row{}
			for _, join := range variantOptions[str(r, "id")] {
				value := clone(values[str(join, "option_value_id")])
				if len(value) > 0 {
					value["option"] = encode(optionIndex[str(value, "option_id")])
					opts = append(opts, value)
				}
			}
			appendRows(payload, "options", opts)
			v := catalog.Variant{ID: str(r, "id"), ProductID: str(r, "product_id"), SKU: text(r, "sku"), Title: str(r, "title"), Payload: encode(payload), CreatedAt: created(r, "created_at"), UpdatedAt: created(r, "updated_at"), DeletedAt: timestamp(r, "deleted_at")}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			counts["variants"]++
		}
		priceVariants := map[string][]string{}
		for _, r := range b.Tables["product_variant_price_set"] {
			if timestamp(r, "deleted_at") == nil {
				key := str(r, "price_set_id")
				priceVariants[key] = append(priceVariants[key], str(r, "variant_id"))
			}
		}
		priceRules := group(b.Tables["price_rule"], "price_id")
		for _, r := range b.Tables["price"] {
			for _, vid := range priceVariants[str(r, "price_set_id")] {
				amount := numeric(r, "amount")
				if amount == nil {
					return fmt.Errorf("price %s has no amount", str(r, "id"))
				}
				payload := clone(r)
				appendRows(payload, "rules", priceRules[str(r, "id")])
				var region *string
				for _, rule := range priceRules[str(r, "id")] {
					if str(rule, "attribute") == "region_id" {
						region = text(rule, "value")
					}
				}
				// A shared price set may belong to several variants; source ID stays in payload.
				p := catalog.Price{ID: "pr_" + fmt.Sprintf("%x", sha256.Sum256([]byte(str(r, "id")+":"+vid)))[:48], VariantID: vid, CurrencyCode: str(r, "currency_code"), Amount: *amount, RegionID: region, PriceListID: text(r, "price_list_id"), MinQuantity: numeric(r, "min_quantity"), MaxQuantity: numeric(r, "max_quantity"), Payload: encode(payload), DeletedAt: timestamp(r, "deleted_at")}
				if err := tx.Create(&p).Error; err != nil {
					return err
				}
				counts["prices"]++
			}
		}
		for _, r := range b.Tables["product_category"] {
			v := catalog.Category{ID: str(r, "id"), Handle: str(r, "handle"), Name: str(r, "name"), ParentCategoryID: text(r, "parent_category_id"), IsActive: boolean(r, "is_active"), IsInternal: boolean(r, "is_internal"), Payload: encode(r), DeletedAt: timestamp(r, "deleted_at")}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			counts["categories"]++
		}
		for _, r := range b.Tables["product_category_product"] {
			v := catalog.CategoryProduct{ProductID: str(r, "product_id"), CategoryID: str(r, "product_category_id")}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
		}
		for _, r := range b.Tables["product_collection"] {
			v := catalog.Collection{ID: str(r, "id"), Handle: str(r, "handle"), Title: str(r, "title"), Payload: encode(r), DeletedAt: timestamp(r, "deleted_at")}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			counts["collections"]++
		}
		countries := group(b.Tables["region_country"], "region_id")
		for _, r := range b.Tables["region"] {
			payload := clone(r)
			appendRows(payload, "countries", countries[str(r, "id")])
			v := catalog.Region{ID: str(r, "id"), Name: str(r, "name"), CurrencyCode: str(r, "currency_code"), Payload: encode(payload), DeletedAt: timestamp(r, "deleted_at")}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			counts["regions"]++
		}
		return tx.Table("migration_runs").Create(map[string]any{"id": "mig_" + uuid.NewString(), "bundle_sha256": hash, "source_commit": b.SourceCommit, "counts": encode(counts), "created_at": time.Now().UTC()}).Error
	})
	return counts, err
}
func group(rows []Row, key string) map[string][]Row {
	out := map[string][]Row{}
	for _, r := range rows {
		out[str(r, key)] = append(out[str(r, key)], r)
	}
	return out
}
func index(rows []Row) map[string]Row {
	out := map[string]Row{}
	for _, r := range rows {
		out[str(r, "id")] = r
	}
	return out
}
func clone(r Row) Row {
	out := Row{}
	for k, v := range r {
		out[k] = v
	}
	return out
}
