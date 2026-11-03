package maildelivery

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
)

// IMAPConfig follows iphone0902 SiteMailbox. No plaintext-login downgrade is
// supported. A configured certificate pin replaces CA/hostname trust only for
// this mailbox and still checks the certificate's validity interval.
type IMAPConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	Encryption string `json:"encryption"`
	CertSHA256 string `json:"cert_sha256"`
}

func ValidateIMAP(c IMAPConfig) error {
	if c.Host == "" || !safeSetting(c.Host, 255) || strings.ContainsAny(c.Host, " /\\\t") || c.Port < 1 || c.Port > 65535 || c.Username == "" || !safeSetting(c.Username, 255) || c.Password == "" || !safeSetting(c.Password, 4096) || c.Encryption != "tls" && c.Encryption != "starttls" {
		return ErrConfig
	}
	_, e := imapTLS(c)
	return e
}
func imapTLS(c IMAPConfig) (*tls.Config, error) {
	config := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if c.CertSHA256 == "" {
		return config, nil
	}
	expected, e := hex.DecodeString(c.CertSHA256)
	if e != nil || len(expected) != sha256.Size {
		return nil, ErrConfig
	}
	config.InsecureSkipVerify = true
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return ErrConfig
		}
		cert := state.PeerCertificates[0]
		actual := sha256.Sum256(cert.Raw)
		now := time.Now()
		if subtle.ConstantTimeCompare(actual[:], expected) != 1 || now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
			return ErrConfig
		}
		return nil
	}
	return config, nil
}

var ErrIMAP = errors.New("mailbox retrieval unavailable")

type IMAPMessage struct {
	UID        uint32
	ReceivedAt time.Time
	Raw        []byte
}
type IMAPBatch struct {
	UIDValidity    uint32
	Messages       []IMAPMessage
	ScannedThrough uint32
}

// FetchIMAP reads a bounded batch after a UID checkpoint. The caller must key
// that checkpoint by mailbox/folder/UIDVALIDITY and persist raw RFC822 messages
// before advancing it. EXAMINE and BODY.PEEK never mark mail read or delete it.
// expectedValidity=0 is for the first scan; changed epochs return their new
// identity without silently applying a checkpoint belonging to another epoch.
func FetchIMAP(ctx context.Context, c IMAPConfig, folder string, expectedValidity, afterUID uint32, limit int) (IMAPBatch, error) {
	out := IMAPBatch{Messages: []IMAPMessage{}}
	if ValidateIMAP(c) != nil || folder == "" || !safeSetting(folder, 255) || limit < 1 || limit > 100 {
		return out, ErrConfig
	}
	tlsConfig, _ := imapTLS(c)
	conn, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if e != nil {
		return out, ErrIMAP
	}
	defer conn.Close()
	deadline := time.Now().Add(20 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if c.Encryption == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if secure.HandshakeContext(ctx) != nil {
			return out, ErrIMAP
		}
		conn = secure
	}
	client, e := imapclient.New(conn)
	if e != nil {
		return out, ErrIMAP
	}
	defer client.Terminate()
	client.Timeout = time.Until(deadline)
	if c.Encryption == "starttls" {
		if client.StartTLS(tlsConfig) != nil {
			return out, ErrIMAP
		}
	}
	if client.Login(c.Username, c.Password) != nil {
		return out, ErrIMAP
	}
	selected, e := client.Select(folder, true)
	if e != nil {
		return out, ErrIMAP
	}
	out.UIDValidity = selected.UidValidity
	if selected.UidValidity == 0 {
		return out, ErrIMAP
	}
	if expectedValidity != 0 && expectedValidity != out.UIDValidity {
		return out, nil
	}
	if afterUID == ^uint32(0) || selected.UidNext > 0 && afterUID >= selected.UidNext-1 {
		return out, nil
	}
	// Search only UIDs in a bounded numeric window. A huge or adversarial mailbox
	// cannot force an unbounded UID SEARCH response or a full mailbox download.
	upper := uint64(afterUID) + uint64(limit)
	if upper > uint64(^uint32(0)) {
		upper = uint64(^uint32(0))
	}
	if selected.UidNext > 0 && upper >= uint64(selected.UidNext) {
		upper = uint64(selected.UidNext - 1)
	}
	set := new(imap.SeqSet)
	set.AddRange(afterUID+1, uint32(upper))
	section := &imap.BodySectionName{Peek: true}
	messages := make(chan *imap.Message)
	done := make(chan error, 1)
	go func() {
		done <- client.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchInternalDate, section.FetchItem()}, messages)
	}()
	var readError error
	seen := map[uint32]bool{}
	total := 0
	for message := range messages {
		if readError != nil {
			continue
		}
		reader := message.GetBody(section)
		if message.Uid <= afterUID || uint64(message.Uid) > upper || seen[message.Uid] || reader == nil || len(out.Messages) >= limit {
			readError = ErrIMAP
			continue
		}
		seen[message.Uid] = true
		raw, e := io.ReadAll(io.LimitReader(reader, 26<<20+1))
		total += len(raw)
		if e != nil || len(raw) > 26<<20 || total > 32<<20 {
			clear(raw)
			readError = ErrIMAP
			continue
		}
		out.Messages = append(out.Messages, IMAPMessage{UID: message.Uid, ReceivedAt: message.InternalDate, Raw: raw})
	}
	if e = <-done; e != nil || readError != nil {
		for _, message := range out.Messages {
			clear(message.Raw)
		}
		out.Messages = nil
		return out, ErrIMAP
	}
	out.ScannedThrough = uint32(upper)
	return out, nil
}
