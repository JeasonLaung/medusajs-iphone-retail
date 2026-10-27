"""Reviewed non-SDK capability/download/callback entries, with source hashes."""
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
BASE = 'medusa-store/apps/backend/'
ENTRIES = [
    ('GET', '/static/mobile-intake/index.html', '/static/mobile-intake/index.html', 'public-static-shell', 'none-read-only', 'T048',
     ['static/mobile-intake/index.html', 'static/mobile-intake/app.js', 'static/mobile-intake/locales.js', 'static/mobile-intake/style.css', 'src/admin/lib/mobile-intake.ts'],
     'Preserve page URL, language, scan/camera/variant/expiry/receipts/delete interactions. Fragment credential never sent as query/referrer; no old token validity. New page consumes Gin API, no Medusa or Vue runtime in final React/Astro target.',
     'HTML noindex,nofollow and no-referrer; preserve static asset provenance; token not cached or logged'),
    ('POST', '/admin/devices/mobile-intake', '/api/admin/devices/mobile-intake', 'new-staff-action-and-source-scope', 'session-origin-csrf', 'T048',
     ['src/api/admin/devices/mobile-intake/route.ts', 'src/workflows/mobile-intake.ts', 'src/lib/mobile-intake-token.ts', 'src/api/mobile-intake/access.ts'],
     'Issue new purpose/actor/source/category/expiry/nonce token after current permission; self source only; two hours; not old JWT key. Idempotency key binds create action; do not issue more privileged token on retry.', 'Private no-store; exact expires_at in milliseconds for existing timer adapter'),
    ('GET', '/mobile-intake', '/api/mobile-intake', 'new-mobile-capability-and-current-actor-scope', 'none-read-only', 'T048',
     ['src/api/mobile-intake/route.ts', 'src/api/mobile-intake/access.ts', 'src/lib/mobile-intake-token.ts'],
     'Read X-Intake-Token only; verify purpose/length/signature/time/category; current actor enabled, source and category action rechecked every time; missing source never falls back to admin.', 'category/source(id,name)/expires_at only; no-store'),
    ('POST', '/mobile-intake', '/api/mobile-intake', 'new-mobile-capability-and-current-actor-scope', 'origin-and-explicit-capability-header', 'T048',
     ['src/api/mobile-intake/route.ts', 'src/api/mobile-intake/validators.ts', 'src/workflows/mobile-intake.ts', 'src/workflows/device-operations.ts', 'static/mobile-intake/app.js'],
     'Server derives source/category/actor from capability, never request override. Durable idempotency per session/command; preserve nonce in intake audit; stock quantity independent of device intake. No general device command exposure.', 'Intake receipt IDs only; private no-store; body according to MobileIntakeBody plus real domain checks'),
    ('GET', '/mobile-intake/variants', '/api/mobile-intake/variants', 'new-mobile-capability-and-current-actor-scope', 'none-read-only', 'T048',
     ['src/api/mobile-intake/variants/route.ts', 'src/lib/device-variant-search.ts'],
     'Bounded search max200 chars/12 terms, take20 offset<=100000, stable SKU/ID order; SKU-only variants; count semantics source-before-empty-SKU-filter needs explicit new consistency test.', 'id/sku/title/requires_imei; no raw product metadata or costs; no-store'),
    ('POST', '/mobile-intake/photos', '/api/mobile-intake/photos', 'new-mobile-capability-and-current-actor-scope', 'origin-and-explicit-capability-header', 'T048',
     ['src/api/mobile-intake/photos/route.ts', 'src/api/device-photo-validators.ts', 'src/workflows/device-photos.ts', 'src/api/middlewares.ts'],
     '1..8 files, declared base64 <=4194304 per file, actual JPEG/PNG/WebP header matches MIME and decoded<=3MiB; request cap34mb in source. Random server name; source creates public device photos, not private buyback/mail storage.', '201 photos(id,url); public image class separately authorized and documented; no-store response'),
    ('DELETE', '/mobile-intake/devices/{id}', '/api/mobile-intake/devices/{id}', 'new-mobile-capability-and-current-actor-scope', 'origin-and-explicit-capability-header', 'T048',
     ['src/api/mobile-intake/devices/[id]/route.ts', 'src/workflows/mobile-intake.ts', 'src/workflows/device-operations.ts'],
     'Only own actor intake event with same nonce/source/category, state pending/inspection/available, no sales assignment history; full current device delete guards. Nonce is not single-use; repeated HTTP delete needs durable replay while permission stays checked.', 'Private no-store; stable error mobile_intake.delete_denied without leaking foreign device'),
    ('GET', '/admin/devices/import-template', '/api/admin/devices/import-template', 'new-staff-template-download-permission-and-scope', 'none-read-only', 'T043',
     ['src/api/admin/devices/import-template/route.ts', 'src/lib/device-import-template.ts', 'src/lib/device-import-columns.ts', 'src/admin/components/device-workbench.tsx'],
     'Reject supplier; warehouse action/scope guard still applies. All variant pages1000 with stable SKU/ID order; preserve EN/ZH headers, text serial/IMEI/EID/SKU and SKU selection/formula behavior. Formula cells from untrusted values must remain literal strings.', 'XLSX attachment device-import-template.xlsx, exact Office MIME, no-store; no business mutation'),
    ('GET', '/mail/unsubscribe/{token}', '/mail/unsubscribe/{token}', 'business-optout-capability', 'none-read-only', 'T056',
     ['src/api/mail/unsubscribe/[token]/route.ts'],
     '64 lowercase hex syntax; GET renders bilingual confirmation, never updates subscription. Existing optout capabilities are business consent, not excluded login credentials; provider scanners cannot unsubscribe by reading link.', 'HTML with CSP default-src none/form-action self/frame-ancestors none; private,no-store; never log token path'),
    ('POST', '/mail/unsubscribe/{token}', '/mail/unsubscribe/{token}', 'business-optout-capability', 'capability-form-origin-policy-not-login-csrf', 'T056',
     ['src/api/mail/unsubscribe/[token]/route.ts', 'src/workflows/unsubscribe-mail.ts'],
     'Lookup protected token; atomically set subscribed=false without undoing suppression/consent history. Repeating valid unsubscribe returns success, invalid 404; no mail send or login required. Do not require storefront session for external email users.', '200 bilingual text success or404 invalid; no-store; capability route logging redacted'),
    ('POST', '/mail/webhooks/{account_id}', '/mail/webhooks/{account_id}', 'resend-signed-raw-body', 'provider-signature-not-browser-csrf', 'T055',
     ['src/api/mail/webhooks/[account_id]/route.ts', 'src/api/middlewares.ts', 'src/workflows/mail-webhook.ts', 'src/modules/mail/security.ts'],
     'Preserve exact raw body <=1mb, Svix id/timestamp/signature mapped into installed verifier. Require canonical integer timestamp and bounded skew; verify before parse/side effects. Scope dedupe account:event, persist accepted event/intent before async work and ack only durable acceptance; processing retries remain recoverable.', '401 invalid signature/config,204 durable acceptance; transient persistence failure non2xx; no credentials/raw payload logs'),
]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    entries = []
    hashes = {}
    for method, old, new, identity, csrf, task, sources, invariant, response in ENTRIES:
        for source in sources:
            file = BASE + source
            hashes[file] = hashlib.sha256((ROOT / file).read_bytes()).hexdigest()
        entries.append({'legacy_method': method, 'legacy_path': old, 'new_method': method, 'new_path': new,
                        'identity': identity, 'csrf_policy': csrf, 'task': task,
                        'sources': [BASE + s for s in sources], 'invariant': invariant,
                        'response': response, 'status': 'designed-not-implemented',
                        'remaining': 'Gin schemas/handlers, isolated permission/concurrency/file/browser tests; route coverage does not prove actual runtime behavior'})
    result = {'format_version': 1, 'scope': 'Reviewed mobile, template and mail boundary designs; not all external webhooks/native/plugin API coverage or implemented Gin behavior',
              'source_sha256': dict(sorted(hashes.items())), 'entries': entries}
    target = OUT / 'capability-entrypoint-contract.json'
    content = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        if target.read_text(encoding='utf-8-sig') != content:
            raise ValueError('Capability contract source/design drift; review before regeneration')
    else:
        target.write_text(content, encoding='utf-8', newline='')
    print(json.dumps({'entries': len(entries), 'source_files': len(hashes), 'status': 'design only'}))


if __name__ == '__main__':
    main()
