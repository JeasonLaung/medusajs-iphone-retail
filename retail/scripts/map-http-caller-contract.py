"""Explicit direct HTTP caller designs. No silent fallback for unmapped endpoints."""
import argparse
import hashlib
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
# path, task, identity, design invariant. Runtime placeholders normalized for coverage only.
GROUPS = [
    (['/store/carts/{}'], 'T032', 'cart-possession-or-owner', 'Private no-store cart DTO; ID alone never credential; versioned quote, no payment secrets'),
    (['/store/products', '/store/product-categories', '/store/collections', '/store/collections/{}', '/store/regions', '/store/regions/{}', '/store/locales', '/store/product-variants/{}', '/store/product-options'], 'T020', 'public-store-context', 'Explicit channel/locale/region context and published visibility; allowlisted nested DTO, complete pagination/variants/options; no source payload passthrough'),
    (['/store/search'], 'T022', 'public-store-context', 'Read-only batch POST; bounded query/facets/histogram with channel visibility and exact price adapter; no write exception inherited by other POSTs'),
    (['/store/seo', '/store/seo/pages', '/store/buy-policies', '/store/customer-service'], 'T058', 'public-store-context', 'Public setting allowlist; no credentials or private source paths; preserve SEO artifact/page overrides and current fallback behavior'),
    (['/store/customers/me'], 'T030', 'new-customer-session', 'Only current new account; historical same-email subjects grant nothing; private no-store'),
    (['/store/orders', '/store/orders/{}', '/store/orders/{}/timeline', '/store/orders/{}/devices', '/store/orders/{}/after-sales', '/store/orders/{}/product-reviews'], 'T034', 'new-customer-owner', 'Check order ownership first, then child items/device/request membership; exact totals and public timeline fields; no historical auto-claim or internal cost/provider data'),
    (['/store/buyback/prices', '/store/buyback/estimate'], 'T050', 'public-store-context', 'Reference/estimate only, not binding final quote; missing price stays NULL; partial assessment and exact decimal adapter; no private application fields'),
    (['/store/buyback/requests', '/store/buyback/requests/{}', '/store/buyback/requests/{}/photos', '/store/buyback/requests/{}/photos/{}'], 'T051', 'new-customer-owner', 'Ownership and nested photo/item membership; full state guards, submission key, private files; final offer equality; never restore old login access'),
    (['/store/notifications'], 'T057', 'new-customer-owner', 'List/read updates scoped to current new customer; no cross-user ID updates; old subject inbox not inherited'),
    (['/store/product-reviews', '/store/product-reviews/purchases'], 'T054', 'public-approved-or-new-customer-write', 'GET approved public DTO only; purchases and POST require new customer ownership; server derives author/verified flag, active unique customer/product'),
    (['/store/shipping-options', '/store/shipping-options/{}/calculate'], 'T033', 'cart-possession-or-owner', 'Own cart and current quote version; server option/address/profile eligibility; calculate read-only POST, no fulfillment side effect; private pricing'),
    (['/store/payment-providers', '/store/paypal/config'], 'T035', 'public-checkout-context', 'Enabled region/provider allowlist and browser-safe fields only; no merchant secrets; attempt creation separately binds owned cart version/amount'),
    (['/store/shipping-demo/orders/{}', '/store/shipping-demo/dhl'], 'T059', 'explicit-demo-fixture-only', 'Development fixture boundary, never anonymous arbitrary real order tracking; production behavior explicitly gated'),
    (['/api/mollie-return'], 'T036', 'cart-possession-or-owner', 'Preserve browser URL/result states; POST with Origin/CSRF and durable dedupe; trusted provider status, not query status; bounded polling and same-site redirect'),
    (['/admin/buyback', '/admin/buyback/{}', '/admin/buyback/{}/photos/{}', '/admin/buyback-prices', '/admin/buyback-catalog'], 'T052', 'staff-buyback-permission', 'Reject warehouse/supplier; read/write distinct permissions, private photos, full state/quote/device checks and operation audit'),
    (['/admin/devices', '/admin/devices/variants', '/admin/devices/photos', '/admin/devices/mobile-intake', '/admin/devices/supplier-accounts', '/admin/devices/refunds'], 'T040', 'staff-device-action-and-scope', '22 commands require per-action warehouse/source/order and every-member checks; cost redaction; supplier_assign remains explicit rejection, no silent replacement behavior'),
    (['/admin/orders/{}/after-sales', '/admin/orders/{}/cancel', '/admin/orders/{}/fulfillments/{}/tracking'], 'T038', 'staff-order-action-and-scope', 'Read scope and state/command guards; restricted warehouse/supplier cannot use generic writes; aftersales nested item membership; tracking is not shipment creation'),
    (['/admin/shipping-demo/dhl'], 'T059', 'staff-explicit-demo', 'Keep demonstration separately scoped from real fulfillment; no production carrier side effect by rendering page'),
    (['/admin/uploads'], 'T016', 'staff-public-asset-upload', 'Native upload capability not in custom routes; explicit MIME/header/size/path limits and asset class; private photos/attachments must use private handlers'),
    (['/admin/seo/submissions', '/admin/seo/{}', '/admin/seo/commands'], 'T058', 'staff-seo-permission', 'Enumerated resource/action only; secret masking/public allowlist, path/URL and external-job boundary, no public tick; ECharts gets actual job/error state'),
    (['/admin/mail', '/admin/mail/commands', '/admin/mail/messages/{}', '/admin/mail/messages/{}/attachments/{}'], 'T055', 'staff-mail-permission', 'Mailbox/message attachment membership, protected content and credentials; consent/suppression, durable send key and first attempt window; no attachment URL in logs'),
    (['/admin/warehouse-access', '/admin/warehouse-access/me'], 'T015', 'staff-scope-config-or-self', 'Me reports current scope only; config write separately authorized, empty bound list != all; no copied legacy user grants'),
    (['/admin/buy-policies', '/admin/customer-service'], 'T058', 'staff-content-config-permission', 'Explicit validated settings schema; public DTO separate; no arbitrary metadata write'),
    (['/admin/mollie'], 'T035', 'staff-payment-config-permission', 'Mask credential, empty edit retains original, environment validated; configure does not initiate payment or call provider'),
    (['/admin/webhook-notifications', '/admin/webhook-notifications/{}', '/admin/webhook-notifications/{}/test'], 'T057', 'staff-webhook-config-permission', 'Protected URL/secret/token and clear semantics; test explicit durable external operation with safe target validation; no implicit send on save'),
    (['/admin/product-reviews', '/admin/product-reviews/{}'], 'T054', 'staff-review-moderation', 'Read/moderate permissions and warehouse/supplier denial; approved public aggregates recalc without revealing private subject fields'),
]
DESIGNS = {p: (task, identity, invariant) for paths, task, identity, invariant in GROUPS for p in paths}
# Manually traced dynamic wrappers: source call line plus explicit caller destinations.
DYNAMIC = {
    ('medusa-store/apps/storefront/src/lib/data/buyback.ts', 27): [
        ('GET', '/store/buyback/requests'), ('GET', '/store/buyback/requests/{}'),
        ('POST', '/store/buyback/requests'), ('POST', '/store/buyback/requests/{}'),
        ('POST', '/store/buyback/requests/{}/photos'), ('GET', '/store/buyback/requests/{}/photos/{}')],
    ('medusa-store/apps/backend/src/admin/components/device-workbench.tsx', 308): [
        ('POST', '/admin/devices'), ('POST', '/admin/devices/supplier-accounts'),
        ('POST', '/admin/devices/refunds'), ('POST', '/admin/orders/{}/after-sales')],
    ('medusa-store/apps/backend/src/admin/routes/settings/webhook-notifications/page.tsx', 55): [
        ('POST', '/admin/webhook-notifications'), ('POST', '/admin/webhook-notifications/{}')],
}


def normalize(pattern):
    # Strip query before placeholders: placeholders can include operators/expressions.
    pattern = re.sub(r'\{[^{}]*\}', '{}', pattern)
    pattern = pattern.split('?', 1)[0]
    if pattern == '{}/store/regions':
        return '/store/regions'  # reviewed middleware backend-origin prefix
    if pattern == '/api/mollie-return{}':
        return '/api/mollie-return'  # reviewed query suffix, not a route segment
    return pattern


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    raw = (OUT / 'http-caller-manifest.json').read_bytes()
    source = json.loads(raw)
    for file, expected in source['source_sha256'].items():
        if hashlib.sha256((ROOT / file).read_bytes()).hexdigest() != expected:
            raise ValueError('HTTP caller source changed; recapture and review')
    rows = []
    used_dynamic = set()
    for index, call in enumerate(source['calls']):
        key = (call['source'], call['line'])
        if call['endpoint_pattern'] is None:
            if key not in DYNAMIC:
                raise ValueError('Unreviewed dynamic caller: ' + str(key))
            branches = DYNAMIC[key]
            used_dynamic.add(key)
        else:
            if len(call['methods']) != 1:
                raise ValueError('Unreviewed dynamic HTTP method')
            branches = [(call['methods'][0], normalize(call['endpoint_pattern']))]
        for method, old in branches:
            if old not in DESIGNS:
                raise ValueError('Missing explicit HTTP design: ' + old)
            task, identity, invariant = DESIGNS[old]
            target = '/api' + old if old.startswith(('/store/', '/admin/')) else old
            rows.append({'source_candidate_index': index, 'source': call['source'], 'line': call['line'],
                         'source_sha256': source['source_sha256'][call['source']],
                         'legacy_method': method, 'legacy_pattern_normalized': old,
                         'new_method': method, 'new_pattern': target, 'task': task, 'identity': identity,
                         'invariant': invariant, 'csrf': 'required-browser-write' if method not in {'GET', 'HEAD'} else 'read-only',
                         'remaining': 'Expand resource/action discriminators and exact request/response/errors; verify reachability, HTTP/DB auth and UI states',
                         'status': 'designed-not-implemented'})
    if used_dynamic != set(DYNAMIC):
        raise ValueError('Dynamic wrapper moved or disappeared; review decisions')
    result = {'format_version': 1, 'source_manifest_sha256': hashlib.sha256(raw.replace(b'\r\n', b'\n')).hexdigest(),
              'scope': 'Direct fetch candidate design with manually traced wrapper branches; not full native Admin/plugin/mobile HTML/anchor-download coverage or accepted endpoint implementation',
              'calls': rows}
    target = OUT / 'http-caller-contract.json'
    content = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        if target.read_text(encoding='utf-8-sig') != content:
            raise ValueError('HTTP caller design drift')
    else:
        target.write_text(content, encoding='utf-8', newline='')
    print(json.dumps({'source_candidates': len(source['calls']), 'expanded_branch_designs': len(rows),
                      'new_operation_patterns': len({(r['new_method'], r['new_pattern']) for r in rows}),
                      'remaining_unmapped_candidates': len(source['calls']) - len({r['source_candidate_index'] for r in rows})}))


if __name__ == '__main__':
    main()
