package devices

import (
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
	"regexp"
	"strconv"
	"strings"
)

var warehouseModelPhrase = regexp.MustCompile(`\b(?:iphone|i)\s*([0-9]{1,2})\s*(pro\s*max|pm|pro|p|plus|mini|m)?\b`)
var warehouseModelTerm = regexp.MustCompile(`^(?:i|iphone)([0-9]{1,2})(pm|promax|p|pro|plus|mini|m)?$`)
var warehouseGradeTerm = regexp.MustCompile(`^([sabcd])(?:级)?$`)
var warehouseCapacityTerm = regexp.MustCompile(`^([0-9]+)(g|gb|t|tb)$`)
var warehouseBatteryTerm = regexp.MustCompile(`^[0-9]+%$`)
var warehouseColors = map[string]string{
	"黑":      "black",
	"黑色":     "black",
	"白":      "white",
	"白色":     "white",
	"蓝":      "blue",
	"蓝色":     "blue",
	"绿":      "green",
	"绿色":     "green",
	"紫":      "purple",
	"紫色":     "purple",
	"粉":      "pink",
	"粉色":     "pink",
	"红":      "red",
	"红色":     "red",
	"黄":      "yellow",
	"黄色":     "yellow",
	"金":      "gold",
	"金色":     "gold",
	"银":      "silver",
	"银色":     "silver",
	"橙":      "orange",
	"橙色":     "orange",
	"石墨":     "graphite",
	"石墨色":    "graphite",
	"午夜色":    "midnight",
	"星光色":    "starlight",
	"深空黑":    "space black",
	"暗紫色":    "deep purple",
	"远峰蓝":    "sierra blue",
	"苍岭绿":    "alpine green",
	"海蓝色":    "pacific blue",
	"原色钛金属":  "natural titanium",
	"黑色钛金属":  "black titanium",
	"白色钛金属":  "white titanium",
	"蓝色钛金属":  "blue titanium",
	"沙漠色钛金属": "desert titanium",
}

func warehouseSearchTerms(q string) []string {
	q = strings.ToLower(strings.TrimSpace(norm.NFKC.String(q)))
	q = warehouseModelPhrase.ReplaceAllStringFunc(q, func(v string) string {
		p := warehouseModelPhrase.FindStringSubmatch(v)
		return "i" + p[1] + strings.ReplaceAll(p[2], " ", "") + " "
	})
	return strings.Fields(q)
}
func warehouseVariantSearch(query *gorm.DB, q string) *gorm.DB {
	contains := func(v string) string {
		return "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(v) + "%"
	}
	option := "EXISTS(SELECT 1 FROM catalog_variant_options wo JOIN catalog_option_values wv ON wv.id=wo.option_value_id AND wv.deleted_at IS NULL WHERE wo.variant_id=v.id AND "
	for _, term := range warehouseSearchTerms(q) {
		if model := warehouseModelTerm.FindStringSubmatch(term); model != nil {
			suffix := map[string]string{"pm": " Pro Max", "promax": " Pro Max", "p": " Pro", "pro": " Pro", "plus": " Plus", "mini": " Mini", "m": " Mini"}[model[2]]
			query = query.Where("LOWER(p.title)=LOWER(?)", "iPhone "+model[1]+suffix)
			continue
		}
		exact := ""
		if grade := warehouseGradeTerm.FindStringSubmatch(term); grade != nil {
			exact = strings.ToUpper(grade[1])
		} else if capacity := warehouseCapacityTerm.FindStringSubmatch(term); capacity != nil {
			n, e := strconv.ParseUint(capacity[1], 10, 32)
			if e == nil {
				if strings.HasPrefix(capacity[2], "t") {
					n *= 1024
				}
				exact = strconv.FormatUint(n, 10) + " GB"
			}
		} else if warehouseBatteryTerm.MatchString(term) || term == "85" || term == "100" {
			n, e := strconv.ParseUint(strings.TrimSuffix(term, "%"), 10, 32)
			if e == nil {
				exact = strconv.FormatUint(n, 10) + "%"
			}
		} else {
			switch term {
			case "64", "128", "256", "512", "1024", "2048":
				exact = term + " GB"
			}
		}
		if exact != "" {
			query = query.Where(option+"wv.value=?)", exact)
			continue
		}
		value := term
		if color, ok := warehouseColors[term]; ok {
			value = color
		}
		pattern := contains(value)
		query = query.Where("(LOWER(v.title) LIKE ? ESCAPE '!' OR LOWER(v.sku) LIKE ? ESCAPE '!' OR LOWER(p.title) LIKE ? ESCAPE '!' OR "+option+"LOWER(wv.value) LIKE ? ESCAPE '!'))", pattern, pattern, pattern, pattern)
	}
	return query
}
