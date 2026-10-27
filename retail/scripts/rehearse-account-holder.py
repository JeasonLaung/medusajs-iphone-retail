"""Real pinned historical holder/provider in an owned disposable tmpfs MySQL.

No application DSN, source database, deployment server or provider is contacted.
Source and target data live in tmpfs; the new target key stays in Go memory.
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
LABEL = 'eurorefurb.account-holder-rehearsal'
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
    snapshot = parser.parse_args().snapshot.resolve()
    if not snapshot.is_file() or not ignored(snapshot) or hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Pinned ignored v2 snapshot required')
    ddl = ROOT / 'docs/migration/mysql-relational-review.sql'
    review = json.loads((ROOT / 'docs/migration/mysql-relational-review.json').read_text(encoding='utf-8'))
    if digest(ddl) != review['ddl_sha256']:
        raise RuntimeError('Reviewed DDL checksum differs')
    if docker('image', 'inspect', 'mysql:8.4', '--format', '{{.Id}}').returncode:
        raise RuntimeError('Local mysql:8.4 image required; no pull performed')
    owner = uuid.uuid4().hex
    executable = ROOT / 'logs/gin-migration' / ('holder-rehearsal-' + owner + '.test')
    if not ignored(executable) or executable.exists():
        raise RuntimeError('New ignored runner path required')
    environment = os.environ.copy()
    environment.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    container, report = None, None
    binary_owned = False
    password = secrets.token_urlsafe(32)
    try:
        binary_owned = True
        compiled = subprocess.run(['go', 'test', '-c', '-o', str(executable), './internal/holderimport'],
                                  cwd=ROOT / 'retail/backend', env=environment, capture_output=True, timeout=120)
        if compiled.returncode:
            raise RuntimeError('Holder rehearsal runner compilation failed')
        started = docker('run', '-d', '--name', 'eurorefurb-holder-rehearsal-' + owner[:12],
                         '--label', LABEL + '=' + owner, '--network', 'none', '--cpus', '1',
                         '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                         '--tmpfs', '/var/lib/mysql:rw,size=512m', '--tmpfs', '/tmp:rw,size=128m,mode=1777',
                         '-e', 'MYSQL_ROOT_PASSWORD=' + password, '-e', 'MYSQL_DATABASE=holder_rehearsal',
                         'mysql:8.4', '--skip-log-bin', '--innodb-buffer-pool-size=64M',
                         '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=8')
        if started.returncode:
            raise RuntimeError('Isolated holder rehearsal creation failed')
        container = started.stdout.decode('ascii').strip()

        def sql(query):
            result = docker('exec', '-i', '-e', 'MYSQL_PWD=' + password, container,
                            'mysql', '--user=root', '--protocol=socket', '--batch', '--raw',
                            '--skip-column-names', '--default-character-set=utf8mb4', 'holder_rehearsal',
                            input=query.encode('utf-8'))
            if result.returncode:
                raise RuntimeError('Isolated holder SQL failed; private output suppressed')
            return result.stdout.decode('utf-8').strip()

        for _ in range(100):
            ready = docker('exec', '-e', 'MYSQL_PWD=' + password, container, 'mysql', '-uroot', '--protocol=socket',
                           '--batch', '--skip-column-names', '-e', 'SELECT 1', timeout=10)
            if ready.returncode == 0:
                break
            time.sleep(0.5)
        else:
            raise RuntimeError('Isolated holder MySQL did not become ready')
        version = sql('SELECT VERSION();')
        sql(ddl.read_text(encoding='utf-8'))
        if sql("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='holder_rehearsal';") != '159':
            raise RuntimeError('Complete holder review schema not created')
        for name in ['0002_unique_key_registry.sql', '0003_unique_key_secret_scopes.sql', '0007_migration_holder_dispositions.sql']:
            sql((ROOT / 'retail/backend/migrations' / name).read_text(encoding='utf-8'))
        # Private input is streamed into the live tmpfs mount. The non-private
        # executable uses /opt because tmpfs has noexec on this Docker host.
        for local, remote in [(executable, '/opt/holder-rehearsal.test'), (snapshot, '/tmp/holder-source.ndjson')]:
            with local.open('rb') as private_file:
                transferred = subprocess.run(['docker', 'exec', '-i', container, '/bin/sh', '-c', 'umask 077; cat > ' + remote],
                                             stdin=private_file, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
            if transferred.returncode:
                raise RuntimeError('Private holder input transfer failed')
        if docker('exec', container, 'chmod', '700', '/opt/holder-rehearsal.test').returncode:
            raise RuntimeError('Holder runner preparation failed')
        tested = docker('exec', '-e', 'MYSQL_PWD=' + password, '-e', 'HOLDER_MYSQL_REHEARSAL=1', container,
                        '/opt/holder-rehearsal.test', '-test.v', '-test.run', '^TestActualHolderMySQL$', timeout=150)
        markers = [line.split('HOLDER_MYSQL_REHEARSAL ', 1)[1]
                   for line in tested.stdout.decode('utf-8').splitlines() if 'HOLDER_MYSQL_REHEARSAL ' in line]
        if tested.returncode or len(markers) != 1:
            reasons = re.findall(r't\.Fatal\("([^"\n]+)"\)',
                                 (ROOT / 'retail/backend/internal/holderimport/rehearsal_test.go').read_text(encoding='utf-8'))
            reason = next((item for item in reasons if item.encode() in tested.stdout), None)
            raise RuntimeError('Actual holder rehearsal failed: ' + (reason or 'unclassified runner failure') + '; private output suppressed')
        counts = json.loads(markers[0])
        files = ['retail/scripts/rehearse-account-holder.py', 'retail/backend/internal/holderimport/import.go',
                 'retail/backend/internal/holderimport/rehearsal_test.go', 'retail/backend/internal/rowimport/history_account_holder.go',
                 'retail/backend/internal/rowimport/convert.go', 'retail/backend/internal/rowimport/record.go',
                 'retail/backend/internal/rowimport/stream.go', 'retail/backend/internal/rowimport/relationships.go',
                 'retail/backend/internal/rowimport/field_plans.json', 'retail/backend/internal/protected/envelope.go',
                 'retail/backend/internal/uniquekey/registry.go', 'retail/backend/internal/uniquekey/source.go',
                 'retail/backend/internal/uniquekey/source_plans.json', 'retail/backend/migrations/0002_unique_key_registry.sql',
                 'retail/backend/migrations/0003_unique_key_secret_scopes.sql', 'retail/backend/migrations/0007_migration_holder_dispositions.sql',
                 'docs/migration/mysql-relational-review.sql']
        report = {'format_version': 1, 'scope': 'One actual historical system-provider actor and related provider in disposable tmpfs MySQL only; no customer binding or full import/runtime acceptance',
                  'snapshot_sha256': PINNED_SHA, 'mysql_version': version, 'counts': counts,
                  'limits': {'cpus': 1, 'memory_mib': 768, 'mysql_tmpfs_mib': 512, 'private_input_tmpfs_mib': 128, 'network': 'none', 'published_ports': 0},
                  'target_business_rows_written': True, 'target_operation_holds_written': True, 'target_full_value_claims_written': True,
                  'production_target_used': False, 'source_database_connection': False, 'provider_called': False,
                  'new_keys_persisted': False, 'private_values_in_report': False, 'relational_acceptance': False,
                  'legacy_customer_payment_links_restored': False,
                  'implementation_sha256': {file: digest(ROOT / file) for file in files}}
    finally:
        password = ''
        if container:
            ownership = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if ownership.returncode or ownership.stdout.decode().strip() != owner:
                raise RuntimeError('Holder rehearsal ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned holder rehearsal cleanup failed')
        if binary_owned and executable.exists():
            executable.unlink()
    if report:
        report['cleanup'] = 'owned container removed after label verification; tmpfs source/target copies and temporary runner removed'
        (ROOT / 'docs/migration/account-holder-mysql-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'counts': report['counts'], 'cleanup': 'complete', 'production_target_used': False}))


if __name__ == '__main__':
    main()
