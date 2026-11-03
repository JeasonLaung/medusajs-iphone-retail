package maildelivery

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
)

func TestIMAPReadOnlyTLSAndSTARTTLS(t *testing.T) {
	for _, encryption := range []string{"tls", "starttls"} {
		t.Run(encryption, func(t *testing.T) {
			certificateServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			certificate := certificateServer.TLS.Certificates[0]
			certificateServer.Close()
			backend := memory.New()
			user, e := backend.Login(nil, "username", "password")
			if e != nil {
				t.Fatal(e)
			}
			mailbox, e := user.GetMailbox("INBOX")
			if e != nil {
				t.Fatal(e)
			}
			memoryMailbox := mailbox.(*memory.Mailbox)
			memoryMailbox.Messages[0].Flags = nil
			server := imapserver.New(backend)
			server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			if encryption == "tls" {
				listener = tls.NewListener(listener, server.TLSConfig)
			}
			done := make(chan error, 1)
			go func() { done <- server.Serve(listener) }()
			defer func() {
				_ = server.Close()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("IMAP test listener leaked")
				}
			}()
			host, rawPort, _ := net.SplitHostPort(listener.Addr().String())
			port, _ := strconv.Atoi(rawPort)
			pin := sha256.Sum256(certificate.Certificate[0])
			config := IMAPConfig{Host: host, Port: port, Username: "username", Password: "password", Encryption: encryption, CertSHA256: hex.EncodeToString(pin[:])}
			// The original sample mailbox starts at UID 6: an empty numeric window must
			// expose a checkpoint so a subsequent scan does not get stuck at UID 0.
			batch, e := FetchIMAP(context.Background(), config, "INBOX", 0, 0, 3)
			if e != nil || batch.UIDValidity != 1 || batch.ScannedThrough != 3 || len(batch.Messages) != 0 {
				t.Fatalf("sparse initial window: %#v %v", batch, e)
			}
			batch, e = FetchIMAP(context.Background(), config, "INBOX", 1, batch.ScannedThrough, 3)
			if e != nil || batch.ScannedThrough != 6 || len(batch.Messages) != 1 || batch.Messages[0].UID != 6 || !strings.Contains(string(batch.Messages[0].Raw), "Hi there") || batch.Messages[0].ReceivedAt.IsZero() {
				t.Fatalf("RFC822 retrieval missing: %#v %v", batch, e)
			}
			if len(memoryMailbox.Messages[0].Flags) != 0 {
				t.Fatal("read-only fetch marked message seen")
			}
			batch, e = FetchIMAP(context.Background(), config, "INBOX", 2, 3, 3)
			if e != nil || batch.UIDValidity != 1 || batch.ScannedThrough != 0 || len(batch.Messages) != 0 {
				t.Fatal("old UID epoch silently reused")
			}
			batch, e = FetchIMAP(context.Background(), config, "INBOX", 1, 6, 3)
			if e != nil || len(batch.Messages) != 0 {
				t.Fatal("checkpoint re-fetched an existing message")
			}
			config.CertSHA256 = strings.Repeat("0", 64)
			if _, e = FetchIMAP(context.Background(), config, "INBOX", 0, 0, 3); e != ErrIMAP {
				t.Fatal("wrong certificate pin accepted")
			}
		})
	}
}

func TestIMAPConfigurationAndCancellation(t *testing.T) {
	config := IMAPConfig{Host: "localhost", Port: 993, Username: "fixture", Password: "secret", Encryption: "tls"}
	for _, mutate := range []func(*IMAPConfig){func(c *IMAPConfig) { c.Encryption = "none" }, func(c *IMAPConfig) { c.Password = "" }, func(c *IMAPConfig) { c.Host = "host\r\n" }, func(c *IMAPConfig) { c.CertSHA256 = "wrong" }} {
		invalid := config
		mutate(&invalid)
		if ValidateIMAP(invalid) == nil {
			t.Fatal("invalid IMAP configuration accepted")
		}
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := listener.Accept()
		if e == nil {
			defer conn.Close()
			buffer := make([]byte, 4096)
			for {
				if _, e = conn.Read(buffer); e != nil {
					return
				}
			}
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	config.Host = host
	config.Port, _ = strconv.Atoi(port)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, e = FetchIMAP(ctx, config, "INBOX", 0, 0, 1); e != ErrIMAP {
		t.Fatal("unresponsive peer did not cancel")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("IMAP connection leaked")
	}
}
