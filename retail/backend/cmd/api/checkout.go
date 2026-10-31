package main

import (
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/checkoutconfig"
)

var errCheckoutConfiguration = checkoutconfig.ErrInvalid

func checkoutOptions(env func(string) string) (*cart.InitialPaymentOptions, error) {
	return checkoutconfig.Load(env)
}
