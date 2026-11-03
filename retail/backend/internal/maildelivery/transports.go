package maildelivery

// Sending protocols adapted from iphone0902/internal/priceupdates/mailer.go.
// Business payload preparation stays shared with the existing Resend sender.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Existing fake senders and Resend retain canonical UUID receipts. The inherited
// gateway returns a distinct opaque submission ID, stored without truncation.
func ValidSubmissionID(sender any, id string) bool {
	if concrete, ok := sender.(*Resend); ok && concrete.dispatch != nil {
		return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,190}$`).MatchString(id)
	}
	return regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id)
}
func CanRepeat(sender any) bool {
	if capability, ok := sender.(interface{ SupportsIdempotency() bool }); ok {
		return capability.SupportsIdempotency()
	}
	return true
}

type SMTPConfig struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password,omitempty"`
	Encryption    string `json:"encryption"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
}
type RelaymeConfig struct {
	APIURL string `json:"api_url"`
	KeyID  string `json:"key_id"`
	Secret string `json:"secret,omitempty"`
}

func safeSetting(s string, max int) bool { return len(s) <= max && !strings.ContainsAny(s, "\r\n\x00") }
func validSender(from, reply string) bool {
	for _, v := range []string{from, reply} {
		if v == "" && v == reply {
			continue
		}
		if _, e := mail.ParseAddress(v); e != nil || !safeSetting(v, 320) {
			return false
		}
	}
	return from != ""
}
func transportSender(kind, from, reply string, config any) *Resend {
	raw, _ := json.Marshal([]any{kind, from, reply, config})
	digest := sha256.Sum256(raw)
	clear(raw)
	return &Resend{from: from, reply: reply, fingerprint: hex.EncodeToString(digest[:]), client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func NewSMTP(c SMTPConfig, from, reply string) (*Resend, error) {
	if !validSender(from, reply) || c.Host == "" || strings.ContainsAny(c.Host, " /\\\t\r\n\x00") || !safeSetting(c.Host, 255) || c.Port < 1 || c.Port > 65535 || !safeSetting(c.Username, 255) || !safeSetting(c.Password, 4096) || c.Encryption != "tls" && c.Encryption != "starttls" && c.Encryption != "none" || c.Username != "" && c.Password == "" {
		return nil, ErrConfig
	}
	sender := transportSender("smtp-v1", from, reply, c)
	sender.dispatch = func(ctx context.Context, key string, message request) Result { return sendSMTP(ctx, c, key, message) }
	return sender, nil
}
func NewRelayme(c RelaymeConfig, from, reply string) (*Resend, error) {
	u, e := url.Parse(c.APIURL)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" && u.Scheme != "http" || !safeSetting(c.APIURL, 500) || !validSender(from, reply) || c.KeyID == "" || !safeSetting(c.KeyID, 128) || strings.ContainsAny(c.KeyID, " .\t") || c.Secret == "" || !safeSetting(c.Secret, 4096) || strings.ContainsAny(c.Secret, " \t") {
		return nil, ErrConfig
	}
	c.APIURL = strings.TrimRight(c.APIURL, "/")
	sender := transportSender("relayme-v1", from, reply, c)
	sender.dispatch = func(ctx context.Context, key string, message request) Result {
		// The original gateway accepts HTML and returns its own submission ID.
		// Do not send Resend-only reply_to/text fields to that API.
		body, _ := json.Marshal(struct {
			From    string            `json:"from"`
			To      []string          `json:"to"`
			Subject string            `json:"subject"`
			HTML    string            `json:"html"`
			Headers map[string]string `json:"headers,omitempty"`
		}{message.From, message.To, message.Subject, message.HTML, map[string]string{"Message-ID": stableMessageID(key), "Reply-To": message.ReplyTo}})
		defer clear(body)
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL+"/api/external/v1/emails", bytes.NewReader(body))
		if e != nil {
			return Result{Code: "invalid_request"}
		}
		req.Header.Set("Authorization", "Bearer "+c.KeyID+"."+c.Secret)
		req.Header.Set("Content-Type", "application/json")
		response, e := sender.client.Do(req)
		if e != nil {
			return Result{Code: "network_error"}
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			return Result{Code: "provider_rejected"}
		}
		raw, e := io.ReadAll(io.LimitReader(response.Body, 65537))
		defer clear(raw)
		if e != nil || len(raw) > 65536 {
			return Result{Code: "provider_response"}
		}
		var accepted struct {
			ID     string `json:"id"`
			Object string `json:"object"`
			Status string `json:"status"`
		}
		if json.Unmarshal(raw, &accepted) != nil || accepted.ID == "" || !safeSetting(accepted.ID, 191) || accepted.Object != "email" || accepted.Status != "accepted" {
			return Result{Code: "provider_response"}
		}
		return Result{ID: accepted.ID}
	}
	return sender, nil
}
func stableMessageID(key string) string {
	digest := sha256.Sum256([]byte(key))
	return "<" + hex.EncodeToString(digest[:]) + "@notifications.eurorefurb.local>"
}
func smtpMessage(key string, message request) ([]byte, error) {
	var body bytes.Buffer
	boundaryDigest := sha256.Sum256([]byte(key))
	mixed := multipart.NewWriter(&body)
	if e := mixed.SetBoundary("retail-" + hex.EncodeToString(boundaryDigest[:24])); e != nil {
		return nil, e
	}
	headers := []string{"From: " + message.From, "To: " + strings.Join(message.To, ", "), "Subject: " + mime.QEncoding.Encode("UTF-8", message.Subject), "Message-ID: " + stableMessageID(key), "MIME-Version: 1.0", "Content-Type: multipart/alternative; boundary=" + mixed.Boundary()}
	if message.ReplyTo != "" {
		headers = append(headers, "Reply-To: "+message.ReplyTo)
	}
	for _, part := range []struct{ kind, value string }{{"text/plain", message.Text}, {"text/html", message.HTML}} {
		writer, e := mixed.CreatePart(map[string][]string{"Content-Type": {part.kind + "; charset=UTF-8"}, "Content-Transfer-Encoding": {"base64"}})
		if e != nil {
			return nil, e
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(part.value))
		for len(encoded) > 76 {
			_, _ = io.WriteString(writer, encoded[:76]+"\r\n")
			encoded = encoded[76:]
		}
		_, _ = io.WriteString(writer, encoded+"\r\n")
	}
	if e := mixed.Close(); e != nil {
		return nil, e
	}
	return append([]byte(strings.Join(headers, "\r\n")+"\r\n\r\n"), body.Bytes()...), nil
}
func sendSMTP(ctx context.Context, c SMTPConfig, key string, message request) Result {
	fail := func() Result { return Result{Code: "network_error"} } // no channel idempotency
	address := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	conn, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", address)
	if e != nil {
		return fail()
	}
	defer conn.Close()
	deadline := time.Now().Add(8 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.SkipTLSVerify}
	if c.Encryption == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if secure.HandshakeContext(ctx) != nil {
			return fail()
		}
		conn = secure
	}
	client, e := smtp.NewClient(conn, c.Host)
	if e != nil {
		return fail()
	}
	defer client.Close()
	if c.Encryption == "starttls" {
		if e = client.StartTLS(tlsConfig); e != nil {
			return fail()
		}
	}
	if c.Username != "" {
		if e = client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); e != nil {
			return Result{Code: "provider_rejected"}
		}
	}
	from, _ := mail.ParseAddress(message.From)
	if e = client.Mail(from.Address); e != nil {
		return Result{Code: "provider_rejected"}
	}
	for _, to := range message.To {
		if e = client.Rcpt(to); e != nil {
			return Result{Code: "provider_rejected"}
		}
	}
	body, e := smtpMessage(key, message)
	if e != nil {
		return Result{Code: "invalid_request"}
	}
	defer clear(body)
	writer, e := client.Data()
	if e != nil {
		return fail()
	}
	if _, e = writer.Write(body); e != nil {
		_ = writer.Close()
		return fail()
	}
	if e = writer.Close(); e != nil {
		return fail()
	}
	// The 250 response to DATA is definitive, even when QUIT disconnects.
	_ = client.Quit()
	// Current dispatchers store UUIDs. This stable receipt is local acceptance
	// identity; it never claims to be a remote SMTP tracking identifier.
	return Result{ID: strings.SplitN(key[strings.LastIndex(key, "/")+1:], "_", 2)[1]}
}
