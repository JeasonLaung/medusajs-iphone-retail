"""Artificial session/CSRF/limiter checks in an owned no-network Redis only."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.session-rehearsal'


def docker(*args, timeout=60):
    return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)


def main():
    if docker('image', 'inspect', 'redis:7-alpine').returncode:
        raise RuntimeError('Existing local redis:7-alpine image required; no pull performed')
    owner = uuid.uuid4().hex
    binary = ROOT / 'logs/gin-migration' / ('redis-sessions-' + owner + '.test')
    if binary.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(binary)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('New ignored runner path required')
    container, owned_binary = None, False
    env = os.environ.copy()
    env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    try:
        owned_binary = True
        built = subprocess.run(['go', 'test', '-c', '-o', str(binary), './internal/sessionstore'],
                               cwd=ROOT / 'retail/backend', env=env, capture_output=True, timeout=120)
        if built.returncode:
            raise RuntimeError('Session runner compilation failed')
        started = docker('run', '-d', '--name', 'eurorefurb-session-rehearsal-' + owner[:12],
                         '--label', LABEL + '=' + owner, '--network', 'none', '--cpus', '1',
                         '--memory', '128m', '--memory-swap', '128m', '--pids-limit', '64',
                         '--tmpfs', '/data:rw,size=16m', '--tmpfs', '/tmp:rw,size=16m,mode=1777',
                         'redis:7-alpine', 'redis-server', '--port', '0', '--unixsocket', '/tmp/redis.sock',
                         '--unixsocketperm', '700', '--save', '', '--appendonly', 'no', '--maxmemory', '16mb', '--maxmemory-policy', 'noeviction')
        if started.returncode:
            raise RuntimeError('Owned isolated Redis creation failed')
        container = started.stdout.decode().strip()
        for _ in range(50):
            ready = docker('exec', container, 'redis-cli', '-s', '/tmp/redis.sock', 'PING')
            if ready.returncode == 0 and ready.stdout.strip() == b'PONG':
                break
            time.sleep(.1)
        else:
            raise RuntimeError('Owned Redis failed to become ready')
        with binary.open('rb') as source:
            copied = subprocess.run(['docker', 'exec', '-i', container, '/bin/sh', '-c', 'umask 077; cat > /opt/session.test'],
                                    stdin=source, capture_output=True, timeout=60)
        if copied.returncode or docker('exec', container, 'chmod', '700', '/opt/session.test').returncode:
            raise RuntimeError('Session runner transfer failed')
        tested = docker('exec', '-e', 'SESSION_REDIS_REHEARSAL=1', container, '/opt/session.test',
                        '-test.v', '-test.run', '^TestOwnedRedis(Sessions|CartPossession)$', '-test.timeout', '60s')
        markers = [line.split('SESSION_REDIS_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'SESSION_REDIS_REHEARSAL ' in line]
        cart_markers = [line.split('CART_POSSESSION_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'CART_POSSESSION_REHEARSAL ' in line]
        if tested.returncode or len(markers) != 1 or len(cart_markers) != 1:
            # Test strings are artificial and fixed; no source snapshots or
            # real tokens/values are ever passed to this test binary.
            print(tested.stdout.decode(errors='replace'))
            raise RuntimeError('Owned Redis session checks failed')
        counts = json.loads(markers[0])
        files = ['retail/scripts/test-redis-sessions.py', 'retail/backend/internal/sessionstore/store.go',
                 'retail/backend/internal/sessionstore/config.go', 'retail/backend/internal/sessionstore/config_test.go',
                 'retail/backend/internal/sessionstore/cart.go', 'retail/backend/internal/sessionstore/cart_test.go',
                 'retail/backend/internal/sessionstore/redis_test.go', 'retail/backend/go.mod', 'retail/backend/go.sum']
        report = {'format_version': 1, 'scope': 'Artificial actual Redis component test only; HTTP/MySQL/role revocation/login not accepted',
                  'counts': counts, 'cart_possession': json.loads(cart_markers[0]),
                  'limits': {'cpus': 1, 'memory_mib': 128, 'network': 'none', 'published_ports': 0},
                  'real_accounts_used': False, 'application_redis_used': False, 'source_database_connection': False,
                  'private_values_in_report': False, 'runtime_login_accepted': False,
                  'implementation_sha256': {name: hashlib.sha256((ROOT / name).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for name in files}}
    finally:
        if container:
            inspected = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if inspected.returncode or inspected.stdout.decode().strip() != owner:
                raise RuntimeError('Redis rehearsal ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned Redis cleanup failed')
        if owned_binary and binary.exists():
            binary.unlink()
    report['cleanup'] = 'owned container and temporary runner removed'
    (ROOT / 'docs/migration/redis-session-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'counts': counts, 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
