package buyback

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RequestItem struct {
	ID              string       `json:"id"`
	ProductID       string       `json:"product_id"`
	Model           string       `json:"model"`
	ModelName       string       `json:"model_name"`
	Capacity        string       `json:"capacity"`
	Assessment      Assessment   `json:"assessment"`
	ReferenceAmount *json.Number `json:"reference_amount"`
	FinalAmount     *json.Number `json:"final_amount"`
	InspectionNotes string       `json:"inspection_notes"`
	SerialNumber    string       `json:"serial_number"`
	IMEI            string       `json:"imei"`
	DeviceID        *string      `json:"device_id"`
}
type PreparedSubmission struct {
	Items           []RequestItem `json:"items"`
	ReferenceAmount *json.Number  `json:"reference_amount"`
}

// PrepareSubmission resolves original product/option eligibility and captures
// current reference prices in the caller's consistent transaction. It neither
// authenticates a customer nor writes/commits. The command must do those steps
// and deduplicate the submission before invoking this stage.
func PrepareSubmission(ctx context.Context, tx *gorm.DB, in Submission) (PreparedSubmission, error) {
	var out PreparedSubmission
	if tx == nil {
		return out, ErrInput
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return out, ErrInput
	}
	in, e = ParseSubmission(raw)
	if e != nil || catalog.Ready(ctx, tx) != nil {
		return out, ErrInput
	}
	tx = tx.WithContext(ctx)
	out.Items = []RequestItem{}
	for _, item := range in.Items {
		var products []struct{ ID, Title, Handle string }
		if tx.Table("catalog_products").Select("id,title,handle").Where("deleted_at IS NULL AND status='published' AND handle IN ?", []string{item.Model, "swappie-" + item.Model}).Order("id").Limit(3).Scan(&products).Error != nil || len(products) > 2 {
			return PreparedSubmission{}, ErrInput
		}
		var candidates []struct{ ID, Title, Handle string }
		for _, p := range products {
			if iphoneTitle(p.Title) {
				candidates = append(candidates, p)
			}
		}
		// A model resolving to multiple products is ambiguous; do not silently
		// snapshot a different phone according to database row order.
		if len(candidates) != 1 {
			return PreparedSubmission{}, ErrInput
		}
		product := candidates[0]
		var options []struct{ Title, Value string }
		if tx.Table("catalog_product_options po").Select("o.title,v.value").Joins("JOIN catalog_options o ON o.id=po.product_option_id AND o.deleted_at IS NULL").Joins("JOIN catalog_product_option_values pv ON pv.product_product_option_id=po.id AND pv.deleted_at IS NULL").Joins("JOIN catalog_option_values v ON v.id=pv.product_option_value_id AND v.option_id=o.id AND v.deleted_at IS NULL").Where("po.product_id=? AND po.deleted_at IS NULL", product.ID).Order("o.id,v.id").Limit(1001).Scan(&options).Error != nil || len(options) > 1000 {
			return PreparedSubmission{}, ErrInput
		}
		eligible := false
		for _, option := range options {
			if storageOption(option.Title) && NormalizeCapacity(option.Value) == NormalizeCapacity(item.Capacity) {
				eligible = true
				break
			}
		}
		if !eligible {
			return PreparedSubmission{}, ErrInput
		}
		// Complete submissions permit irrelevant skipped fields that the partial
		// public estimate parser does not; use the already validated assessment.
		estimate, e := estimateInTransaction(tx, product.ID, item.Capacity, item.Assessment)
		if e != nil {
			return PreparedSubmission{}, e
		}
		out.Items = append(out.Items, RequestItem{ID: "bbi_" + strings.ReplaceAll(uuid.NewString(), "-", ""), ProductID: product.ID, Model: item.Model, ModelName: product.Title, Capacity: item.Capacity, Assessment: item.Assessment, ReferenceAmount: estimate.Amount})
	}
	out.ReferenceAmount, e = sumReference(out.Items)
	if e != nil {
		return PreparedSubmission{}, e
	}
	return out, nil
}

// Match the original JS /^iPhone\b/i, whose word characters are ASCII here.
func iphoneTitle(title string) bool {
	if len(title) < 6 || !strings.EqualFold(title[:6], "iphone") {
		return false
	}
	if len(title) == 6 {
		return true
	}
	c := title[6]
	return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_')
}
func storageOption(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "storage") || strings.Contains(lower, "capacity") || strings.Contains(lower, "memory") || strings.Contains(title, "容量")
}
func sumReference(items []RequestItem) (*json.Number, error) {
	var sum float64
	for _, item := range items {
		if item.ReferenceAmount == nil {
			return nil, nil
		}
		n, e := strconv.ParseFloat(string(*item.ReferenceAmount), 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return nil, ErrInput
		}
		sum += n
	}
	// Original workflow rounds the final JS Number sum, not each phone.
	n := math.Round(sum*100) / 100
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil, ErrInput
	}
	amount := json.Number(strconv.FormatFloat(n, 'f', -1, 64))
	return &amount, nil
}
