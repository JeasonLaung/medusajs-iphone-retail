package rowimport

import (
	"bytes"
	"encoding/json"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
)

// ReviewPayPalConfigJSON supports the exact merchant settings sections observed
// in the pinned snapshot. Nonempty per-environment credentials require a
// separate nested decryption program; do not blindly wrap old ciphertext or
// permit unreviewed extensions. It never grants customer/staff authentication.
func ReviewPayPalConfigJSON(table, column string, raw json.RawMessage) (json.RawMessage, error) {
	if (table != "paypal_connection" || column != "metadata") && (table != "paypal_settings" || column != "data") {
		return nil, ErrProtection
	}
	if len(raw) > protected.MaxPlaintext {
		return nil, ErrProtection
	}
	if _, err := importvalue.JSONText(raw); err != nil {
		return nil, ErrProtection
	}
	if jsonNull(raw) {
		return bytes.Clone(raw), nil
	}
	values, err := object(raw)
	if err != nil {
		return nil, ErrProtection
	}
	for name, value := range values {
		if table == "paypal_connection" {
			switch name {
			case "credentials":
				credentials, err := object(value)
				if err != nil || len(credentials) != 0 {
					return nil, ErrProtection
				}
			case "active_environment":
				if !paymentChoice(value, "live", "sandbox") {
					return nil, ErrProtection
				}
			default:
				return nil, ErrProtection
			}
			continue
		}
		section, err := object(value)
		if err != nil {
			return nil, ErrProtection
		}
		switch name {
		case "api_details":
			for key, field := range section {
				switch key {
				case "currency_code":
					currency, ok := paymentString(field)
					if !ok || !paymentCurrency.MatchString(currency) {
						return nil, ErrProtection
					}
				case "storefront_url":
					if !paymentURL(field) {
						return nil, ErrProtection
					}
				default:
					return nil, ErrProtection
				}
			}
		case "onboarding_config":
			for key, field := range section {
				switch key {
				case "bn_code", "backend_url", "seller_nonce", "partner_js_url", "partner_service_url", "partner_merchant_id_live", "partner_merchant_id_sandbox":
					// These are private merchant setup facts, not URLs to fetch or
					// permission to exchange a nonce. Preserve exact source text.
					if _, err := importvalue.Text(field, 0); err != nil {
						return nil, ErrProtection
					}
				default:
					return nil, ErrProtection
				}
			}
		default:
			return nil, ErrProtection
		}
	}
	return bytes.Clone(raw), nil
}
