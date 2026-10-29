package catalog

import "strings"

// Catalog color aliases inherited from the original search projection.
var searchColorAliases = map[string]string{
	"alpine green":     "Green",
	"black titanium":   "Black",
	"blue titanium":    "Blue",
	"cloud white":      "White",
	"cosmic orange":    "Orange",
	"deep blue":        "Blue",
	"deep purple":      "Purple",
	"desert titanium":  "Gold",
	"graphite":         "Gray",
	"grey":             "Gray",
	"lavender":         "Purple",
	"light gold":       "Gold",
	"midnight":         "Black",
	"midnight green":   "Green",
	"mist blue":        "Blue",
	"natural titanium": "Gray",
	"pacific blue":     "Blue",
	"sage":             "Green",
	"sierra blue":      "Blue",
	"sky blue":         "Blue",
	"soft pink":        "Pink",
	"space black":      "Black",
	"space gray":       "Gray",
	"space grey":       "Gray",
	"starlight":        "White",
	"teal":             "Green",
	"ultramarine":      "Blue",
	"white titanium":   "White",
	"(product)red":     "Red",
}

func searchColorFamily(name string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(name)), " ")
	for _, color := range []string{"Black", "Blue", "Gold", "Gray", "Green", "Orange", "Pink", "Purple", "Red", "Silver", "White", "Yellow"} {
		if strings.ToLower(color) == normalized {
			return color
		}
	}
	return searchColorAliases[normalized]
}
