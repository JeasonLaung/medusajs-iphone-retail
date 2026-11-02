package catalog

import "testing"

func TestSourceIphoneImageKeyRules(t *testing.T) {
	options := []AdminOption{{ID: "color", Title: "颜色", Values: []AdminOptionValue{{ID: "red", Value: "Product (RED)"}}}}
	if got := iphoneVariantImageKey("swappie-iphone-15", "ignored", options, map[string]bool{"red": true}); got != "iphone-15/red" {
		t.Fatal("source model/color mapping differs", got)
	}
	if got := iphoneVariantImageKey("swappie-iphone-15", "ignored", options, map[string]bool{}); got != "" {
		t.Fatal("unselected color matched")
	}
	if parts := iphoneImagePattern.FindStringSubmatch("https://example.test/iphone-products/iphone-15/product-red/02.webp?size=600"); len(parts) != 3 || parts[1]+"/"+imageColor(parts[2]) != "iphone-15/red" {
		t.Fatal("source image URL mapping differs")
	}
	if iphoneImagePattern.MatchString("https://example.test/device-photos/other.webp") {
		t.Fatal("unrelated photo matched")
	}
}
