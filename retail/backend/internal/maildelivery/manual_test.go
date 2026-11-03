package maildelivery

import (
	"bytes"
	"context"
	"encoding/json"
	message "github.com/emersion/go-message/mail"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManualSMTPMIMEAndRelaymeEnvelope(t *testing.T) {
	m := ManualMessage{To: []string{"to@example.invalid"}, Cc: []string{"cc@example.invalid"}, Bcc: []string{"secret@example.invalid"}, Subject: "回复附件", Text: "body <script>", Attachments: []Attachment{{Filename: "中文.txt", Content: "aGVsbG8="}}}
	sender, _ := NewSMTP(SMTPConfig{Host: "localhost", Port: 25, Encryption: "none"}, "sender@example.invalid", "")
	raw, e := sender.PrepareManual(m)
	if e != nil {
		t.Fatal(e)
	}
	var payload request
	_ = json.Unmarshal(raw, &payload)
	mime, e := smtpMessage("mail/mc_00000000-0000-4000-8000-000000000001", payload)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(mime, []byte("secret@example.invalid")) || !bytes.Contains(mime, []byte("Cc: cc@example.invalid")) {
		t.Fatal("Bcc leaked or Cc missing")
	}
	reader, e := message.CreateReader(bytes.NewReader(mime))
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	count := 0
	attachment := false
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		count++
		body, _ := io.ReadAll(part.Body)
		if h, ok := part.Header.(*message.AttachmentHeader); ok {
			name, _ := h.Filename()
			attachment = name == "中文.txt" && string(body) == "hello"
		}
	}
	if count != 3 || !attachment {
		t.Fatal("multipart alternatives or decoded attachment lost")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var p request
		if json.NewDecoder(r.Body).Decode(&p) != nil || len(p.Cc) != 1 || len(p.Bcc) != 1 || len(p.Attachments) != 1 {
			t.Error("gateway envelope lost")
		}
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"id":"fixture-accepted","object":"email","status":"accepted"}`)
	}))
	defer server.Close()
	relay, _ := NewRelayme(RelaymeConfig{APIURL: server.URL, KeyID: "id", Secret: "secret"}, "sender@example.invalid", "")
	prepared, _ := relay.PrepareManual(m)
	result := relay.Send(context.Background(), "mail/mc_00000000-0000-4000-8000-000000000001", prepared)
	if result.ID != "fixture-accepted" || calls != 1 {
		t.Fatal("manual gateway rejected")
	}
	m.Bcc = append(m.Bcc, m.To[0])
	if ValidateManual(m) == nil {
		t.Fatal("duplicate recipient accepted")
	}
	m.Bcc = nil
	m.Subject = "bad\r\nBcc: attacker"
	if ValidateManual(m) == nil {
		t.Fatal("header injection accepted")
	}
	m.Subject = "subject"
	m.Attachments[0].Filename = "../bad"
	if ValidateManual(m) == nil {
		t.Fatal("attachment path accepted")
	}
	m.Attachments = nil
	m.Text = strings.Repeat("x", 65537)
	if ValidateManual(m) == nil {
		t.Fatal("unbounded body accepted")
	}
}
