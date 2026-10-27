"""Join every inventoried custom route method to reviewed migration designs."""
import argparse
import hashlib
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
BASE = 'medusa-store/apps/backend/'
# These methods have no direct-fetch/capability design. They are not unused just
# because a caller scanner cannot see native Admin or SDK internals.
EXTRA = {
    ('GET', '/admin/custom'): (
        '/api/admin/custom', 'T014', 'staff-session',
        'Source only sends HTTP 200, no business query or mutation. Preserve a bounded compatibility probe under staff authorization; not a health check or completed business feature.'),
    ('GET', '/store/custom'): (
        '/api/store/custom', 'T014', 'public-store-context',
        'Source only sends HTTP 200, no business query or mutation. Preserve bounded compatibility behavior; do not infer catalog, account or checkout implementation from it.'),
    ('GET', '/admin/inventory-items'): (
        '/api/admin/inventory-items', 'T024', 'staff-inventory-read',
        'Preserve native inventory filters/fields/pagination plus +/-reserved_quantity and +/-stocked_quantity sort. Null aggregate displays and sorts as zero, ID breaks ties. New SQL exact aggregate order before paging; count and detail use same authorized scope and filters. Generic endpoint denied to warehouse/supplier roles; product inventory nesting has separate scoped DTO.'),
    ('GET', '/admin/orders'): (
        '/api/admin/orders', 'T038', 'staff-order-read-and-scope',
        'Preserve native order filters/count/page with warehouse order-read permission and current fulfillment-location membership. Empty location scope means none, null all. Crop nested fulfillments/items/quantities/inventory and sensitive fields, not only order IDs; no hidden global totals. Supplier generic order endpoint remains denied; supplier device command bindings do not grant this API.'),
    ('GET', '/admin/orders/{}'): (
        '/api/admin/orders/{id}', 'T038', 'staff-order-read-and-scope',
        'Check order access before DTO projection. Preserve warehouse fulfillment/item/quantity and field allowlist restrictions; supplier generic order endpoint remains denied, even with a device order binding. ID knowledge and matching email confer no access.'),
    ('GET', '/admin/stock-locations'): (
        '/api/admin/stock-locations', 'T025', 'staff-location-read-and-scope',
        'AND current location scope with validated native filters before pagination/count. Null means all, [] none; no sentinel ID in target storage. Supplier generic access remains denied; fields cannot traverse unrelated private data.'),
    ('GET', '/admin/stock-locations/{}'): (
        '/api/admin/stock-locations/{id}', 'T025', 'staff-location-read-and-scope',
        'Require current location scope and location-read permission before fetching nested DTO. Preserve denial for out-of-scope location; list permission cannot grant arbitrary detail access.'),
    ('POST', '/admin/orders/{}/fulfillments/{}/cancel'): (
        '/api/admin/orders/{id}/fulfillments/{fulfillment_id}/cancel', 'T044', 'staff-fulfillment-cancel-and-scope',
        'Source dispatches cancel_fulfillment with native_cancel including order_id, no_notification and actor. Require nested fulfillment/order membership, device guard/state, every affected assignment and durable idempotency; atomic quantity/device cancellation and audited notification intent. Generic warehouse/supplier write remains denied; warehouse device commands are a separate authorization path.'),
    ('POST', '/admin/orders/{}/fulfillments/{}/shipments'): (
        '/api/admin/orders/{id}/fulfillments/{fulfillment_id}/shipments', 'T044', 'staff-shipment-create-and-scope',
        'Source selects device ship if parcel contains managed variants OR has assignments; otherwise native ordinary shipment. Preserve both paths, all labels, quantities and notification intent, not only first tracking number. Enforce nested membership, every device/assignment and stock/payment guards, current actor, durable idempotency/outbox. Generic warehouse/supplier write remains denied; tracking edits are separate.'),
    ('POST', '/store/payment-collections/{}/payment-sessions'): (
        '/api/store/carts/{cart_id}/payment-attempts', 'T035', 'cart-possession-or-owner',
        'Replace arbitrary collection addressing with owned cart. Source resumes Mollie hosted session before creating one; preserve safe resume while binding attempt to current cart version, exact amount/currency and allowed region/provider. Reject production manual provider, whitelist provider data and browser DTO; no merchant secrets. Durable operation key, Origin/CSRF, provider reconciliation and unique active-attempt policy; old collection ID is not a bearer capability.'),
}
SUPPORT = [
    'src/api/middlewares.ts', 'src/api/warehouse-guards.ts', 'src/api/device-access.ts',
    'src/api/device-guards.ts', 'src/api/payment-guards.ts', 'src/lib/warehouse-access.ts',
    'src/workflows/device-operations.ts', 'src/workflows/resume-mollie-session.ts',
]


def normalize(path):
    return re.sub(r'\[[^/\]]+\]|\{[^{}]*\}', '{}', path)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def read_contract(name):
    raw = (OUT / name).read_bytes()
    return json.loads(raw), digest(raw.replace(b'\r\n', b'\n'))


def build():
    source, source_hash = read_contract('source-manifest.json')
    http, http_hash = read_contract('http-caller-contract.json')
    capability, capability_hash = read_contract('capability-entrypoint-contract.json')
    indexed = {}
    for file, collection, old_path in [
        ('http-caller-contract.json', http['calls'], 'legacy_pattern_normalized'),
        ('capability-entrypoint-contract.json', capability['entries'], 'legacy_path'),
    ]:
        for index, entry in enumerate(collection):
            key = (entry['legacy_method'], normalize(entry[old_path]))
            # Multiple actual callers are retained as provenance, not counted as
            # multiple server methods or collapsed into a claim of equivalence.
            indexed.setdefault(key, []).append((file, index, entry))
    hashes = dict(capability['source_sha256'])
    for entry in http['calls']:
        previous = hashes.setdefault(entry['source'], entry['source_sha256'])
        if previous != entry['source_sha256']:
            raise ValueError('Conflicting evidence source hash')
    for relative in SUPPORT:
        path = BASE + relative
        hashes[path] = digest((ROOT / path).read_bytes())
    rows = []
    seen = set()
    used_extra = set()
    for route in source['routes']:
        file = route['source']
        hashes[file] = source['source_sha256'][file]
        for export in route['methods']:
            key = (export['method'], normalize(route['path']))
            if key in seen:
                raise ValueError('Duplicate custom route method: ' + str(key))
            seen.add(key)
            refs = indexed.get(key, [])
            row = {
                'legacy_method': key[0], 'legacy_path': route['path'],
                'source': file, 'line': export['line'], 'source_sha256': hashes[file],
                'design_references': [{'file': f, 'collection': 'calls' if f.startswith('http-') else 'entries',
                                       'index': i} for f, i, _ in refs],
                'status': 'designed-not-implemented',
                'remaining': 'Exact new schemas/errors, action/resource discriminators, native handler internals, isolated authorization/transaction/UI tests; no runtime acceptance',
            }
            if key in EXTRA:
                used_extra.add(key)
                target, task, identity, invariant = EXTRA[key]
                row.update(new_method=key[0], new_path=target, task=task,
                           identity=identity, invariant=invariant, design_kind='explicit-supplement')
            elif refs:
                # Capability designs contain the closer server-boundary review.
                preferred = [r for r in refs if r[0].startswith('capability-')] or refs
                operations = {(r['new_method'], r.get('new_path', r.get('new_pattern')),
                               r['task'], r['identity'], r['invariant']) for _, _, r in preferred}
                if len(operations) != 1:
                    raise ValueError('Conflicting preferred designs: ' + str(key))
                method, target, task, identity, invariant = operations.pop()
                row.update(new_method=method, new_path=target, task=task,
                           identity=identity, invariant=invariant, design_kind='linked-reviewed-design')
            else:
                raise ValueError('Custom method has no reviewed migration design: ' + str(key))
            rows.append(row)
    if used_extra != set(EXTRA):
        raise ValueError('Supplemented method disappeared; review before changing coverage')
    for path, expected in hashes.items():
        if digest((ROOT / path).read_bytes()) != expected:
            raise ValueError('Source evidence drift: ' + path)
    return {
        'format_version': 1,
        'scope': 'All inventoried custom route methods, including native route wrappers; excludes unwrapped native/plugin APIs and is not schema or runtime acceptance',
        'input_sha256': {'source-manifest.json': source_hash, 'http-caller-contract.json': http_hash,
                         'capability-entrypoint-contract.json': capability_hash},
        'source_sha256': dict(sorted(hashes.items())), 'methods': rows,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    result = build()
    content = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    target = OUT / 'custom-route-contract.json'
    if args.check:
        if target.read_text(encoding='utf-8-sig') != content:
            raise ValueError('Custom route contract drift; review evidence and regenerate')
    else:
        target.write_text(content, encoding='utf-8', newline='')
    rows = result['methods']
    print(json.dumps({'route_files': len({r['source'] for r in rows}), 'method_designs': len(rows),
                      'supplemented_methods': sum(r['design_kind'] == 'explicit-supplement' for r in rows),
                      'source_files': len(result['source_sha256']), 'status': 'design-only'}))


if __name__ == '__main__':
    main()
