package rowimport

import (
	"encoding/json"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/legacysecret"
	"github.com/eurorefurb/retail/internal/protected"
)

// ConfigSecrets converts only the twelve explicitly reviewed merchant string
// fields and the explicitly reviewed PayPal merchant JSON baseline. Other
// non-NULL protected JSON returns ErrProtection. The caller owns independent
// target key provisioning/backup and must not log BoundRow secrets.
type ConfigSecrets struct {
	target *protected.Ring
	source *legacysecret.Keys
	paypal *legacysecret.PayPal
}

func NewConfigSecrets(target *protected.Ring, source *legacysecret.Keys) (*ConfigSecrets, error) {
	return NewConfigSecretsWithPayPal(target, source, nil)
}

// PayPal source encryption has its own explicit key and versioned format.
// A nil PayPal reader accepts legacy plaintext only, never encrypted fallback.
func NewConfigSecretsWithPayPal(target *protected.Ring, source *legacysecret.Keys, paypal *legacysecret.PayPal) (*ConfigSecrets, error) {
	if target == nil {
		return nil, ErrProtection
	}
	return &ConfigSecrets{target: target, source: source, paypal: paypal}, nil
}

func configDomain(table, column string, row SourceRow) (domain, purpose string, err error) {
	purpose = "merchant-config"
	switch table + "." + column {
	case "mail_account.api_key", "mail_account.webhook_secret":
		domain = "mail"
	case "seo_provider.api_key":
		domain = "seo"
	case "seo_setting.value":
		cell, present := row["is_secret"]
		if !present || cell.SQLNull {
			return "", "", ErrProtection
		}
		secret, e := importvalue.Boolean(cell.Raw)
		if e != nil {
			return "", "", ErrProtection
		}
		if secret {
			domain = "seo"
		}
	case "webhook_endpoint.url", "webhook_endpoint.secret", "webhook_endpoint.token":
		domain = "webhook"
	case "mollie_config.api_key":
		domain = "mollie"
	case "paypal_connection.auth_code", "paypal_connection.seller_client_secret", "paypal_connection.app_access_token":
	case "mail_contact.unsubscribe_token":
		purpose = "business-capability"
	default:
		return "", "", ErrProtection
	}
	return domain, purpose, nil
}

func (c *ConfigSecrets) Convert(table string, row SourceRow) (BoundRow, error) {
	if c == nil || c.target == nil {
		return BoundRow{}, ErrProtection
	}
	return convertRow(table, row, func(f field, raw json.RawMessage) (any, error) {
		if f.Transform != "protected_string" && f.Transform != "protected_exact_json" {
			return nil, ErrProtection
		}
		idCell, exists := row["id"]
		if !exists || idCell.SQLNull {
			return nil, ErrProtection
		}
		id, err := importvalue.Text(idCell.Raw, 191)
		if err != nil || id == "" {
			return nil, ErrProtection
		}
		plans, err := compiledPlans()
		if err != nil {
			return nil, ErrProtection
		}
		if f.Transform == "protected_exact_json" {
			var reviewed json.RawMessage
			if table == "paypal_connection" && f.Source == "metadata" {
				reviewed, err = c.convertPayPalMetadata(raw, row)
			} else {
				reviewed, err = ReviewPayPalConfigJSON(table, f.Source, raw)
			}
			if err != nil {
				return nil, ErrProtection
			}
			defer clear(reviewed)
			if table == "paypal_connection" && !jsonNull(raw) {
				values, _ := object(raw)
				if active, exists := values["active_environment"]; exists {
					environment, err := importvalue.Text(row["environment"].Raw, 0)
					selected, ok := paymentString(active)
					if row["environment"].SQLNull || err != nil || !ok || selected != environment {
						return nil, ErrProtection
					}
				}
			}
			envelope, err := c.target.Seal(protected.Context{Purpose: "merchant-config", Table: plans[table].Target, Column: f.Target, Row: id}, reviewed)
			if err != nil {
				return nil, ErrProtection
			}
			return envelope, nil
		}
		domain, purpose, err := configDomain(table, f.Source, row)
		if err != nil {
			return nil, ErrProtection
		}
		value, err := importvalue.Text(raw, 0)
		if err != nil || len(value) > 2*protected.MaxPlaintext {
			return nil, ErrProtection
		}
		var plaintext []byte
		if table == "paypal_connection" && (f.Source == "seller_client_secret" || f.Source == "app_access_token") {
			plaintext, err = c.paypal.Open(value)
			if err != nil {
				return nil, ErrProtection
			}
		} else if domain != "" {
			plaintext, err = c.source.Open(domain, value)
			if err != nil {
				return nil, ErrProtection
			}
		} else {
			plaintext = []byte(value)
		}
		defer clear(plaintext)
		envelope, err := c.target.Seal(protected.Context{Purpose: purpose, Table: plans[table].Target, Column: f.Target, Row: id}, plaintext)
		if err != nil {
			return nil, ErrProtection
		}
		return envelope, nil
	}, "")
}
