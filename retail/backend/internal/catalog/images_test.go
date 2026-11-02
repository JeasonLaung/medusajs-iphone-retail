package catalog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

func imageInputFixture() ImageEditInput {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	zero := 0
	return ImageEditInput{OperationID: "d89473cd-86ce-4240-a1c7-d3c9842ea421", ExpectedRevision: strings.Repeat("0", 64), Images: []ImageEntry{}, ThumbnailIndex: &zero, Upload: &ImageUpload{Filename: "sample.png", MimeType: "image/png", Content: base64.StdEncoding.EncodeToString(b.Bytes())}}
}
func TestImageWireAndActualDecoder(t *testing.T) {
	good := imageInputFixture()
	b, _ := json.Marshal(good)
	if _, e := decodeImages(bytes.NewReader(b)); e != nil {
		t.Fatal("valid image rejected")
	}
	for _, raw := range []string{string(b) + "{}", strings.Replace(string(b), `"images":[]`, `"images":null`, 1), strings.Replace(string(b), `"filename":"sample.png"`, `"filename":"a","filename":"b"`, 1), strings.Replace(string(b), `"thumbnail_index":0`, `"thumbnail_index":1`, 1), strings.Replace(string(b), `"mime_type":"image/png"`, `"mime_type":"image/jpeg"`, 1), strings.Replace(string(b), `"filename":"sample.png"`, `"filename":"../private.png"`, 1)} {
		if _, e := decodeImages(strings.NewReader(raw)); e == nil {
			t.Fatal("invalid or ambiguous image accepted")
		}
	}
	bad := good
	copyUpload := *good.Upload
	bad.Upload = &copyUpload
	data, _ := base64.StdEncoding.DecodeString(good.Upload.Content)
	bad.Upload.Content = base64.StdEncoding.EncodeToString(data[:24])
	if _, e := validateImages(bad); e == nil {
		t.Fatal("truncated image header accepted")
	}
	bad.Upload.Content = base64.StdEncoding.EncodeToString([]byte(`<svg onload="alert(1)"></svg>`))
	if _, e := validateImages(bad); e == nil {
		t.Fatal("active content accepted")
	}
}

func TestCatalogUploadJPEGAndExistingPublicWebP(t *testing.T) {
	var b bytes.Buffer
	if jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil) != nil {
		t.Fatal("JPEG fixture failed")
	}
	if _, e := imageBytes(&ImageUpload{Filename: "image.jpg", MimeType: "image/jpeg", Content: base64.StdEncoding.EncodeToString(b.Bytes())}); e != nil {
		t.Fatal("valid JPEG rejected")
	}
	webp, e := os.ReadFile("../../../storefront/public/iphone-thumbnails/iphone-16/black.webp")
	if e != nil {
		t.Fatal("existing public WebP fixture unavailable")
	}
	if _, e := imageBytes(&ImageUpload{Filename: "black.webp", MimeType: "image/webp", Content: base64.StdEncoding.EncodeToString(webp)}); e != nil {
		t.Fatal("existing public WebP rejected")
	}
}
