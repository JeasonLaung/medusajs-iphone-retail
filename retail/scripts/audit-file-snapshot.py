"""Read-only file/reference audit. Private paths and row identifiers stay ignored."""
import argparse
import base64
from collections import Counter
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
SOURCE_HASH = 'aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132'
SOURCES = [
    'medusa-store/apps/backend/medusa-config.ts',
    'medusa-store/apps/backend/src/workflows/device-photos.ts',
    'medusa-store/apps/backend/src/workflows/buyback.ts',
    'medusa-store/apps/backend/src/workflows/expire-buyback-photos.ts',
    'medusa-store/apps/backend/src/api/buyback-photo.ts',
    'medusa-store/apps/backend/src/api/store/buyback/requests/[id]/photos/[photo_id]/route.ts',
    'medusa-store/apps/backend/src/api/admin/buyback/[id]/photos/[photo_id]/route.ts',
    'medusa-store/apps/backend/src/api/admin/mail/messages/[id]/attachments/[attachment_id]/route.ts',
    'medusa-store/apps/backend/src/modules/mail/types.ts',
    'medusa-store/node_modules/.pnpm/@medusajs+file-local@2.21.2_131e369d4736c402c6ec81bbdb5721d5/node_modules/@medusajs/file-local/dist/services/local-file.js',
]


def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def safe_key(value):
    if not isinstance(value, str) or not value or '\\' in value or '\x00' in value:
        return None
    if any(ord(char) < 32 or char in ':<>"|?*' for char in value):
        return None
    parts = value.split('/')
    if value.startswith('/') or any(part in {'', '.', '..'} or part.endswith(('.', ' ')) for part in parts):
        return None
    return str(PurePosixPath(*parts))


def inventory(directory):
    result = {}
    def fail(error):
        raise ValueError('Storage enumeration failed; inspect privately') from None
    for base, dirs, files in os.walk(directory, followlinks=False, onerror=fail):
        for name in dirs:
            if (Path(base) / name).is_symlink() or (Path(base) / name).is_junction():
                raise ValueError('Linked storage directory rejected; inspect privately')
        for name in files:
            file = Path(base) / name
            if file.is_symlink():
                raise ValueError('Linked storage file rejected; inspect privately')
            relative = file.relative_to(directory).as_posix()
            if safe_key(relative) is None:
                raise ValueError('Nonportable storage filename; inspect privately')
            before = file.stat()
            digest = sha(file)
            after = file.stat()
            if (before.st_size, before.st_mtime_ns) != (after.st_size, after.st_mtime_ns):
                raise ValueError('Storage changed while hashing; rerun inventory')
            result[relative] = {'bytes': after.st_size, 'sha256': digest}
    return result


def image_mime(file):
    with file.open('rb') as handle:
        header = handle.read(12)
    if header.startswith(b'\xff\xd8\xff'):
        return 'image/jpeg'
    if header.startswith(b'\x89PNG\r\n\x1a\n'):
        return 'image/png'
    if header[:4] == b'RIFF' and header[8:12] == b'WEBP':
        return 'image/webp'
    return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True)
    parser.add_argument('--public-root', required=True)
    parser.add_argument('--private-root', required=True)
    parser.add_argument('--private-index', required=True)
    parser.add_argument('--report', default='docs/migration/file-snapshot-report.json')
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    bundle = Path(args.bundle).resolve()
    if sha(bundle) != SOURCE_HASH:
        raise ValueError('Unexpected source snapshot; review baseline before auditing')
    index = Path(args.private_index).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(index)], cwd=ROOT).returncode:
        raise ValueError('Detailed file index must be Git ignored')
    public = Path(args.public_root).resolve(strict=True)
    private = Path(args.private_root).resolve(strict=True)
    if public == private or public in private.parents or private in public.parents:
        raise ValueError('Public and private storage must be disjoint')
    if any(root == index or root in index.parents for root in [public, private]):
        raise ValueError('Private audit index cannot be written inside asset storage')
    files = {'public': inventory(public), 'private': inventory(private)}
    tables = json.loads(bundle.read_text(encoding='utf-8'))['tables']
    refs = []
    failures = Counter()
    observations = Counter()

    def add(kind, table, row, field, value, access, mime=None):
        ref = {'kind': kind, 'table': table, 'legacy_id': row.get('id'), 'field': field,
               'access': access, 'deleted': bool(row.get('deleted_at'))}
        if access == 'private':
            key = safe_key(value)
            ref['legacy_key'] = value
            if not key:
                ref['status'] = 'invalid-key'
            elif not key.startswith('private-'):
                ref['status'] = 'unexpected-private-key-prefix'
            elif key not in files['private']:
                ref['status'] = 'missing-local-file'
            else:
                ref.update(key=key, **files['private'][key], status='present')
                detected = image_mime(private / key)
                if mime and mime != detected:
                    ref['status'] = 'image-header-mismatch'
                if ref['bytes'] > 3 * 1024 * 1024:
                    ref['status'] = 'photo-size-exceeded'
                if key in files['public']:
                    ref['status'] = 'private-key-also-in-public-root'
        else:
            ref['legacy_url'] = value
            if not isinstance(value, str) or not value:
                ref['status'] = 'invalid-url'
            else:
                try:
                    url = urlsplit(value)
                    decoded = unquote(url.path, errors='strict')
                except (ValueError, UnicodeError):
                    ref['status'] = 'invalid-url'
                else:
                    if url.scheme and url.scheme not in {'https', 'http'}:
                        ref['status'] = 'unsupported-url-scheme'
                    elif decoded.startswith('/static/') and url.hostname and url.hostname not in {'localhost', '127.0.0.1', '::1'}:
                        ref['status'] = 'remote-not-fetched'
                        observations['external-static-origin-not-treated-as-local'] += 1
                    elif decoded.startswith('/static/'):
                        key = safe_key(decoded[len('/static/'):])
                        if not key:
                            ref['status'] = 'invalid-local-url-key'
                        elif key.startswith('private-') or key in files['private']:
                            ref['status'] = 'private-key-referenced-as-public'
                        elif key not in files['public']:
                            ref.update(key=key, status='missing-local-file')
                        else:
                            ref.update(key=key, **files['public'][key], status='present')
                        if url.query or url.fragment:
                            observations['local-url-query-or-fragment'] += 1
                        if url.hostname and url.hostname not in {'localhost', '127.0.0.1', '::1'}:
                            observations['static-url-external-host-needs-origin-review'] += 1
                    elif url.scheme in {'http', 'https'} and url.hostname:
                        ref['status'] = 'remote-not-fetched'
                    elif value.startswith('/') and not url.netloc:
                        ref['status'] = 'storefront-asset-reference-needs-separate-root'
                    else:
                        ref['status'] = 'unsupported-relative-url'
        if ref['status'] not in {'present', 'remote-not-fetched', 'storefront-asset-reference-needs-separate-root'}:
            failures[ref['status']] += 1
        refs.append(ref)

    for table in ['product', 'product_variant']:
        for row in tables[table]:
            if row.get('thumbnail'):
                add('catalog-thumbnail', table, row, 'thumbnail', row['thumbnail'], 'public')
    for row in tables['image']:
        add('catalog-image', 'image', row, 'url', row.get('url'), 'public')
    for row in tables['device']:
        photos = row.get('photos')
        if not isinstance(photos, list):
            failures['device-photos-not-array'] += 1
            continue
        for i, value in enumerate(photos):
            add('device-photo', 'device', row, f'photos[{i}]', value, 'public')
    for row in tables['buyback_photo']:
        if row.get('review_status') == 'expired' and not row.get('storage_key'):
            observations['expired-photo-no-object'] += 1
        else:
            add('buyback-photo', 'buyback_photo', row, 'storage_key', row.get('storage_key'), 'private', row.get('mime_type'))
    attachment_counts = Counter()
    for row in tables['mail_message']:
        attachments = row.get('attachments')
        if not isinstance(attachments, list):
            failures['mail-attachments-not-array'] += 1
            continue
        for i, item in enumerate(attachments):
            ref = {'kind': 'mail-attachment', 'table': 'mail_message', 'legacy_id': row['id'],
                   'index': i, 'access': 'private', 'deleted': bool(row.get('deleted_at'))}
            if not isinstance(item, dict):
                ref['status'] = 'invalid-attachment'
            elif item.get('content'):
                try:
                    data = base64.b64decode(item['content'], validate=True)
                    ref.update(status='inline-base64-present', bytes=len(data), sha256=hashlib.sha256(data).hexdigest())
                    if item.get('size') is not None and item['size'] != len(data):
                        ref['status'] = 'inline-size-mismatch'
                except (ValueError, TypeError):
                    ref['status'] = 'invalid-inline-base64'
            elif item.get('id') and row.get('provider_id') and row.get('account_id'):
                ref['status'] = 'provider-only-not-fetched'
            else:
                ref['status'] = 'attachment-locator-missing'
            attachment_counts[ref['status']] += 1
            if ref['status'] not in {'inline-base64-present', 'provider-only-not-fetched'}:
                failures[ref['status']] += 1
            refs.append(ref)
    referenced = {access: {r['key'] for r in refs if r['access'] == access and 'key' in r} for access in files}
    detailed = {'format_version': 1, 'source_snapshot_sha256': SOURCE_HASH,
                'roots': {'public': str(public), 'private': str(private)}, 'files': files, 'references': refs}
    # Detailed paths/IDs/hashes are private audit material, never part of stdout.
    detail_content = json.dumps(detailed, ensure_ascii=False, sort_keys=True, indent=2) + '\n'
    if args.check:
        if index.read_text(encoding='utf-8') != detail_content:
            raise ValueError('Private file index drift; inspect changes before regenerating')
    else:
        index.parent.mkdir(parents=True, exist_ok=True)
        index.write_text(detail_content, encoding='utf-8')
    report = {
        'format_version': 1, 'scope': 'Fixed DB snapshot versus current local storage; read-only, no network fetch/copy/delete/import. Counts include soft-deleted rows. Not a point-in-time filesystem snapshot or accepted migration.',
        'source_snapshot_sha256': SOURCE_HASH,
        'source_sha256': {source: sha(ROOT / source) for source in SOURCES},
        'detailed_index_sha256': sha(index),
        'storage': {access: {'files': len(entries), 'bytes': sum(e['bytes'] for e in entries.values()),
                            'referenced_keys': len(referenced[access]),
                            'not_referenced_by_audited_fields': len(set(entries) - referenced[access])}
                    for access, entries in files.items()},
        'references_by_kind': {kind: sum(r['kind'] == kind for r in refs) for kind in
                               ['catalog-thumbnail', 'catalog-image', 'device-photo', 'buyback-photo', 'mail-attachment']},
        'references_by_status': dict(sorted(Counter(r['status'] for r in refs).items())),
        'mail_attachment_status': dict(sorted(attachment_counts.items())),
        'failures': dict(sorted(failures.items())), 'observations': dict(sorted(observations.items())),
        'remaining': ['private root path must match live deployment configuration before cutover',
                      'remote URLs/provider attachments not downloaded or verified',
                      'storefront public assets/CSS/HTML and arbitrary metadata URLs require separate manifest',
                      'unreferenced files retained; no orphan deletion authorization',
                      'transaction-consistent file freeze/copy/hash and new authorized serving tests',
                      'old account exclusion means historical customer photos not assigned by email'],
        'status': 'failed-needs-review' if failures else 'local-files-audited-migration-not-accepted',
    }
    report_content = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
    target = ROOT / args.report
    if args.check:
        if target.read_text(encoding='utf-8-sig') != report_content:
            raise ValueError('Aggregate file report drift; inspect before regenerating')
    else:
        target.write_text(report_content, encoding='utf-8')
    print(json.dumps({'storage': report['storage'], 'references_by_status': report['references_by_status'],
                      'failures': report['failures'], 'status': report['status']}))
    return 1 if failures else 0


if __name__ == '__main__':
    raise SystemExit(main())
