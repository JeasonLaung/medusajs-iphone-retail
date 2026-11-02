"""Actual source review import on a fresh owned native MySQL target; always clean up."""
import getpass
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
MYSQL = '/opt/homebrew/bin/mysql'
GO = '/opt/homebrew/opt/go/bin/go'
SOURCE = ROOT / 'logs/gin-migration/source-native-20261031-144745.ndjson'
SOURCE_SHA = '3b8c007f7854ab61b46009df87ba8f3a082dacfb3bf03078b0c16dcede367aab'


def main():
    if not sys.stdin.isatty():
        raise RuntimeError('Hidden root setup/cleanup prompt requires a terminal')
    if hashlib.sha256(SOURCE.read_bytes()).hexdigest() != SOURCE_SHA:
        raise RuntimeError('Complete private source changed')
    owner = secrets.token_hex(6)
    database = 'eurorefurb_review_history_' + owner
    user = 'retail_rh_' + owner
    password = secrets.token_urlsafe(32)
    directory = ROOT / 'logs/gin-migration' / ('product-review-history-' + owner)
    directory.mkdir(mode=0o700)
    root_env = os.environ.copy()
    root_env['MYSQL_PWD'] = getpass.getpass('Owned history fixture root password (hidden): ')
    env = {k: v for k, v in os.environ.items() if k not in ['MYSQL_PWD', 'MYSQL_DSN']}
    dsn = f'{user}:{password}@tcp(127.0.0.1:3306)/{database}?charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=True'
    env.update(MYSQL_DSN=dsn, NATIVE_HISTORY_DSN=dsn,
               PRODUCT_REVIEWS_HISTORY_NATIVE='1', NATIVE_HISTORY_SNAPSHOT=str(SOURCE),
               NATIVE_HISTORY_SHA256=SOURCE_SHA, CUSTOMER_KEY_ACTIVE='fixture',
               CUSTOMER_KEYRING_JSON=json.dumps({'fixture': secrets.token_hex(32)}))
    def mysql(sql):
        r = subprocess.run([MYSQL, '--user=root', '--batch', '--skip-column-names'],
                           input=sql, env=root_env, text=True, capture_output=True, timeout=30)
        if r.returncode:
            raise RuntimeError('Owned SQL setup/cleanup failed; credentials suppressed')
        return r.stdout.split()
    def hashes():
        backend = ROOT / 'retail/backend'
        paths = [p for p in backend.rglob('*') if p.is_file() and
                 (p.suffix in ['.go', '.sql', '.json'] or p.name in ['go.mod', 'go.sum'])]
        return {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}
    original = hashes()
    resource = dict(owner=owner, database=database, user=user, source_sha256=SOURCE_SHA,
                    source_sha256_files=original, complete_import_accepted=False)
    (directory / 'resources.json').write_text(json.dumps(resource, indent=2) + '\n')
    def run(name, args):
        with (directory / (name + '.log')).open('w') as output:
            result = subprocess.run([GO, *args], cwd=ROOT / 'retail/backend', env=env,
                                    stdout=output, stderr=subprocess.STDOUT, timeout=300)
        if result.returncode:
            raise RuntimeError('Owned history ' + name + ' failed; inspect bounded log')
    try:
        mysql(f'CREATE DATABASE `{database}` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;\n' +
              '\n'.join(f"CREATE USER '{user}'@'{host}' IDENTIFIED BY '{password}';\nGRANT ALL PRIVILEGES ON `{database}`.* TO '{user}'@'{host}';" for host in ['127.0.0.1', 'localhost']))
        run('migrate', ['run', './cmd/migrate'])
        run('native-empty-target', ['test', './internal/historyimport', '-run', '^TestNativeProductReviewsRollbackReplayAndConflict$', '-count=1', '-v'])
        cli = ['run', './cmd/import-history', '--snapshot', str(SOURCE), '--sha256', SOURCE_SHA,
               '--target-database', database, '--include-product-reviews']
        run('cli-rollback', cli + ['--rollback'])
        run('cli-apply', cli + ['--apply'])
        run('native-committed-target', ['test', './internal/historyimport', '-run', '^TestNativeProductReviewsRollbackReplayAndConflict$', '-count=1', '-v'])
        if original != hashes():
            raise RuntimeError('Source changed during native evidence collection')
        report = dict(source_sha256=SOURCE_SHA, migrations=94, empty_and_committed_replay=True,
                      cli_rollback_and_apply=True, actual_source_reviews=2, goal_complete=False,
                      complete_import_accepted=False, real_payments=False, real_mail=False)
        (directory / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps({'history_review_native': 'passed', 'evidence': str(directory)}))
    finally:
        result = mysql(f'DROP DATABASE IF EXISTS `{database}`;\n' +
                       '\n'.join(f"DROP USER IF EXISTS '{user}'@'{host}';" for host in ['127.0.0.1', 'localhost']) +
                       f"\nSELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}';\nSELECT COUNT(*) FROM mysql.user WHERE user='{user}';")
        if result != ['0', '0']:
            raise RuntimeError('Owned database/user cleanup not proven')
        (directory / 'cleanup.json').write_text(json.dumps(dict(owned_database_absent=True, owned_users_absent=True)) + '\n')


if __name__ == '__main__':
    main()
