package identity_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"testing"
)

func checkDevicePhotos(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	h := devices.Handler{DB: db}
	ctx := t.Context()
	content, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=")
	in := devices.PhotoInput{OperationID: uuid.NewString(), Filename: "phone.png", MimeType: "image/png", Content: base64.StdEncoding.EncodeToString(content)}
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, r, e := h.SavePhoto(ctx, actor, in)
			if e != nil {
				failed.Store(true)
			} else if r {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if failed.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("photo upload concurrency failed")
	}
	var uploaded struct {
		Photo    devices.Photo
		Replayed bool
	}
	if json.Unmarshal(call("POST", "/api/admin/device-photos", body(in), 200), &uploaded) != nil || !uploaded.Replayed {
		t.Fatal("photo receipt absent")
	}
	if !bytes.Equal(call("GET", uploaded.Photo.URL, "", 200), content) {
		t.Fatal("served image bytes differ")
	}
	changed := in
	changed.Filename = "changed.png"
	call("POST", "/api/admin/device-photos", body(changed), 409)
	changed.OperationID = uuid.NewString()
	changed.MimeType = "image/jpeg"
	call("POST", "/api/admin/device-photos", body(changed), 400)
	changed = in
	changed.OperationID = uuid.NewString()
	changed.Content += "\n"
	call("POST", "/api/admin/device-photos", body(changed), 400)
	changed = in
	changed.OperationID = uuid.NewString()
	changed.Content = base64.StdEncoding.EncodeToString(append([]byte{255, 216, 255}, make([]byte, devices.MaxPhotoBytes)...))
	changed.MimeType = "image/jpeg"
	call("POST", "/api/admin/device-photos", body(changed), 400)
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("photo role missing")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("photo fixture failed")
		}
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='device.intake'", role)
	call("POST", "/api/admin/device-photos", body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'device.intake')", role)
	path := "/api/admin/products/intake-product/variants/intake-v/device-intake"
	read := func() devices.IntakeContext {
		var d devices.IntakeContext
		if json.Unmarshal(call("GET", path, "", 200), &d) != nil {
			t.Fatal("photo intake read")
		}
		return d
	}
	intake := devices.IntakeInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Devices: []devices.IntakeDevice{{Identity: devices.Identity{SerialNumber: "PHOTO-INTAKE", IMEI: "000000000000076"}, SourceID: "intake-source", Category: "sale", CurrencyCode: "eur", Photos: []devices.Photo{uploaded.Photo}}}}
	var saved devices.IntakeResult
	if json.Unmarshal(call("POST", path, body(intake), 200), &saved) != nil || len(saved.Devices) != 1 || len(saved.Devices[0].Photos) != 1 || saved.Devices[0].Photos[0] != uploaded.Photo {
		t.Fatal("photo intake result lost image")
	}
	found := false
	for _, d := range read().RecentDevices {
		if d.SerialNumber == "PHOTO-INTAKE" {
			found = len(d.Photos) == 1 && d.Photos[0] == uploaded.Photo
		}
	}
	if !found {
		t.Fatal("photo lost on refresh")
	}
	call("POST", path, body(intake), 200)
	forged := intake
	forged.OperationID = uuid.NewString()
	forged.ExpectedRevision = read().Revision
	forged.Devices = append([]devices.IntakeDevice(nil), intake.Devices...)
	forged.Devices[0].SerialNumber = "PHOTO-FORGED"
	forged.Devices[0].IMEI = "000000000000077"
	missing := "dphoto_" + uuid.NewString()
	forged.Devices[0].Photos = []devices.Photo{{ID: missing, URL: "/api/store/device-photos/" + missing}}
	call("POST", path, body(forged), 400)
	exec("UPDATE device_photos SET content=? WHERE id=?", []byte{255, 216, 255}, uploaded.Photo.ID)
	call("GET", uploaded.Photo.URL, "", 503)
	forged.Devices[0].Photos = []devices.Photo{uploaded.Photo}
	call("POST", path, body(forged), 503)
	exec("UPDATE device_photos SET content=? WHERE id=?", content, uploaded.Photo.ID)
	var count int64
	if db.Table("devices").Where("serial_number='PHOTO-FORGED'").Count(&count).Error != nil || count != 0 {
		t.Fatal("invalid photo intake left device")
	}
	t.Log(`DEVICE_PHOTOS_REHEARSAL {"concurrent_uploads":4,"single_creation":true,"immutable_replay":true,"revoked_replay_denied":true,"mime_and_size_verified":true,"unregistered_photo_denied":true,"corrupt_stored_bytes_denied":true,"intake_and_reload_preserve_photos":true,"public_bytes_equal_upload":true}`)
}
