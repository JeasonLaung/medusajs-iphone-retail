# PayPal and Mollie setup

Installed backend packages:

- `@easypayment/medusa-payment-paypal@1.3.2`
- `@variablevic/mollie-payments-medusa@0.0.13`

Medusa remains on `2.21.2`. PayPal database migrations have been applied.
The storefront includes `@easypayment/medusa-paypal-ui@1.3.1`, PayPal wallet/card
checkout on the review step, and Mollie hosted checkout with a verified return
flow. Unconfigured or region-disabled PayPal and Mollie options are shown as
unavailable. Installation alone does not connect a merchant account.

## PayPal

1. Set `PAYPAL_ENCRYPTION_KEY` to a stable random secret in the backend environment
   and set `PAYPAL_ENCRYPTION_STRICT=true` before connecting credentials.
2. Set `MEDUSA_BACKEND_URL` and `STOREFRONT_URL` to your deployment URLs. Payment
   notifications require a publicly reachable backend, including sandbox tests.
3. Open Medusa Admin > Settings > PayPal, explicitly choose Sandbox for testing,
   and connect your account or enter credentials manually. The plugin defaults
   to Live; installing it does not connect a merchant account.
4. Enable `pp_paypal_paypal` (and optionally `pp_paypal_card_paypal_card`) in
   Settings > Regions after connecting your merchant account.

## Mollie

Set these variables in the backend environment, then restart the backend:

```dotenv
MOLLIE_API_KEY=<your Mollie test_ key>
STOREFRONT_URL=https://shop.example
MOLLIE_REDIRECT_URL=https://shop.example/api/mollie-return
MEDUSA_BACKEND_URL=<public backend URL>
```

Mollie is registered only when `MOLLIE_API_KEY` is set. The redirect URL and
backend URL are also required; do not set a placeholder key on the running
server. `MEDUSA_BACKEND_URL` maps to the plugin's `medusaUrl` option.
If `MOLLIE_REDIRECT_URL` is omitted, it defaults to
`STOREFRONT_URL + /api/mollie-return`. Set the storefront's
`NEXT_PUBLIC_BASE_URL` to the same origin as `STOREFRONT_URL`, and use that exact
origin in the browser (do not mix `localhost` and `127.0.0.1`). The server adds
the cart ID to the return URL. The buyer's cart cookie must match it.

Enable the desired methods in both Mollie and Medusa Admin > Settings > Regions.
Provider IDs include `pp_mollie-hosted-checkout_mollie`, `pp_mollie-ideal_mollie`,
`pp_mollie-bancontact_mollie`, and `pp_mollie-card_mollie`.

The Mollie package declares peers pinned to Medusa 2.5.1 and MikroORM 6.4.3.
Installation preserves this project's existing versions. All seven Mollie
provider classes load and initialize on this project, but real API operations,
payment callbacks, capture, and refunds require end-to-end sandbox testing.
The local provider adapter retains the upstream SDK and provider IDs, while
fixing webhook amounts, session references, payment amount changes, partial
refunds, and per-cart return URLs. Payment status is fetched from Mollie during
webhook handling and order authorization; browser return parameters are never
treated as proof of payment.

## Enable a region

Use Admin > Settings > Regions to enable only the methods supported by your
merchant account, or use this helper after merchant setup:

```powershell
cd medusa-store/apps/backend
corepack pnpm exec medusa exec ./src/scripts/enable-checkout-payments.ts reg_YOUR_REGION_ID
```

The helper adds the registered PayPal/Mollie methods while preserving existing
providers. Review the resulting methods in Admin; Mollie must also have the
chosen payment methods enabled in its dashboard.

## Verification

```powershell
cd medusa-store/apps/storefront
corepack pnpm run test:payments
corepack pnpm exec tsc --noEmit --incremental false

cd ../backend
$env:TEST_TYPE = "unit"
corepack pnpm exec jest src/modules/mollie/__tests__/service.unit.spec.ts --runInBand
corepack pnpm exec medusa exec ./src/scripts/check-checkout-payments.ts
```

The last command reads an existing open cart and checks the local rendered
checkout without completing an order. Sandbox merchant credentials are still
required to test successful payment, cancellation, webhook delivery, capture,
and refunds end to end.

## Sources

- https://medusajs.com/integrations/@easypaymentmedusa-payment-paypal
- https://medusajs.com/integrations/@variablevicmollie-payments-medusa
