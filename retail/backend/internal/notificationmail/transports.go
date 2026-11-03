package notificationmail

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/mail"

	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"
	"gorm.io/gorm"
)

type TransportConfig struct {
	Provider string                      `json:"provider"`
	SMTP     *maildelivery.SMTPConfig    `json:"smtp,omitempty"`
	Relayme  *maildelivery.RelaymeConfig `json:"relayme,omitempty"`
	IMAP     *maildelivery.IMAPConfig    `json:"imap,omitempty"`
}
type PublicTransport struct {
	TransportConfig
	HasSMTPPassword  bool `json:"has_smtp_password"`
	HasRelaymeSecret bool `json:"has_relayme_secret"`
	HasIMAPPassword  bool `json:"has_imap_password"`
}

func publicTransport(c TransportConfig) *PublicTransport {
	out := &PublicTransport{TransportConfig: c}
	if c.SMTP != nil {
		v := *c.SMTP
		out.HasSMTPPassword = v.Password != ""
		v.Password = ""
		out.SMTP = &v
	}
	if c.Relayme != nil {
		v := *c.Relayme
		out.HasRelaymeSecret = v.Secret != ""
		v.Secret = ""
		out.Relayme = &v
	}
	if c.IMAP != nil {
		v := *c.IMAP
		out.HasIMAPPassword = v.Password != ""
		v.Password = ""
		out.IMAP = &v
	}
	return out
}
func transportContext(id string) protected.Context {
	return protected.Context{Purpose: "notification-mail-transport", Table: "notification_mail_transports", Column: "config_envelope", Row: id}
}
func (s Store) transport(tx *gorm.DB, id string) (TransportConfig, error) {
	var row struct{ ConfigEnvelope []byte }
	e := tx.Table("notification_mail_transports").Where("account_id=?", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return TransportConfig{Provider: "resend"}, nil
	}
	if e != nil {
		return TransportConfig{}, ErrUnavailable
	}
	raw, e := s.Ring.Open(transportContext(id), row.ConfigEnvelope)
	if e != nil {
		return TransportConfig{}, ErrUnavailable
	}
	defer clear(raw)
	var config TransportConfig
	if decodeTransport(raw, &config) != nil {
		return TransportConfig{}, ErrUnavailable
	}
	return config, nil
}
func decodeTransport(raw []byte, out *TransportConfig) error {
	if !uniqueTransportFields(raw) {
		return ErrInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF || out.Provider != "resend" && out.Provider != "smtp" && out.Provider != "quguo" {
		return ErrInput
	}
	if out.Provider == "smtp" && (out.SMTP == nil || out.Relayme != nil) || out.Provider == "quguo" && (out.Relayme == nil || out.SMTP != nil) || out.Provider == "resend" && (out.SMTP != nil || out.Relayme != nil) {
		return ErrInput
	}
	return nil
}

// Match the parent command's duplicate-key rule throughout nested credentials.
func uniqueTransportFields(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var read func() bool
	read = func() bool {
		token, e := decoder.Token()
		if e != nil {
			return false
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter != '{' {
				return false
			}
			seen := map[string]bool{}
			for decoder.More() {
				token, e = decoder.Token()
				key, ok := token.(string)
				if e != nil || !ok || seen[key] {
					return false
				}
				seen[key] = true
				if !read() {
					return false
				}
			}
			token, e = decoder.Token()
			return e == nil && token == json.Delim('}')
		}
		return token != nil
	}
	if !read() {
		return false
	}
	_, e := decoder.Token()
	return e == io.EOF
}
func mergeTransport(next, previous TransportConfig) TransportConfig {
	if next.SMTP != nil && previous.SMTP != nil && next.SMTP.Password == "" {
		copy := *next.SMTP
		copy.Password = previous.SMTP.Password
		next.SMTP = &copy
	}
	if next.Relayme != nil && previous.Relayme != nil && next.Relayme.Secret == "" {
		copy := *next.Relayme
		copy.Secret = previous.Relayme.Secret
		next.Relayme = &copy
	}
	if next.IMAP != nil && previous.IMAP != nil && next.IMAP.Password == "" {
		copy := *next.IMAP
		copy.Password = previous.IMAP.Password
		next.IMAP = &copy
	}
	return next
}
func makeSender(c TransportConfig, key, name, email, reply string) (*maildelivery.Resend, error) {
	from := (&mail.Address{Name: name, Address: email}).String()
	if c.IMAP != nil && maildelivery.ValidateIMAP(*c.IMAP) != nil {
		return nil, ErrInput
	}
	switch c.Provider {
	case "resend":
		return maildelivery.NewResend(key, from, reply)
	case "smtp":
		if c.SMTP != nil {
			return maildelivery.NewSMTP(*c.SMTP, from, reply)
		}
	case "quguo":
		if c.Relayme != nil {
			return maildelivery.NewRelayme(*c.Relayme, from, reply)
		}
	}
	return nil, ErrInput
}
func (s Store) saveTransport(tx *gorm.DB, id string, c TransportConfig) error {
	raw, e := json.Marshal(c)
	if e != nil {
		return ErrUnavailable
	}
	defer clear(raw)
	sealed, e := s.Ring.Seal(transportContext(id), raw)
	if e != nil {
		return ErrUnavailable
	}
	defer clear(sealed)
	if tx.Exec("INSERT INTO notification_mail_transports(account_id,config_envelope) VALUES(?,?) ON DUPLICATE KEY UPDATE config_envelope=VALUES(config_envelope)", id, sealed).Error != nil {
		return ErrUnavailable
	}
	return nil
}
