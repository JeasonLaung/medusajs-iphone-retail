package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func createInputFixture() ProductCreateInput {
	return ProductCreateInput{OperationID: "d89473cd-86ce-4240-a1c7-d3c9842ea421", Title: "测试商品", Handle: "native-create", Status: "draft", SalesChannelIDs: []string{"channel-fixture"}, ShippingProfileID: "profile-fixture", CategoryIDs: []string{}, ImageURLs: []string{}, Options: []ProductCreateOption{{Title: "容量", Values: []string{"128 GB", "256 GB"}}}}
}
func TestProductCreationWireRejectsAmbiguity(t *testing.T) {
	in := createInputFixture()
	body, _ := json.Marshal(in)
	if _, e := decodeProductCreate(strings.NewReader(string(body))); e != nil {
		t.Fatal("valid bilingual product rejected")
	}
	for _, body := range []string{
		strings.Replace(string(body), `"title":"测试商品"`, `"title":"a","title":"b"`, 1),
		strings.Replace(string(body), `"values":["128 GB","256 GB"]`, `"values":["a"],"values":["b"]`, 1),
		strings.Replace(string(body), `"category_ids":[]`, `"category_ids":null`, 1),
		strings.Replace(string(body), `"handle":"native-create"`, `"handle":"../private"`, 1),
		strings.Replace(string(body), `"128 GB","256 GB"`, `"128 GB","128 GB"`, 1),
		strings.Replace(string(body), `"image_urls":[]`, `"image_urls":["https://user:password@example.test/a"]`, 1),
		string(body) + `{}`, strings.Repeat(" ", 262145),
	} {
		if _, e := decodeProductCreate(strings.NewReader(body)); e == nil {
			t.Fatal("ambiguous or invalid product accepted")
		}
	}
}
