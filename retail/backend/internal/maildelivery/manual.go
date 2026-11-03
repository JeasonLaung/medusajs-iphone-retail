package maildelivery

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Manual messages inherit the original mail center's To/Cc/Bcc and attachments.
// The bounded payload can be stored in one authenticated encrypted envelope.
type Attachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}
type ManualMessage struct {
	To          []string     `json:"to"`
	Cc          []string     `json:"cc"`
	Bcc         []string     `json:"bcc"`
	Subject     string       `json:"subject"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
}

func validateManual(m ManualMessage) error {
	if len(m.To) == 0 || len(m.To)+len(m.Cc)+len(m.Bcc) > 50 || len(m.Attachments) > 10 || len(m.Subject) > 1024 || strings.TrimSpace(m.Subject) == "" || !safeSetting(m.Subject, 1024) || !utf8.ValidString(m.Subject) || len(m.Text) > 65536 || strings.TrimSpace(m.Text) == "" || !utf8.ValidString(m.Text) {
		return ErrConfig
	}
	seen := map[string]bool{}
	for _, group := range [][]string{m.To, m.Cc, m.Bcc} {
		for _, v := range group {
			a, e := mail.ParseAddress(v)
			key := strings.ToLower(v)
			if e != nil || a.Name != "" || a.Address != v || !safeSetting(v, 254) || seen[key] {
				return ErrConfig
			}
			seen[key] = true
		}
	}
	total := 0
	for _, a := range m.Attachments {
		b, e := base64.StdEncoding.Strict().DecodeString(a.Content)
		total += len(b)
		clear(b)
		if e != nil || total > 512<<10 || a.Filename == "" || !utf8.ValidString(a.Filename) || !safeSetting(a.Filename, 200) || strings.ContainsAny(a.Filename, "/\\") {
			return ErrConfig
		}
	}
	return nil
}
func ValidateManual(m ManualMessage) error { return validateManual(m) }
func (s *Resend) PrepareManual(m ManualMessage) ([]byte, error) {
	if s == nil || validateManual(m) != nil {
		return nil, ErrConfig
	}
	return json.Marshal(request{From: s.from, ReplyTo: s.reply, To: m.To, Cc: m.Cc, Bcc: m.Bcc, Subject: m.Subject, Text: m.Text, HTML: "<pre style=\"white-space:pre-wrap;font-family:inherit\">" + html.EscapeString(m.Text) + "</pre>", Attachments: m.Attachments})
}
