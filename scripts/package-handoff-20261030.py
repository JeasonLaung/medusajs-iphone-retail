"""Read-only project database/data handoff. Private outputs stay Git-ignored."""
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
import shutil
import subprocess
import uuid
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def run(args, **kwargs):
    result = subprocess.run(args, capture_output=True, timeout=180, **kwargs)
    if result.returncode:
        raise RuntimeError('Backup operation failed; private command output suppressed')
    return result.stdout


def main():
    stamp = datetime.now().strftime('%Y%m%d-%H%M%S')
    folder = ROOT / 'backups' / ('handoff-' + stamp)
    archive = folder.with_suffix('.zip')
    for path in (folder, archive):
        if path.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(path)], cwd=ROOT).returncode:
            raise RuntimeError('A new Git-ignored destination is required')
    folder.mkdir(parents=True)
    database = folder / 'postgres' / 'medusa_trial.dump'
    database.parent.mkdir()
    # pg_dump uses one consistent snapshot. No role passwords or remote server.
    with database.open('xb') as output:
        result = subprocess.run([
            'docker', 'exec', 'medusa-trial-postgres-1', 'pg_dump', '-U', 'medusa',
            '-d', 'medusa_trial', '--format=custom', '--compress=6', '--no-owner', '--no-acl',
        ], stdout=output, stderr=subprocess.PIPE, timeout=180)
    if result.returncode or database.stat().st_size < 1024:
        raise RuntimeError('PostgreSQL dump failed; details suppressed')
    toc = run(['docker', 'exec', '-i', 'medusa-trial-postgres-1', 'pg_restore', '--list'], input=database.read_bytes())
    if b'TABLE DATA' not in toc:
        raise RuntimeError('PostgreSQL dump directory verification failed')
    (database.parent / 'dump-contents.txt').write_bytes(toc)

    # The legacy Redis is a separate optional source backup, never Gin sessions.
    remote = '/tmp/medusa-handoff-' + uuid.uuid4().hex + '.rdb'
    redis = folder / 'redis' / 'medusa-trial.rdb'
    redis.parent.mkdir()
    try:
        run(['docker', 'exec', 'medusa-trial-redis-1', 'redis-cli', '--rdb', remote])
        run(['docker', 'exec', 'medusa-trial-redis-1', 'redis-check-rdb', remote])
        run(['docker', 'cp', 'medusa-trial-redis-1:' + remote, str(redis)])
    finally:
        run(['docker', 'exec', 'medusa-trial-redis-1', 'rm', '-f', remote])

    # Preserve source files, frozen migration snapshots and UI evidence. Runtime
    # binaries/dependencies are reinstalled on the destination computer.
    copied = 0
    for relative in ['imports', 'artifacts', 'logs/gin-migration',
                     'medusa-store/apps/backend/static',
                     'medusa-store/apps/backend/.private-files', '.private-files']:
        source = ROOT / relative
        if not source.exists():
            continue
        for path in source.rglob('*'):
            if not path.is_file() or path.is_symlink():
                continue
            rel = path.relative_to(ROOT)
            if path.suffix.lower() in ('.exe', '.test', '.dll') or any(
                part == 'node_modules' or part.startswith('node-v') for part in rel.parts
            ):
                continue
            dest = folder / 'workspace' / rel
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, dest)
            copied += 1

    configs = []
    for relative in ['.env', '.admin-credentials', 'medusa-store/apps/backend/.env',
                     'medusa-store/apps/storefront/.env', 'medusa-store/apps/storefront/.env.local',
                     'retail/backend/.env', 'retail/admin/.env', 'retail/storefront/.env']:
        source = ROOT / relative
        if source.is_file():
            dest = folder / 'private-config' / relative
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, dest)
            configs.append(relative)

    instructions = """# Private migration handoff

Read task20261030.md from the Git branch codex/gin-react-astro first.
This archive contains business data, private images and local secrets; it is not in Git.

1. Clone/pull the code branch. Install Docker, Go 1.26+, Node 22.23.4+ and Python.
2. Extract this archive outside the Git checkout. Verify the SHA256 sidecar.
3. Copy workspace/ contents into the checkout, preserving relative paths.
   Old path marker files in logs are evidence only: never reuse another computer's
   D:/ temporary backend snapshots. Run tests against the current retail/backend.
4. Review private-config/ manually. Copy needed local env files to matching paths,
   update paths/origins, and choose new dedicated Gin keys and sessions.
5. For the OLD Medusa baseline only, use a NEW/EMPTY PostgreSQL 16 database.
   With the root Compose postgres running and medusa_trial empty:
     docker cp postgres/medusa_trial.dump medusa-trial-postgres-1:/tmp/handoff.dump
     docker exec medusa-trial-postgres-1 pg_restore -U medusa -d medusa_trial --no-owner --no-acl --exit-on-error /tmp/handoff.dump
   This command adds objects: refuse to run it over an existing business database.
   Run start.bat for the old development services after restoring assets/config.
6. The Redis RDB is an optional backup of the OLD project's transient state.
   A fresh Redis is preferred for development; do not import these old sessions
   into Gin. The new application uses its own Redis namespace and new accounts.
7. No persistent Gin/MySQL application database was configured/running at backup
   time. MySQL regression databases are disposable and already cleaned up.
   Install mysql:8.4 and redis:7-alpine for isolated rehearsal scripts; initialize
   new permanent Gin data through reviewed import tools, never old login records.

PostgreSQL custom dump TOC was read with pg_restore --list; no full restore was run.
Redis RDB was checked by redis-check-rdb. Every archive member has a manifest SHA256.
See task20261030.md for the remaining full SQL regression failure and next tasks.
"""
    (folder / 'RESTORE.md').write_text(instructions, encoding='utf-8')
    members = {}
    for path in sorted(folder.rglob('*')):
        if path.is_file():
            members[path.relative_to(folder).as_posix()] = {
                'bytes': path.stat().st_size,
                'sha256': hashlib.sha256(path.read_bytes()).hexdigest(),
            }
    manifest = {
        'created_at_utc': datetime.now(timezone.utc).isoformat(),
        'branch': run(['git', 'branch', '--show-current'], cwd=ROOT).decode().strip(),
        'head_before_handoff_commit': run(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip(),
        'source_postgres_container': 'medusa-trial-postgres-1',
        'source_database': 'medusa_trial',
        'postgres_toc_verified': True, 'full_restore_tested': False,
        'redis_rdb_verified': True, 'persistent_gin_mysql_present': False,
        'workspace_files': copied, 'private_configuration_files': configs,
        'files': members,
    }
    (folder / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    with zipfile.ZipFile(archive, 'x', compression=zipfile.ZIP_DEFLATED, compresslevel=4) as packed:
        for path in sorted(folder.rglob('*')):
            if path.is_file():
                packed.write(path, path.relative_to(folder).as_posix())
    with zipfile.ZipFile(archive) as packed:
        if packed.testzip() is not None:
            raise RuntimeError('Archive integrity check failed')
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    archive.with_suffix('.zip.sha256').write_text(digest + '  ' + archive.name + '\n', encoding='ascii')
    (ROOT / 'backups' / 'handoff-latest.json').write_text(json.dumps({
        'archive': str(archive), 'sha256': digest, 'bytes': archive.stat().st_size,
        'manifest': str(folder / 'manifest.json'),
    }, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'archive': str(archive), 'sha256': digest, 'MiB': round(archive.stat().st_size / 1048576, 2), 'workspace_files': copied}))


if __name__ == '__main__':
    main()
