package cart

import (
	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
)

func initialKnownProvider(id string) bool {
	return id == hostedMollie || id == "pp_paypal_paypal" || id == "pp_paypal_card_paypal_card"
}
func initialSelectedProvider(tx *gorm.DB, current Snapshot, id string, o InitialPaymentOptions) error {
	if !initialKnownProvider(id) {
		return ErrConfiguration
	}
	if _, err := o.Adapters.Lookup(id); err != nil {
		return ErrConfiguration
	}
	return initialProviderID(tx, current, id)
}
func initialSettlement(provider, amount, currency string) (string, error) {
	if provider == hostedMollie {
		return payments.MollieSettlementAmount(amount)
	}
	if initialKnownProvider(provider) {
		return payments.PayPalSettlementAmount(amount, currency)
	}
	return "", ErrConfiguration
}
