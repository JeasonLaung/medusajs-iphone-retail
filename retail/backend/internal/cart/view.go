package cart

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// Saved presentation belongs to the authorized draft, including retired SKUs.
// No live catalog join may erase an item the customer still needs to remove.
type ViewItem struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	ProductID     *string           `json:"product_id"`
	ProductTitle  *string           `json:"product_title"`
	ProductHandle *string           `json:"product_handle"`
	Thumbnail     *string           `json:"thumbnail"`
	VariantTitle  *string           `json:"variant_title"`
	OptionValues  map[string]string `json:"option_values" gorm:"-"`
}

type View struct {
	Amounts AmountDetail `json:"amounts"`
	Items   []ViewItem   `json:"items"`
}

func viewText(s string) bool {
	return len(s) <= 4096 && utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return r < 32 || r == 127 })
}

// ReadView keeps display facts and exact amounts in the same SQL snapshot.
// Existing amounts-only consumers keep their original response contract.
func ReadView(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference) (View, error) {
	var result View
	err := readCartSnapshot(ctx, db, store, ref, func(tx *gorm.DB, cart Snapshot) error {
		var err error
		result.Amounts, err = assembleAmounts(tx, cart)
		if err != nil {
			return err
		}
		var rows []struct {
			ViewItem
			VariantOptionValues *string
		}
		if tx.Table("cart_line_items").Select("id,title,product_id,product_title,product_handle,thumbnail,variant_title,variant_option_values").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("created_at DESC,id ASC").Limit(1001).Scan(&rows).Error != nil || len(rows) != len(result.Amounts.Lines) {
			return ErrSchema
		}
		wanted := map[string]bool{}
		for _, line := range result.Amounts.Lines {
			wanted[line.ID] = true
		}
		result.Items = make([]ViewItem, 0, len(rows))
		for _, row := range rows {
			if !wanted[row.ID] || !viewText(row.Title) {
				return ErrSchema
			}
			delete(wanted, row.ID)
			if row.ProductID != nil && !idPattern.MatchString(*row.ProductID) {
				return ErrSchema
			}
			for _, s := range []*string{row.ProductTitle, row.ProductHandle, row.Thumbnail, row.VariantTitle} {
				if s != nil && !viewText(*s) {
					return ErrSchema
				}
			}
			row.OptionValues = map[string]string{}
			if row.VariantOptionValues != nil {
				if len(*row.VariantOptionValues) > 32768 || json.Unmarshal([]byte(*row.VariantOptionValues), &row.OptionValues) != nil || row.OptionValues == nil || len(row.OptionValues) > 64 {
					return ErrSchema
				}
				for k, v := range row.OptionValues {
					if !viewText(k) || !viewText(v) {
						return ErrSchema
					}
				}
			}
			result.Items = append(result.Items, row.ViewItem)
		}
		// Match the browser's bounded response instead of returning a partial cart.
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) > 4*1024*1024 {
			return ErrSchema
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return result, nil
}
