package devices

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPhotoWire(t *testing.T) {
	in := PhotoInput{uuid.NewString(), "phone.png", "image/png", base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10})}
	raw, _ := json.Marshal(in)
	if _, e := decodePhoto(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{`{}`, string(raw) + `{}`, strings.Replace(string(raw), `"filename":`, `"filename":"x","filename":`, 1), strings.Replace(string(raw), `"phone.png"`, `null`, 1), strings.Replace(string(raw), `"filename":`, `"extra":1,"filename":`, 1)} {
		if _, e := decodePhoto(strings.NewReader(body)); e == nil {
			t.Fatal("accepted invalid photo request")
		}
	}
	p := Photo{"dphoto_" + uuid.NewString(), ""}
	p.URL = photoURL(p.ID)
	if !validPhotoRef(p) {
		t.Fatal("valid reference rejected")
	}
	p.URL = "https://example.invalid/" + p.ID
	if validPhotoRef(p) {
		t.Fatal("external photo accepted")
	}
	if validPhotoRef(Photo{ID: "x"}) {
		t.Fatal("short id accepted")
	}
}
func TestPhotoHeaders(t *testing.T) {
	for _, v := range []struct {
		data []byte
		mime string
	}{{[]byte{255, 216, 255}, "image/jpeg"}, {[]byte{137, 80, 78, 71, 13, 10, 26, 10}, "image/png"}, {[]byte("RIFF0000WEBP"), "image/webp"}, {[]byte("<svg/>"), ""}, {[]byte("GIF89a"), ""}, {nil, ""}} {
		if photoMIME(v.data) != v.mime {
			t.Fatal("unexpected photo header")
		}
	}
}
