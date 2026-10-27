"""Pinned merchant configurations in an owned disposable disconnected MySQL.

Source keys and independent Node expectations travel through anonymous stdin.
Source snapshot and target rows live in tmpfs. New target keys stay in Go memory.
No application DSN, source connection, provider or production target is used.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.merchant-config-rehearsal'
PINNED_SHA = 'b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985'


def docker(*args, input=None, timeout=120):
    return subprocess.run(['docker', *args], input=input, capture_output=True, timeout=timeout)


def ignored(path):
    return subprocess.run(['git', 'check-ignore', '--quiet', str(path)], cwd=ROOT, capture_output=True).returncode == 0


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--snapshot', type=Path, required=True)
    parser.add_argument('--env', type=Path, required=True)
    args = parser.parse_args()
    snapshot, env_file = args.snapshot.resolve(), args.env.resolve()
    if not all(path.is_file() and ignored(path) for path in (snapshot, env_file)):
        raise RuntimeError('Explicit ignored snapshot and source key file required')
    if hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Pinned private v2 snapshot required')
    ddl = ROOT / 'docs/migration/mysql-relational-review.sql'
    review = json.loads((ROOT / 'docs/migration/mysql-relational-review.json').read_text(encoding='utf-8'))
    if digest(ddl) != review['ddl_sha256']:
        raise RuntimeError('Reviewed DDL checksum differs')
    if docker('image', 'inspect', 'mysql:8.4', '--format', '{{.Id}}').returncode:
        raise RuntimeError('Local mysql:8.4 image required; no pull performed')
    owner = uuid.uuid4().hex
    executable = ROOT / 'logs/gin-migration' / ('merchant-rehearsal-' + owner + '.test')
    if not ignored(executable) or executable.exists():
        raise RuntimeError('New ignored runner path required')
    environment = os.environ.copy()
    environment.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    container = None
    binary_owned = False
    password = secrets.token_urlsafe(32)
    report = None
    private_payload = b''
    try:
        binary_owned = True
        compiled = subprocess.run(['go', 'test', '-c', '-o', str(executable), './internal/merchantimport'],
                                  cwd=ROOT / 'retail/backend', env=environment, capture_output=True, timeout=120)
        if compiled.returncode:
            raise RuntimeError('Merchant rehearsal runner compilation failed')
        private_input = subprocess.run(['node', str(ROOT / 'retail/scripts/merchant-rehearsal-input.cjs'),
                                        str(snapshot), str(env_file)], cwd=ROOT, capture_output=True, timeout=60)
        if private_input.returncode or not private_input.stdout or len(private_input.stdout) > 1 << 20:
            raise RuntimeError('Independent private source expectations unavailable; private output suppressed')
        private_payload = private_input.stdout
        private_input = None
        started = docker('run', '-d', '--name', 'eurorefurb-merchant-rehearsal-' + owner[:12],
                         '--label', LABEL + '=' + owner, '--network', 'none', '--cpus', '1',
                         '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                         '--tmpfs', '/var/lib/mysql:rw,size=512m', '--tmpfs', '/tmp:rw,size=128m,mode=1777',
                         '-e', 'MYSQL_ROOT_PASSWORD=' + password, '-e', 'MYSQL_DATABASE=merchant_rehearsal',
                         'mysql:8.4', '--skip-log-bin', '--innodb-buffer-pool-size=64M',
                         '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=8')
        if started.returncode:
            raise RuntimeError('Isolated merchant rehearsal creation failed')
        container = started.stdout.decode('ascii').strip()

        def sql(query):
            result = docker('exec', '-i', '-e', 'MYSQL_PWD=' + password, container,
                            'mysql', '--user=root', '--protocol=socket', '--batch', '--raw',
                            '--skip-column-names', '--default-character-set=utf8mb4', 'merchant_rehearsal',
                            input=query.encode('utf-8'))
            if result.returncode:
                raise RuntimeError('Isolated merchant SQL failed; private output suppressed')
            return result.stdout.decode('utf-8').strip()

        for _ in range(100):
            ready = docker('exec', '-e', 'MYSQL_PWD=' + password, container, 'mysql', '-uroot', '--protocol=socket',
                           '--batch', '--skip-column-names', '-e', 'SELECT 1', timeout=10)
            if ready.returncode == 0:
                break
            time.sleep(0.5)
        else:
            raise RuntimeError('Isolated merchant MySQL did not become ready')
        version = sql('SELECT VERSION();')
        sql(ddl.read_text(encoding='utf-8'))
        if sql("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='merchant_rehearsal';") != '159':
            raise RuntimeError('Complete review schema not created')
        for migration in ['0002_unique_key_registry.sql', '0003_unique_key_secret_scopes.sql', '0006_migration_merchant_dispositions.sql']:
            sql((ROOT / 'retail/backend/migrations' / migration).read_text(encoding='utf-8'))
        # Fixed remote paths only. Stream into the live tmpfs mount rather than
        # using an archive copy that can bypass it on this Docker host.
        for local, remote in [(executable, '/opt/merchant-rehearsal.test'), (snapshot, '/tmp/merchant-source.ndjson')]:
            with local.open('rb') as private_file:
                transferred = subprocess.run(['docker', 'exec', '-i', container, '/bin/sh', '-c', 'umask 077; cat > ' + remote],
                                             stdin=private_file, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
            if transferred.returncode:
                raise RuntimeError('Private merchant input transfer failed')
        if docker('exec', container, 'chmod', '700', '/opt/merchant-rehearsal.test').returncode:
            raise RuntimeError('Merchant runner preparation failed')
        tested = docker('exec', '-i', '-e', 'MYSQL_PWD=' + password, '-e', 'MERCHANT_MYSQL_REHEARSAL=1', container,
                        '/opt/merchant-rehearsal.test', '-test.v', '-test.run', '^TestActualMerchantMySQL$',
                        input=private_payload, timeout=150)
        private_payload = b''
        markers = [line.split('MERCHANT_MYSQL_REHEARSAL ', 1)[1]
                   for line in tested.stdout.decode('utf-8').splitlines() if 'MERCHANT_MYSQL_REHEARSAL ' in line]
        if tested.returncode or len(markers) != 1:
            reasons = re.findall(r't\.Fatal\("([^"\n]+)"\)',
                                 (ROOT / 'retail/backend/internal/merchantimport/rehearsal_test.go').read_text(encoding='utf-8'))
            reason = next((item for item in reasons if item.encode() in tested.stdout), None)
            raise RuntimeError('Actual merchant rehearsal failed: ' + (reason or 'unclassified runner failure') + '; private output suppressed')
        counts = json.loads(markers[0])
        files = ['retail/scripts/rehearse-merchant-config.py', 'retail/scripts/merchant-rehearsal-input.cjs',
                 'retail/scripts/audit-protected-config.cjs', 'retail/backend/internal/merchantimport/import.go',
                 'retail/backend/internal/merchantimport/rehearsal_test.go', 'retail/backend/internal/rowimport/config_secret.go',
                 'retail/backend/internal/rowimport/paypal_config_json.go', 'retail/backend/internal/rowimport/paypal_credentials.go',
                 'retail/backend/internal/rowimport/convert.go', 'retail/backend/internal/rowimport/field_plans.json',
                 'retail/backend/internal/rowimport/stream.go', 'retail/backend/internal/rowimport/relationships.go',
                 'retail/backend/internal/legacysecret/decrypt.go', 'retail/backend/internal/legacysecret/paypal.go',
                 'retail/backend/internal/protected/envelope.go', 'retail/backend/internal/uniquekey/registry.go',
                 'retail/backend/internal/uniquekey/source_plans.json', 'retail/backend/migrations/0002_unique_key_registry.sql',
                 'retail/backend/migrations/0003_unique_key_secret_scopes.sql', 'retail/backend/migrations/0006_migration_merchant_dispositions.sql',
                 'docs/migration/mysql-relational-review.sql',
                 'medusa-store/node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/lib/main.js',
                 'medusa-store/apps/backend/src/modules/mail/security.ts', 'medusa-store/apps/backend/src/lib/seo/core.ts',
                 'medusa-store/apps/backend/src/modules/webhook-notifications/security.ts',
                 'medusa-store/apps/backend/src/modules/mollie-settings/security.ts']
        report = {'format_version': 1, 'scope': 'Five actual pinned merchant configurations in disposable tmpfs MySQL only; not full migration, nonempty PayPal credential or runtime acceptance',
                  'snapshot_sha256': PINNED_SHA, 'mysql_version': version, 'counts': counts,
                  'limits': {'cpus': 1, 'memory_mib': 768, 'mysql_tmpfs_mib': 512, 'private_input_tmpfs_mib': 128, 'network': 'none', 'published_ports': 0},
                  'private_legacy_keys_and_expectations_stdin_only': True, 'target_business_rows_written': True,
                  'target_operation_holds_written': True, 'target_full_value_claims_written': True,
                  'production_target_used': False, 'source_database_connection': False, 'provider_called': False,
                  'new_keys_persisted': False, 'private_values_in_report': False, 'relational_acceptance': False,
                  'selected_env_proves_live_runtime_keys': False, 'empty_source_tables_not_accepted_as_real_samples': ['seo_provider', 'seo_setting'],
                  'implementation_sha256': {file: digest(ROOT / file) for file in files}}
    finally:
        private_payload = b''
        password = ''
        if container:
            ownership = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if ownership.returncode or ownership.stdout.decode().strip() != owner:
                raise RuntimeError('Merchant rehearsal ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned merchant rehearsal cleanup failed')
        if binary_owned and executable.exists():
            executable.unlink()
    if report:
        report['cleanup'] = 'owned container removed after label verification; tmpfs snapshot and target copies and temporary runner removed'
        (ROOT / 'docs/migration/merchant-mysql-rehearsal-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'counts': report['counts'], 'cleanup': 'complete', 'production_target_used': False}))


if __name__ == '__main__':
    main()
