package notificationmail

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestInheritedCallbackSignaturesBindRawBodyAndPath(t *testing.T) {
	now := time.Unix(1800000000, 0)
	stamp := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"event_id":"owned"}`)
	secret := "fixture-secret"
	path := "/api/webhooks/mail/owned/relayme"
	digest := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("POST\n" + path + "\n" + stamp + "\nnonce\n" + hex.EncodeToString(digest[:])))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !verifyRelayme("POST", path, body, stamp, "nonce", signature, secret, now) {
		t.Fatal("original Relayme signature rejected")
	}
	if verifyRelayme("POST", path+"wrong", body, stamp, "nonce", signature, secret, now) || verifyRelayme("POST", path, append(body, ' '), stamp, "nonce", signature, secret, now) || verifyRelayme("POST", path, body, stamp, "nonce", signature, secret, now.Add(6*time.Minute)) {
		t.Fatal("Relayme tampering/expiry accepted")
	}
	mac = hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("event." + stamp + "."))
	mac.Write(body)
	svix := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	key := "whsec_" + base64.StdEncoding.EncodeToString([]byte(secret))
	if !verifyResend(body, "event", stamp, "v0,ignored "+svix, key, now) {
		t.Fatal("original Resend signature rejected")
	}
	if verifyResend(body, "wrong", stamp, svix, key, now) || verifyResend(append(body, ' '), "event", stamp, svix, key, now) || verifyResend(body, "event", stamp, svix, key, now.Add(-6*time.Minute)) {
		t.Fatal("Resend tampering/expiry accepted")
	}
}
