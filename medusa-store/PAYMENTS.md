# PayPal and Mollie setup

Installed backend packages:

- `@easypayment/medusa-payment-paypal@1.3.2`
- `@variablevic/mollie-payments-medusa@0.0.13`

Medusa remains on `2.21.2`. PayPal database migrations have been applied.
These are backend integrations. The storefront still needs the PayPal UI and
Mollie redirect/return flow before customers can complete these payment methods.

## PayPal

1. Set `PAYPAL_ENCRYPTION_KEY` to a stable random secret in the backend environment
   and set `PAYPAL_ENCRYPTION_STRICT=true` before connecting credentials.
2. Set `MEDUSA_BACKEND_URL` and `STOREFRONT_URL` to your deployment URLs. Payment
   notifications require a publicly reachable backend, including sandbox tests.
3. Open Medusa Admin > Settings > PayPal, explicitly choose Sandbox for testing,
   and connect your account or enter credentials manually. The plugin defaults
   to Live; installing it does not connect a merchant account.
4. Enable `pp_paypal_paypal` (and optionally `pp_paypal_card_paypal_card`) in
   Settings > Regions once the storefront integration is ready.

## Mollie

Set these variables in the backend environment, then restart the backend:

```dotenv
MOLLIE_API_KEY=<your Mollie test_ key>
MOLLIE_REDIRECT_URL=<implemented storefront payment return URL>
MEDUSA_BACKEND_URL=<public backend URL>
```

Mollie is registered only when `MOLLIE_API_KEY` is set. The redirect URL and
backend URL are also required; do not set a placeholder key on the running
server. `MEDUSA_BACKEND_URL` maps to the plugin's `medusaUrl` option.

Enable the desired methods in both Mollie and Medusa Admin > Settings > Regions.
Provider IDs include `pp_mollie-hosted-checkout_mollie`, `pp_mollie-ideal_mollie`,
`pp_mollie-bancontact_mollie`, and `pp_mollie-card_mollie`.

The Mollie package declares peers pinned to Medusa 2.5.1 and MikroORM 6.4.3.
Installation preserves this project's existing versions. All seven Mollie
provider classes load and initialize on this project, but real API operations,
payment callbacks, capture, and refunds require end-to-end sandbox testing.

## Sources

- https://medusajs.com/integrations/@easypaymentmedusa-payment-paypal
- https://medusajs.com/integrations/@variablevicmollie-payments-medusa
