package maildelivery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSMTPAcceptanceAndPreparedBusinessPayloads(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	delivered := make(chan []byte, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 localhost ready\r\n")
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				_, _ = fmt.Fprint(conn, "250 localhost\r\n")
			case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
				_, _ = fmt.Fprint(conn, "250 ok\r\n")
			case line == "DATA\r\n":
				_, _ = fmt.Fprint(conn, "354 send data\r\n")
				var data strings.Builder
				for {
					line, e = reader.ReadString('\n')
					if e != nil {
						return
					}
					if line == ".\r\n" {
						break
					}
					data.WriteString(line)
				}
				delivered <- []byte(data.String())
				_, _ = fmt.Fprint(conn, "250 accepted\r\n")
			case line == "QUIT\r\n":
				return // accepted DATA must survive QUIT disconnect
			default:
				return
			}
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	sender, e := NewSMTP(SMTPConfig{Host: host, Port: number, Encryption: "none"}, "EuroRefurb <sender@example.invalid>", "reply@example.invalid")
	if e != nil {
		t.Fatal(e)
	}
	payload, e := sender.Prepare("buyer@example.invalid", "https://shop.example.invalid/verify?token=owned", "zh")
	if e != nil {
		t.Fatal(e)
	}
	key := "mail/cv_00000000-0000-4000-8000-000000000001"
	result := sender.Send(context.Background(), key, payload)
	if result.ID != "00000000-0000-4000-8000-000000000001" || result.Retry || sender.SupportsIdempotency() {
		t.Fatalf("unexpected acceptance %#v", result)
	}
	select {
	case raw := <-delivered:
		message, e := mail.ReadMessage(strings.NewReader(string(raw)))
		if e != nil {
			t.Fatal(e)
		}
		if message.Header.Get("Reply-To") != "reply@example.invalid" || message.Header.Get("Message-ID") != stableMessageID(key) {
			t.Fatal("missing envelope metadata")
		}
		_, params, e := mime.ParseMediaType(message.Header.Get("Content-Type"))
		if e != nil {
			t.Fatal(e)
		}
		parts := multipart.NewReader(message.Body, params["boundary"])
		count := 0
		for {
			part, e := parts.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal(e)
			}
			if part.Header.Get("Content-Transfer-Encoding") != "base64" {
				t.Fatal("non portable body encoding")
			}
			count++
		}
		if count != 2 {
			t.Fatal("expected text and HTML alternatives")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP delivery missing")
	}
	for _, prefix := range []string{"cv_", "op_", "ce_"} {
		last := prefix + "00000000-0000-4000-8000-000000000001"
		if got := strings.SplitN(last, "_", 2)[1]; got != "00000000-0000-4000-8000-000000000001" {
			t.Fatal("non registration receipt identity")
		}
	}
}

func TestRelaymeOriginalWireContract(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/external/v1/emails" || r.Header.Get("Authorization") != "Bearer fixture.secret" {
			t.Error("wrong Relayme route or credentials")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid body")
		}
		if body["reply_to"] != nil || body["text"] != nil || body["html"] == nil || body["headers"] == nil {
			t.Error("Resend fields leaked or MIME metadata lost")
		}
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"id":"gateway-submission-123","object":"email","status":"accepted","delivery_status":"pending"}`)
	}))
	defer server.Close()
	sender, e := NewRelayme(RelaymeConfig{APIURL: server.URL + "/", KeyID: "fixture", Secret: "secret"}, "sender@example.invalid", "")
	if e != nil {
		t.Fatal(e)
	}
	payload, e := sender.Prepare("buyer@example.invalid", "https://shop.example.invalid/verify", "en")
	if e != nil {
		t.Fatal(e)
	}
	result := sender.Send(context.Background(), "mail/cv_00000000-0000-4000-8000-000000000001", payload)
	if result.ID != "gateway-submission-123" || result.Retry || calls != 1 || sender.SupportsIdempotency() {
		t.Fatalf("acceptance lost %#v calls=%d", result, calls)
	}
}

func TestTransportValidationAndUncertainDelivery(t *testing.T) {
	for _, c := range []SMTPConfig{{Host: "smtp.example.invalid\r\n", Port: 465, Encryption: "tls"}, {Host: "smtp.example.invalid", Port: 0, Encryption: "tls"}, {Host: "smtp.example.invalid", Port: 465, Encryption: "invalid"}, {Host: "smtp.example.invalid", Port: 465, Encryption: "tls", Username: "sender"}} {
		if _, e := NewSMTP(c, "sender@example.invalid", ""); e == nil {
			t.Fatal("invalid SMTP configuration accepted")
		}
	}
	for _, root := range []string{"file:///mail", "https://user:password@example.invalid", "https://example.invalid?key=secret"} {
		if _, e := NewRelayme(RelaymeConfig{APIURL: root, KeyID: "fixture", Secret: "secret"}, "sender@example.invalid", ""); e == nil {
			t.Fatal("invalid gateway URL accepted")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	sender, _ := NewRelayme(RelaymeConfig{APIURL: server.URL, KeyID: "fixture", Secret: "secret"}, "sender@example.invalid", "")
	body, _ := sender.Prepare("buyer@example.invalid", "https://shop.example.invalid/verify", "en")
	result := sender.Send(context.Background(), "mail/cv_00000000-0000-4000-8000-000000000001", body)
	if result.ID != "" || result.Retry || result.Code != "provider_rejected" {
		t.Fatal("gateway uncertainty allowed blind retry")
	}
	resend, _ := NewResend("re_artificial_key", "sender@example.invalid", "")
	if !resend.SupportsIdempotency() {
		t.Fatal("existing Resend retry policy changed")
	}
}
