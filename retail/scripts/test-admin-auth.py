"""New artificial admin over real Gin dispatch, owned MySQL and Redis only."""
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.identity-rehearsal'


def docker(*args, timeout=90):
    return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)


def main():
    for image in ['mysql:8.4', 'redis:7-alpine']:
        if docker('image', 'inspect', image).returncode:
            raise RuntimeError('Existing MySQL/Redis fixture images required; no pull performed')
    owner = uuid.uuid4().hex
    binary = ROOT / 'logs/gin-migration' / ('identity-' + owner + '.test')
    if binary.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(binary)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('New ignored runner path required')
    env = os.environ.copy()
    env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    containers, owned_binary = [], False
    password = secrets.token_urlsafe(32)
    try:
        owned_binary = True
        built = subprocess.run(['go', 'test', '-c', '-o', str(binary), './internal/identity'],
                               cwd=ROOT / 'retail/backend', env=env, capture_output=True, timeout=120)
        if built.returncode:
            raise RuntimeError('Identity runner compilation failed')
        mysql = docker('run', '-d', '--name', 'eurorefurb-identity-mysql-' + owner[:12],
                       '--label', LABEL + '=' + owner, '--network', 'none', '--cpus', '1',
                       '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                       '--tmpfs', '/var/lib/mysql:rw,size=512m', '--tmpfs', '/tmp:rw,size=32m,mode=1777',
                       '-e', 'MYSQL_ROOT_PASSWORD=' + password, '-e', 'MYSQL_DATABASE=identity_rehearsal',
                       'mysql:8.4', '--skip-log-bin', '--innodb-buffer-pool-size=64M',
                       '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=16')
        if mysql.returncode:
            raise RuntimeError('Owned MySQL creation failed')
        mysql_id = mysql.stdout.decode().strip()
        containers.append(mysql_id)
        # Redis shares ONLY the owned no-network namespace. Loopback TCP is
        # inside these two containers, never published on the user's machine.
        redis = docker('run', '-d', '--name', 'eurorefurb-identity-redis-' + owner[:12],
                       '--label', LABEL + '=' + owner, '--network', 'container:' + mysql_id,
                       '--cpus', '1', '--memory', '128m', '--memory-swap', '128m', '--pids-limit', '64',
                       '--tmpfs', '/data:rw,size=16m', 'redis:7-alpine', 'redis-server',
                       '--bind', '127.0.0.1', '--save', '', '--appendonly', 'no',
                       '--maxmemory', '16mb', '--maxmemory-policy', 'noeviction')
        if redis.returncode:
            raise RuntimeError('Owned Redis creation failed')
        redis_id = redis.stdout.decode().strip()
        containers.append(redis_id)
        for _ in range(100):
            ready = docker('exec', '-e', 'MYSQL_PWD=' + password, mysql_id, 'mysql', '-uroot', '--protocol=socket', '-e', 'SELECT 1', timeout=10)
            if ready.returncode == 0:
                break
            time.sleep(.5)
        else:
            raise RuntimeError('Owned MySQL did not become ready')
        if docker('exec', redis_id, 'redis-cli', 'PING').stdout.strip() != b'PONG':
            raise RuntimeError('Owned Redis not ready')
        with binary.open('rb') as source:
            copied = subprocess.run(['docker', 'exec', '-i', mysql_id, '/bin/sh', '-c', 'umask 077; cat > /opt/identity.test'],
                                    stdin=source, capture_output=True, timeout=60)
        if copied.returncode or docker('exec', mysql_id, 'chmod', '700', '/opt/identity.test').returncode:
            raise RuntimeError('Identity runner transfer failed')
        tested = docker('exec', '-e', 'MYSQL_PWD=' + password, '-e', 'IDENTITY_REHEARSAL=1', mysql_id,
                        '/opt/identity.test', '-test.v', '-test.run', '^TestOwned(AdminAuthentication|CatalogSchemaTransition)$', '-test.timeout', '150s', timeout=180)
        markers = [line.split('IDENTITY_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'IDENTITY_REHEARSAL ' in line]
        if tested.returncode or len(markers) != 1:
            # All inputs and failure messages in this binary are artificial.
            print(tested.stdout.decode(errors='replace'))
            raise RuntimeError('Owned MySQL/Redis authentication checks failed')
        counts = json.loads(markers[0])
        schema_markers = [line.split('CATALOG_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CATALOG_SCHEMA_REHEARSAL ' in line]
        if len(schema_markers)!=1:
            raise RuntimeError('Catalog schema verification missing')
        files = ['retail/scripts/test-admin-auth.py', 'retail/backend/internal/identity/admin.go',
                 'retail/backend/internal/identity/admin_test.go', 'retail/backend/internal/identity/integration_test.go',
                 'retail/backend/internal/server/router.go', 'retail/backend/internal/platform/http.go',
                 'retail/backend/internal/platform/request.go', 'retail/backend/cmd/api/main.go',
                 'retail/backend/cmd/create-admin/main.go', 'retail/backend/migrations/0009_admin_auth_version.sql',
                 'retail/backend/internal/sessionstore/store.go', 'retail/backend/internal/sessionstore/config.go']
        files += ['retail/backend/internal/identity/catalog_schema_test.go','retail/backend/internal/schema/ready.go',
                  'retail/backend/internal/catalog/schema.go','retail/backend/internal/catalog/schema_plan.json',
                  'retail/backend/migrations/0010_catalog_relational.sql']
        report = {'format_version': 1, 'scope': 'New artificial admin with real Gin dispatch/MySQL/Redis; not customer/authz/UI acceptance',
                  'counts': counts, 'limits': {'mysql_cpus': 1, 'mysql_memory_mib': 768, 'redis_cpus': 1, 'redis_memory_mib': 128,
                                              'network': 'owned shared namespace with no external network', 'published_ports': 0},
                  'real_accounts_used': False, 'application_resources_used': False, 'private_values_in_report': False,
                  'full_authentication_accepted': False,
                  'catalog_schema_transition': json.loads(schema_markers[0]),
                  'implementation_sha256': {name: hashlib.sha256((ROOT / name).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for name in files}}
    finally:
        password = ''
        for container in reversed(containers):
            inspected = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if inspected.returncode or inspected.stdout.decode().strip() != owner:
                raise RuntimeError('Identity fixture ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned identity fixture cleanup failed')
        if owned_binary and binary.exists():
            binary.unlink()
    report['cleanup'] = 'both owned containers and temporary runner removed'
    (ROOT / 'docs/migration/admin-auth-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'counts': counts, 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
