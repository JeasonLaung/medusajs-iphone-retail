"""Map every recorded storefront SDK candidate to an explicit independent Gin operation."""
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
# New operation designs, not claims about Medusa's underlying HTTP paths.
# method, path, identity, response, task, migration behavior
OPERATIONS = {
    'sdk.store.customer.retrieve': ('GET', '/api/store/customers/me', 'customer-session', 'customer', 'T030', 'Only new registered customer; no historical subject/email matching; no-store; registration no longer probes retrieval errors as proof customer does not exist'),
    'sdk.store.customer.update': ('PATCH', '/api/store/customers/me', 'customer-session', 'customer', 'T030', 'Allowlisted new customer profile; no metadata/role/identity grants'),
    'sdk.store.customer.create': ('POST', '/api/store/auth/register', 'anonymous-csrf', 'auth-state', 'T030', 'Fold profile creation into durable registration command; frontend removes legacy retrieve-failure/create/relogin loop; never create based on an arbitrary retrieve failure'),
    'sdk.auth.register': ('POST', '/api/store/auth/register', 'anonymous-csrf', 'auth-state', 'T030', 'New customer identity/profile only; do not link a staff identity sharing email; verification-required state preserves current UI'),
    'sdk.auth.login': ('POST', '/api/store/auth/login', 'anonymous-csrf', 'auth-state', 'T030', 'Customer-only session or restricted verification challenge; no general customer privileges before verification; no bearer token exposed to browser JS'),
    'sdk.auth.logout': ('POST', '/api/store/auth/logout', 'customer-session', 'success', 'T030', 'Invalidate current session and cookie; Astro removes private cached data/cart ownership assumptions'),
    'sdk.auth.verification.request': ('POST', '/api/store/auth/verification/request', 'verification-challenge', 'delivery-state', 'T030', 'Bound new identity email; 60s resend limit; protected outbox; no arbitrary recipient/entity_type; old verification tokens rejected'),
    'sdk.auth.verification.confirm': ('POST', '/api/store/auth/verification/confirm', 'new-verification-capability-csrf', 'success', 'T030', 'Consume valid new token atomically on explicit POST; page GET only renders confirmation state; expired/replaced/used tokens fail; no old token restoration'),
    'sdk.store.customer.createAddress': ('POST', '/api/store/customers/me/addresses', 'customer-session', 'customer', 'T030', 'New customer address book only; server assigns owner; country/address validation; idempotent create'),
    'sdk.store.customer.updateAddress': ('PATCH', '/api/store/customers/me/addresses/{address_id}', 'customer-session', 'customer', 'T030', 'Check current ownership and version; no old address-book restore'),
    'sdk.store.customer.deleteAddress': ('DELETE', '/api/store/customers/me/addresses/{address_id}', 'customer-session', 'customer', 'T030', 'Check current ownership; existing order address snapshots unaffected'),
    'sdk.store.region.list': ('GET', '/api/store/regions', 'public-context', 'regions-page', 'T020', 'Country/currency/locale/channel context explicit; paginated sitemap traversal, no assumption one page includes all'),
    'sdk.store.product.list': ('GET', '/api/store/products', 'public-context', 'products-page', 'T020', 'Published and channel-visible; bounded include; exact price string adapter; complete variants/images/options required by selected page DTO'),
    'sdk.store.collection.list': ('GET', '/api/store/collections', 'public-context', 'collections-page', 'T020', 'Public DTO only; stable pagination; preserve collection URLs/sitemap'),
    'sdk.store.category.list': ('GET', '/api/store/product-categories', 'public-context', 'categories-page', 'T020', 'Active noninternal tree with explicit parent/descendant query; no unrestricted fields syntax'),
    'sdk.store.cart.create': ('POST', '/api/store/carts', 'anonymous-csrf', 'cart', 'T032', 'Issue scoped anonymous cart possession cookie; new region/locale/channel version; cart ID alone is insufficient credential'),
    'sdk.store.cart.retrieve': ('GET', '/api/store/carts/{cart_id}', 'cart-possession-or-owner', 'cart', 'T032', 'Private no-store; owned customer or valid scoped possession; completed state checked before reuse'),
    'sdk.store.cart.update': ('PATCH', '/api/store/carts/{cart_id}', 'cart-possession-or-owner', 'cart', 'T032', 'Explicit addresses/contact/locale/region/promotion fields; version checked; changes invalidate incompatible shipping/payment quotes; no client-provided totals'),
    'sdk.store.cart.createLineItem': ('POST', '/api/store/carts/{cart_id}/line-items', 'cart-possession-or-owner', 'cart', 'T032', 'Variant and quantity only; price/stock validated on server; add does not reserve stock; inventory error code replaces English string matching'),
    'sdk.store.cart.updateLineItem': ('PATCH', '/api/store/carts/{cart_id}/line-items/{item_id}', 'cart-possession-or-owner', 'cart', 'T032', 'Item must belong to cart; exact quantity and version; no zero-default coercion'),
    'sdk.store.cart.deleteLineItem': ('DELETE', '/api/store/carts/{cart_id}/line-items/{item_id}', 'cart-possession-or-owner', 'cart', 'T032', 'Item ownership and cart version; recompute quote and invalidate payment binding'),
    'sdk.store.cart.addShippingMethod': ('POST', '/api/store/carts/{cart_id}/shipping-methods', 'cart-possession-or-owner', 'cart', 'T033', 'Validate option/profile/address/data and recalculate server amount; no client shipping price'),
    'sdk.store.cart.transferCart': ('POST', '/api/store/carts/{cart_id}/claim', 'customer-session-and-cart-possession', 'cart', 'T032', 'Explicitly transfer current cart ownership; this is not merging two carts; previous owner mismatch fails; new session cannot claim historical customer carts by email'),
    'sdk.store.payment.initiatePaymentSession': ('POST', '/api/store/carts/{cart_id}/payment-attempts', 'cart-possession-or-owner', 'payment-attempt', 'T035', 'Derive collection from cart rather than trust arbitrary collection ID; bind cart version/amount/currency/provider; stable durable idempotency; browser gets allowlisted checkout config only'),
    'sdk.store.cart.complete': ('POST', '/api/store/carts/{cart_id}/complete', 'cart-possession-or-owner', 'completion-state', 'T034', 'Unique cart order; durable operation/intent and inventory locks; pending unknown states remain recoverable; GET return page cannot complete transaction'),
    'sdk.store.order.requestTransfer': ('POST', '/api/store/orders/{order_id}/transfer-requests', 'customer-session', 'transfer-request-state', 'T034', 'Request does not grant ownership; durable explicit new transfer capability; no historical account email auto-claim; prevent leaking order email/existence'),
    'sdk.store.order.acceptTransfer': ('POST', '/api/store/orders/{order_id}/transfer-requests/accept', 'customer-session-and-transfer-capability', 'order', 'T034', 'Validate purpose/order/new recipient/expiry/current state and consume once atomically; old transfer capability rejected; confirmation POST only'),
    'sdk.store.order.declineTransfer': ('POST', '/api/store/orders/{order_id}/transfer-requests/decline', 'customer-session-and-transfer-capability', 'success', 'T034', 'Same capability binding; decline revokes request without changing ownership; confirmation GET cannot consume'),
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    source = json.loads((OUT / 'source-manifest.json').read_text(encoding='utf-8-sig'))
    candidates = source['storefront_sdk_call_candidates']
    used = {c['expression'] for c in candidates}
    if used != set(OPERATIONS):
        raise ValueError('SDK method set changed; review additions/removals explicitly')
    operations = []
    for expression, (method, path, identity, response, task, behavior) in OPERATIONS.items():
        sites = [dict(c) for c in candidates if c['expression'] == expression]
        for site in sites:
            digest = hashlib.sha256((ROOT / site['source']).read_bytes()).hexdigest()
            if digest != source['source_sha256'][site['source']]:
                raise ValueError('SDK caller changed after baseline; review source evidence')
            site['source_sha256'] = digest
        operations.append({'legacy_sdk_expression': expression, 'new_method': method, 'new_path': path,
                           'identity': identity, 'response_contract': response, 'task': task,
                           'csrf': method not in {'GET', 'HEAD'},
                           'idempotency': 'not-applicable-read' if method == 'GET' else 'durable-command-or-explicit-version-policy-required',
                           'behavior': behavior, 'call_sites': sites, 'status': 'designed-not-implemented',
                           'remaining': 'Request/response field schemas, error/status cases, route/caller reachability and runtime contract tests still required'})
    result = {'format_version': 1,
              'scope': 'All 36 statically recorded SDK candidates mapped; not all direct fetch, built-in Admin, plugin or custom route APIs. New paths are design choices, not claims about old HTTP paths.',
              'source_manifest_sha256': hashlib.sha256((OUT / 'source-manifest.json').read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
              'operations': operations}
    target = OUT / 'storefront-sdk-contract.json'
    content = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        if target.read_text(encoding='utf-8-sig') != content:
            raise ValueError('SDK contract changed; review before regeneration')
    else:
        target.write_text(content, encoding='utf-8', newline='')
    print(json.dumps({'sdk_candidates': len(candidates), 'method_designs': len(operations),
                      'distinct_new_operations': len({(o['new_method'], o['new_path']) for o in operations}),
                      'status': 'design only; T007 incomplete'}))


if __name__ == '__main__':
    main()
