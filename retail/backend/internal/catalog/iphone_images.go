package catalog

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Source: Medusa lib/iphone-product-images.ts and its variant-created subscriber.
var imageSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)
var iphoneImagePattern = regexp.MustCompile(`/iphone-products/([^/]+)/([^/]+)/[^/?]+(?:\?.*)?$`)
var colorOptionPattern = regexp.MustCompile(`(?i)^(colou?r|颜色)$`)

func imageSlug(s string) string {
	return strings.Trim(imageSlugPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-"), "-")
}
func imageColor(s string) string {
	slug := imageSlug(s)
	if slug == "product-red" {
		return "red"
	}
	return slug
}
func iphoneVariantImageKey(handle, title string, options []AdminOption, selected map[string]bool) string {
	model := strings.TrimPrefix(handle, "swappie-")
	if model == "" {
		model = title
	}
	model = imageSlug(model)
	for _, o := range options {
		if colorOptionPattern.MatchString(o.Title) {
			for _, v := range o.Values {
				if selected[v.ID] && model != "" && v.Value != "" {
					return model + "/" + imageColor(v.Value)
				}
			}
			break
		}
	}
	return ""
}
func attachCreatedVariantImages(tx *gorm.DB, p, v, key string) error {
	if key == "" {
		return nil
	}
	images := []AdminImage{}
	if tx.Table("catalog_images").Select("id,url,`rank`").Where("product_id=? AND deleted_at IS NULL", p).Order("url,id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(1001).Scan(&images).Error != nil || len(images) > 1000 {
		return ErrSchema
	}
	matches := []AdminImage{}
	for _, i := range images {
		parts := iphoneImagePattern.FindStringSubmatch(i.URL)
		if len(parts) == 3 && parts[1]+"/"+imageColor(parts[2]) == key {
			matches = append(matches, i)
		}
	}
	for _, i := range matches {
		if e := tx.Exec("INSERT INTO catalog_variant_images(id,variant_id,image_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "vimg_"+uuid.NewString(), v, i.ID).Error; e != nil {
			return e
		}
	}
	if len(matches) > 0 {
		return tx.Table("catalog_variants").Where("id=?", v).Update("thumbnail", matches[0].URL).Error
	}
	return nil
}
