"""Read-only post-cutover verification of the actual persistent local preview.

Private row dumps stay in the ignored, protected preview directory. Reports
contain counts and digests only; no customer rows, credentials or DSNs.
"""
import datetime
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess

HERE = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


m = load('preview_mysql', HERE / 'preview-mysql.py')
api = load('preview_api', HERE / 'preview-api.py')


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def dump(name, connection, env):
    path = m.BASE / name
    with path.open('wb') as output:
        path.chmod(0o600)
        run = subprocess.run(['/opt/homebrew/bin/mysqldump', '--no-defaults', *connection,
            '--single-transaction', '--skip-lock-tables', '--no-tablespaces',
            '--set-gtid-purged=OFF', '--hex-blob', '--order-by-primary',
            '--skip-extended-insert', '--skip-comments', '--default-character-set=utf8mb4',
            '--no-create-info', '--skip-triggers', m.DATABASE],
            stdout=output, stderr=subprocess.PIPE, env=env, timeout=120)
    require(run.returncode == 0 and path.stat().st_size > 0, 'Private dump failed; output suppressed')
    return path


def fingerprints(path, names):
    rows = {name: [] for name in names}
    for line in path.read_bytes().splitlines():
        found = re.match(rb'INSERT INTO `([a-zA-Z0-9_]+)`(?: \([^)]*\))? VALUES ', line)
        if found:
            name = found[1].decode()
            require(name in rows, 'Unexpected table in private dump')
            rows[name].append(line)
    return {name: {'rows': len(lines), 'sha256': hashlib.sha256(b'\n'.join(sorted(lines))).hexdigest()}
            for name, lines in rows.items()}


def main():
    service = api.status()
    require(service['running'], 'Preview API must be running')
    cutover = json.loads((m.BASE / 'cutover.json').read_text())
    evidence = m.ROOT / 'logs/gin-migration' / ('preview-upgrade-' + cutover['fresh_backup_owner'])
    rehearsal = json.loads((evidence / 'report.json').read_text())
    before = json.loads((evidence / 'backup-fingerprints.json').read_text())
    expected = json.loads((evidence / 'migrated-fingerprints.json').read_text())
    current_rehearsal = rehearsal
    if cutover['schema_migrations'] in (100, 101, 102):
        upgraded = m.ROOT / 'logs/gin-migration' / ('preview-' + str(cutover['schema_migrations']) + '-upgrade-' + cutover['schema_upgrade']['owner'])
        current_rehearsal = json.loads((upgraded / 'report.json').read_text())
        expected = json.loads((upgraded / 'migrated-fingerprints.json').read_text())
    names = [r[0] for r in m.query("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name;", database=True)]
    require(set(names) == set(expected), 'Persistent table inventory differs')
    c = m.config()
    path = dump('post-cutover-private-data.sql', ['--protocol=SOCKET', '--socket=' + str(m.SOCKET), '-uroot'],
                {**m.ENV, 'MYSQL_PWD': c['root_password']})
    actual = fingerprints(path, names)
    # Migration execution times legitimately differ between the two restores.
    business = set(names) - {'schema_migrations', 'schema_migration_steps'}
    require(all(actual[t] == expected[t] for t in business), 'Persistent business facts differ from fresh rehearsal')
    require(dict(m.query('SELECT version,checksum FROM schema_migrations ORDER BY version;', database=True)) == current_rehearsal['source_migrations_sha256'], 'Migration checksums differ')
    require(m.query("SELECT COUNT(*) FROM schema_migration_steps WHERE status<>'applied';", database=True) == [['0']], 'Incomplete migration steps')
    unchanged = [t for t in before if before[t] == actual[t]]
    require(len(unchanged) == rehearsal['existing_tables_unchanged'], 'Original business table fingerprints differ')
    # Compare live row counts too, so a missing INSERT parser case cannot pass.
    require(all(re.fullmatch(r'[a-zA-Z0-9_]+', t) for t in names), 'Invalid table identifier')
    counts = m.query(' UNION ALL '.join("SELECT '" + t + "',COUNT(*) FROM `" + t + '`' for t in names) + ';', database=True)
    require({t: int(n) for t, n in counts} == {t: v['rows'] for t, v in actual.items()}, 'Live counts differ from dump')
    keys = m.query("SELECT constraint_name,table_name,column_name,referenced_table_name,referenced_column_name FROM information_schema.key_column_usage WHERE constraint_schema=DATABASE() AND referenced_table_name IS NOT NULL ORDER BY table_name,constraint_name,ordinal_position;", database=True)
    groups = {}
    for constraint, table, column, parent, parent_column in keys:
        require(all(re.fullmatch(r'[a-zA-Z0-9_]+', s) for s in [constraint, table, column, parent, parent_column]), 'Invalid FK identifier')
        groups.setdefault((table, constraint, parent), []).append((column, parent_column))
    for (table, constraint, parent), columns in groups.items():
        join = ' AND '.join('c.`' + col + '`=p.`' + pcol + '`' for col, pcol in columns)
        nonnull = ' AND '.join('c.`' + col + '` IS NOT NULL' for col, _ in columns)
        require(m.query('SELECT COUNT(*) FROM `' + table + '` c LEFT JOIN `' + parent + '` p ON ' + join + ' WHERE ' + nonnull + ' AND p.`' + columns[0][1] + '` IS NULL;', database=True) == [['0']], 'Foreign key orphan found')
    gates = m.query("SELECT command,enabled FROM catalog_command_capabilities WHERE command IN ('product.create','product.images','product.options','variant.edit','variant.inventory','product.option-definitions','store.buy-policies') ORDER BY command;", database=True)
    require(len(gates) == 7 and all(v == '0' for _, v in gates), 'New catalog gate differs')
    for sql, expected_gate in [('SELECT enabled FROM customer_cart_runtime;', [['0']]), ('SELECT enabled FROM order_payment_runtime;', [['0']]), ('SELECT reads_enabled,writes_enabled FROM product_review_runtime;', [['0', '0']])]:
        require(m.query(sql, database=True) == expected_gate, 'New commerce gate differs')
    if cutover['schema_migrations'] in (101, 102):
        require(m.query('SELECT id,version,enabled FROM seo_command_state;', database=True) == [['1','1','0']], 'SEO command gate differs')
        require(m.query('SELECT COUNT(*) FROM seo_command_receipts;', database=True) == [['0']], 'Unexpected preview SEO receipts')
    if cutover['schema_migrations'] == 102:
        require(m.query('SELECT id,version,enabled FROM cart_context_runtime;', database=True) == [['1','1','0']], 'Cart context command gate differs')
        require(m.query('SELECT COUNT(*) FROM cart_context_operations;', database=True) == [['0']], 'Unexpected preview cart context receipts')
    original = api.api.parse_env((m.BASE / 'original-backend.env').read_text())
    source = re.fullmatch(r'([^:]+):(.*)@tcp\(127\.0\.0\.1:3306\)/eurorefurb_local_trial\?.*', original['MYSQL_DSN'])
    require(source is not None, 'Unexpected original source')
    old_path = dump('retained-86-private-data.sql', ['--protocol=TCP', '-h127.0.0.1', '-P3306', '-u' + source[1]],
                    {**m.ENV, 'MYSQL_PWD': source[2]})
    require(fingerprints(old_path, before) == before, 'Retained86 facts differ from frozen backup')
    m.save(m.BASE / 'post-cutover-fingerprints.json', actual)
    report = {'checked_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'fresh_backup_owner': cutover['fresh_backup_owner'], 'service': service,
              'persistent_tables': len(names), 'business_tables_equal_to_rehearsal': len(business),
              'migration_journal_execution_times_excluded': True, 'migration_checksums_match': True,
              'original_tables_unchanged': len(unchanged), 'live_counts_match_dump': True,
              'foreign_keys_checked': len(groups), 'orphan_rows': 0,
              'new_gates_default_closed': True, 'retained_shared86_all_table_fingerprints_match': True,
              'shared_mysql_modified': False, 'real_provider_called': False,
              'post_cutover_data_verified': True, 'goal_complete': False}
    m.save(m.BASE / 'post-cutover-verification.json', report)
    print(json.dumps(report))


if __name__ == '__main__':
    main()
