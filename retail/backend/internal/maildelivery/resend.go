// Package maildelivery sends explicitly prepared mail, never queries business
// data, and never logs provider bodies, credentials, recipients or links.
package maildelivery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrConfig = errors.New("mail sender configuration invalid")

type Result struct {
	ID    string
	Code  string
	Retry bool
}
type Resend struct {
	key, from, reply, fingerprint string
	client                        *http.Client
}

func NewResend(key, from, reply string) (*Resend, error) {
	return newResend(key, from, reply, http.DefaultTransport)
}
func newResend(key, from, reply string, transport http.RoundTripper) (*Resend, error) {
	valid := func(raw string) bool {
		_, err := mail.ParseAddress(raw)
		return err == nil && !strings.ContainsAny(raw, "\r\n\x00") && len(raw) <= 320
	}
	if !strings.HasPrefix(key, "re_") || len(key) < 8 || len(key) > 512 || strings.ContainsAny(key, " \t\r\n\x00") || !valid(from) || reply != "" && !valid(reply) || transport == nil {
		return nil, ErrConfig
	}
	config, _ := json.Marshal([]string{"resend-verification-v1", key, from, reply})
	digest := sha256.Sum256(config)
	return &Resend{key: key, from: from, reply: reply, fingerprint: hex.EncodeToString(digest[:]), client: &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (s *Resend) Fingerprint() string { return s.fingerprint }

type request struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
	ReplyTo string   `json:"reply_to,omitempty"`
}

// Copy and structure inherited from lib/registration-email.ts. The caller must
// independently verify the origin, challenge ownership and current expiry.
func (s *Resend) Prepare(to, link, locale string) ([]byte, error) {
	address, err := mail.ParseAddress(to)
	parsed, urlErr := url.Parse(link)
	if err != nil || address.Name != "" || address.Address != to || strings.ContainsAny(to, "\r\n\x00") || urlErr != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || len(link) > 2048 || locale != "en" && locale != "zh" {
		return nil, ErrConfig
	}
	title := "Verify your email to join EuroRefurb"
	intro := "Verify your email using the link below, then sign in to complete your registration."
	action := "Verify email"
	expiry := "This link expires 15 minutes after your request and can only be used once. If you did not request this, you can ignore this email."
	if locale == "zh" {
		title = "验证邮箱，完成 EuroRefurb 注册"
		intro = "请点击下方链接验证邮箱，之后登录即可完成注册。"
		action = "验证邮箱"
		expiry = "链接在申请后 15 分钟内有效，且只能使用一次。如果你没有申请注册，请忽略此邮件。"
	}
	return json.Marshal(request{From: s.from, To: []string{to}, ReplyTo: s.reply, Subject: title, Text: intro + "\n\n" + link + "\n\n" + expiry, HTML: "<h1>" + title + "</h1><p>" + intro + "</p><p><a href=\"" + html.EscapeString(link) + "\">" + action + "</a></p><p>" + expiry + "</p>"})
}
func (s *Resend) Send(ctx context.Context, key string, payload []byte) Result {
	if s == nil || s.client == nil || len(payload) == 0 || len(payload) > 16384 || (!strings.HasPrefix(key, "mail/cv_") && !strings.HasPrefix(key, "mail/op_")) {
		return Result{Code: "invalid_request"}
	}
	prefix := "mail/cv_"
	if strings.HasPrefix(key, "mail/op_") {
		prefix = "mail/op_"
	}
	id, err := uuid.Parse(strings.TrimPrefix(key, prefix))
	if err != nil || id == uuid.Nil || key != prefix+id.String() {
		return Result{Code: "invalid_request"}
	}
	var message request
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&message) != nil || decoder.Decode(new(any)) != io.EOF || message.From != s.from || message.ReplyTo != s.reply || len(message.To) != 1 || message.Subject == "" || message.Text == "" || message.HTML == "" {
		return Result{Code: "invalid_request"}
	}
	if _, err := mail.ParseAddress(message.To[0]); err != nil {
		return Result{Code: "invalid_request"}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return Result{Code: "invalid_request"}
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", key)
	response, err := s.client.Do(req)
	if err != nil {
		return Result{Code: "network_error", Retry: true}
	}
	defer response.Body.Close()
	if response.StatusCode == 429 || response.StatusCode >= 500 {
		return Result{Code: "provider_temporary", Retry: true}
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return Result{Code: "provider_rejected"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(body) > 8192 {
		return Result{Code: "provider_response", Retry: true}
	}
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &result) != nil {
		return Result{Code: "provider_response", Retry: true}
	}
	parsed, err := uuid.Parse(result.ID)
	if err != nil || parsed == uuid.Nil || result.ID != parsed.String() {
		return Result{Code: "provider_response", Retry: true}
	}
	return Result{ID: result.ID}
}
