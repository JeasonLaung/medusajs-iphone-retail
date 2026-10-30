package buyback

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestPhotoOriginalContentAndStrictJSON(t *testing.T) {
	for _, v := range []struct {
		mime string
		data []byte
	}{{"image/jpeg", []byte{255, 216, 255}}, {"image/png", []byte{137, 80, 78, 71, 13, 10, 26, 10}}, {"image/webp", []byte("RIFF0000WEBP")}} {
		in := PhotoUpload{"bbi_fixture", "front", v.mime, base64.StdEncoding.EncodeToString(v.data)}
		raw, _ := json.Marshal(in)
		parsed, e := ParsePhotoUpload(raw)
		if e != nil {
			t.Fatal(e)
		}
		data, e := photoData(parsed)
		if e != nil || !bytes.Equal(data, v.data) {
			t.Fatal("format changed")
		}
		in.MimeType = "image/gif"
		if _, e := photoData(in); e == nil {
			t.Fatal("MIME mismatch accepted")
		}
	}
	for _, content := range []string{"/9j/", "/9j/=", "/9j/==", "/9j/A", "/9j/AA", "/9j/AAA", "/9j/AB=="} {
		if _, e := photoData(PhotoUpload{"item", "front", "image/jpeg", content}); e != nil {
			t.Fatalf("original Buffer-compatible input rejected: %s", content)
		}
	}
	trimmed, e := ParsePhotoUpload([]byte(`{"item_id":"  item  ","photo_type":"front","mime_type":"image/jpeg","content":"/9j/"}`))
	if e != nil || trimmed.ItemID != "item" {
		t.Fatal("original trimmed ID normalization")
	}
	raw := `{"item_id":"item","photo_type":"front","mime_type":"image/jpeg","content":"/9j/"}`
	for _, bad := range []string{strings.Replace(raw, `"item_id":"item"`, `"item_id":"item","item_id":"other"`, 1), strings.Replace(raw, "item_id", "ITEM_ID", 1), strings.Replace(raw, `"item"`, `null`, 1), strings.Replace(raw, "front", "front|back", 1), raw + "{}", strings.Replace(raw, "/9j/", "data:image/jpeg;base64,/9j/", 1), strings.Replace(raw, `"content":`, `"extra":true,"content":`, 1)} {
		if _, e := ParsePhotoUpload([]byte(bad)); e == nil {
			t.Fatal("malformed photo accepted")
		}
	}
	data := make([]byte, MaxPhotoBytes)
	copy(data, []byte{255, 216, 255})
	if _, e := photoData(PhotoUpload{"item", "front", "image/jpeg", base64.StdEncoding.EncodeToString(data)}); e != nil {
		t.Fatal("3 MiB photo rejected")
	}
	data = append(data, 0)
	if _, e := photoData(PhotoUpload{"item", "front", "image/jpeg", base64.StdEncoding.EncodeToString(data)}); e == nil {
		t.Fatal("oversized photo accepted")
	}
}
