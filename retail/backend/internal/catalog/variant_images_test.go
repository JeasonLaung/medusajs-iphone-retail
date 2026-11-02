package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func variantImageFixture() VariantImageInput {
	return VariantImageInput{OperationID: "d89473cd-86ce-4240-a1c7-d3c9842ea421", ExpectedRevision: strings.Repeat("0", 64), ImageIDs: []string{"img-fixture"}, KeepThumbnail: true}
}
func TestVariantImageWireRejectsAmbiguity(t *testing.T) {
	in := variantImageFixture()
	raw, _ := json.Marshal(in)
	if _, e := decodeVariantImages(strings.NewReader(string(raw))); e != nil {
		t.Fatal("valid association rejected")
	}
	for _, body := range []string{string(raw) + "{}", strings.Replace(string(raw), `"keep_thumbnail":true`, `"keep_thumbnail":null`, 1), strings.Replace(string(raw), `"keep_thumbnail":true`, `"keep_thumbnail":true,"keep_thumbnail":false`, 1), strings.Replace(string(raw), `"image_ids":["img-fixture"]`, `"image_ids":null`, 1), strings.Replace(string(raw), `"image_ids":["img-fixture"]`, `"image_ids":["img-fixture","img-fixture"]`, 1), strings.Replace(string(raw), `"thumbnail_image_id":null`, `"thumbnail_image_id":"other-image"`, 1)} {
		if _, e := decodeVariantImages(strings.NewReader(body)); e == nil {
			t.Fatal("ambiguous SKU image assignment accepted")
		}
	}
}
