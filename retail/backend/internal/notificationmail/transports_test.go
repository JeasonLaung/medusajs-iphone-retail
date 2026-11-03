package notificationmail

import (
	"encoding/json"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"strings"
	"testing"
)

func TestProtocolSecretsProjectionAndPreservation(t *testing.T) {
	previous := TransportConfig{Provider: "smtp", SMTP: &maildelivery.SMTPConfig{Host: "smtp.example.invalid", Port: 465, Username: "fixture", Password: "smtp-secret", Encryption: "tls"}, IMAP: &maildelivery.IMAPConfig{Host: "imap.example.invalid", Port: 993, Username: "fixture", Password: "imap-secret", Encryption: "tls"}}
	next := publicTransport(previous)
	raw, _ := json.Marshal(next)
	if strings.Contains(string(raw), "smtp-secret") || strings.Contains(string(raw), "imap-secret") || !next.HasSMTPPassword || !next.HasIMAPPassword {
		t.Fatal("credential projection leaked or lost configured state")
	}
	if previous.SMTP.Password != "smtp-secret" || previous.IMAP.Password != "imap-secret" {
		t.Fatal("projection mutated live credentials")
	}
	merged := mergeTransport(next.TransportConfig, previous)
	if merged.SMTP.Password != "smtp-secret" || merged.IMAP.Password != "imap-secret" {
		t.Fatal("blank input lost credentials")
	}
	sender, e := makeSender(merged, "", "EuroRefurb", "sender@example.invalid", "")
	if e != nil || sender.SupportsIdempotency() {
		t.Fatal("saved SMTP sender unavailable")
	}
	changed := TransportConfig{Provider: "quguo", Relayme: &maildelivery.RelaymeConfig{APIURL: "https://gateway.example.invalid", KeyID: "fixture", Secret: "gateway-secret"}}
	if _, e = makeSender(changed, "", "EuroRefurb", "sender@example.invalid", ""); e != nil {
		t.Fatal(e)
	}
	projection := publicTransport(changed)
	raw, _ = json.Marshal(projection)
	if strings.Contains(string(raw), "gateway-secret") || !projection.HasRelaymeSecret {
		t.Fatal("Relayme secret projection wrong")
	}
	if mergeTransport(projection.TransportConfig, changed).Relayme.Secret != "gateway-secret" {
		t.Fatal("gateway blank secret lost")
	}
}
func TestStrictProtocolShape(t *testing.T) {
	for _, raw := range []string{`null`, `{"provider":"smtp"}`, `{"provider":"resend","other":true}`, `{"provider":"resend","provider":"smtp"}`, `{"provider":"smtp","smtp":{"host":"one","host":"two"}}`, `{"provider":"smtp","smtp":null}`, `{"provider":"resend","imap":{"password":null}}`, `{"provider":"resend","smtp":{}}`} {
		var config TransportConfig
		if decodeTransport([]byte(raw), &config) == nil {
			t.Fatal("ambiguous or malformed configuration accepted")
		}
	}
	var config TransportConfig
	if decodeTransport([]byte(`{"provider":"smtp","smtp":{"host":"smtp.example.invalid","port":465,"username":"","password":"","encryption":"tls","skip_tls_verify":false}}`), &config) != nil {
		t.Fatal("valid configuration rejected")
	}
}
