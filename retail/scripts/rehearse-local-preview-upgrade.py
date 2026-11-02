"""Back up the local preview and rehearse migration on an owned private server.

Never starts an application/worker or modifies the original database/server.
The retained dump contains private local data: ignored directory and mode 0600.
"""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]
MYSQL = '/opt/homebrew/bin/mysql'
DUMP = '/opt/homebrew/bin/mysqldump'
DATABASE = 'eurorefurb_local_trial'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    if shutil.disk_usage(ROOT).free < 2 * 1024**3:
        raise RuntimeError('2 GiB free required to preserve development services')
    values = [line.split('=', 1)[1].strip().strip('"').strip("'")
              for line in (ROOT / 'retail/backend/.env').read_text().splitlines()
              if line.startswith('MYSQL_DSN=')]
    if len(values) != 1:
        raise RuntimeError('One preview connection required')
    match = re.fullmatch(r'([^:]+):(.*)@tcp\(127\.0\.0\.1:3306\)/eurorefurb_local_trial\?.*', values[0])
    if not match:
        raise RuntimeError('Only the original local preview target is allowed')
    owner = secrets.token_hex(6)
    base = ROOT / 'logs/gin-migration' / ('preview-upgrade-' + owner)
    base.mkdir(mode=0o700)
    socket_dir = Path(tempfile.mkdtemp(prefix='erpreview-' + owner + '-', dir='/private/tmp'))
    socket = socket_dir / 'mysql.sock'
    datadir = base / 'mysql-data'
    datadir.mkdir(mode=0o700)
    report = {'owner': owner, 'main_database_modified': False,
              'main_services_restarted': False, 'application_started_on_clone': False,
              'real_provider_called': False, 'goal_complete': False,
              'started_utc': datetime.datetime.now(datetime.timezone.utc).isoformat()}
    clean_env = {k: os.environ[k] for k in ['PATH', 'HOME', 'TMPDIR'] if k in os.environ}
    main_env = {**clean_env, 'MYSQL_PWD': match[2]}
    main_connection = ['--protocol=TCP', '-h127.0.0.1', '-P3306', '-u' + match[1]]
    private_connection = ['--protocol=SOCKET', '--socket=' + str(socket), '-uroot']
    server = None

    def write(name, value):
        p = base / name
        p.write_text(json.dumps(value, indent=2) + '\n')
        p.chmod(0o600)

    def query(sql, original=False, database=True):
        cmd = [MYSQL, '--no-defaults', *(main_connection if original else private_connection),
               '-N', '--batch', *([DATABASE] if database else [])]
        run = subprocess.run(cmd, input=sql, text=True, capture_output=True,
                             env=main_env if original else clean_env, timeout=60)
        if run.returncode:
            raise RuntimeError('Read-only audit query failed; private output suppressed')
        return [row.split('\t') for row in run.stdout.splitlines()]

    def dump(path, original=False, data_only=False):
        flags = ['--single-transaction', '--skip-lock-tables', '--no-tablespaces',
                 '--set-gtid-purged=OFF', '--hex-blob', '--order-by-primary',
                 '--skip-extended-insert', '--skip-comments', '--default-character-set=utf8mb4']
        flags += ['--no-create-info', '--skip-triggers'] if data_only else ['--routines', '--events', '--triggers']
        with path.open('xb') as out:
            path.chmod(0o600)
            run = subprocess.run([DUMP, '--no-defaults', *(main_connection if original else private_connection),
                                  *flags, DATABASE], stdout=out, stderr=subprocess.PIPE,
                                 env=main_env if original else clean_env, timeout=120)
        if run.returncode or path.stat().st_size == 0:
            raise RuntimeError('Consistent backup failed; private output suppressed')

    def fingerprints(path, tables):
        rows = {table: [] for table in tables}
        for line in path.read_bytes().splitlines():
            found = re.match(rb'INSERT INTO `([a-zA-Z0-9_]+)`(?: \([^)]*\))? VALUES ', line)
            if found:
                table = found[1].decode()
                if table not in rows:
                    raise RuntimeError('Unexpected backup table')
                rows[table].append(line)
        return {table: {'rows': len(lines), 'sha256': hashlib.sha256(b'\n'.join(sorted(lines))).hexdigest()}
                for table, lines in rows.items()}

    def tables(original=False):
        result = [row[0] for row in query("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name;", original)]
        if not result or any(not re.fullmatch(r'[a-zA-Z0-9_]+', name) for name in result):
            raise RuntimeError('Unexpected table inventory')
        return result

    def objects(original=False):
        selects = {
          'views': "SELECT table_name,view_definition,check_option,is_updatable,definer,security_type,character_set_client,collation_connection FROM information_schema.views WHERE table_schema=DATABASE() ORDER BY table_name;",
          'triggers': "SELECT trigger_name,event_manipulation,event_object_table,action_order,action_statement,action_timing,sql_mode,definer,character_set_client,collation_connection,database_collation FROM information_schema.triggers WHERE trigger_schema=DATABASE() ORDER BY trigger_name;",
          'events': "SELECT event_name,event_definition,event_type,execute_at,interval_value,interval_field,sql_mode,starts,ends,status,on_completion,definer,character_set_client,collation_connection,database_collation FROM information_schema.events WHERE event_schema=DATABASE() ORDER BY event_name;",
          'routines': "SELECT routine_name,routine_type,data_type,routine_definition,is_deterministic,sql_data_access,security_type,sql_mode,definer,character_set_client,collation_connection,database_collation FROM information_schema.routines WHERE routine_schema=DATABASE() ORDER BY routine_name,routine_type;"
        }
        return {name:{'count':len(rows),'sha256':hashlib.sha256(json.dumps(rows,ensure_ascii=False).encode()).hexdigest()}
                for name,sql in selects.items() for rows in [query(sql,original)]}

    def check_foreign_keys():
        keys=query("SELECT constraint_name,table_name,column_name,referenced_table_name,referenced_column_name FROM information_schema.key_column_usage WHERE constraint_schema=DATABASE() AND referenced_table_name IS NOT NULL ORDER BY table_name,constraint_name,ordinal_position;")
        grouped={}
        for constraint,table,column,parent,parent_column in keys:
            if any(not re.fullmatch(r'[a-zA-Z0-9_]+',x) for x in [constraint,table,column,parent,parent_column]):
                raise RuntimeError('Unexpected foreign key identifier')
            grouped.setdefault((table,constraint,parent),[]).append((column,parent_column))
        counts={}
        for (table,constraint,parent),columns in grouped.items():
            join=' AND '.join('c.`'+c+'`=p.`'+p+'`' for c,p in columns)
            nonnull=' AND '.join('c.`'+c+'` IS NOT NULL' for c,_ in columns)
            count=int(query('SELECT COUNT(*) FROM `'+table+'` c LEFT JOIN `'+parent+'` p ON '+join+' WHERE '+nonnull+' AND p.`'+columns[0][1]+'` IS NULL;')[0][0])
            counts[table+'.'+constraint]=count
        if any(counts.values()):
            write('foreign-key-orphans.json',counts)
            raise RuntimeError('Existing foreign-key orphan facts require reconciliation')
        return {'constraints_checked':len(counts),'orphan_rows':0}

    def count_rows(names):
        # The private clone has no writers, event scheduler, application or jobs.
        return {name: int(query('SELECT COUNT(*) FROM `' + name + '`;')[0][0]) for name in names}

    try:
        source = ROOT / 'logs/gin-migration/native-sql-local-d87bbfe447dd/retail/backend'
        state = json.loads((ROOT / 'logs/gin-migration/native-sql-isolated-state.json').read_text())
        if Path(state['snapshot']) / 'retail/backend' != source:
            raise RuntimeError('Expected tested frozen source required')
        for name, digest in state['source_sha256'].items():
            if name.startswith('retail/backend/'):
                if sha(Path(state['snapshot']) / name) != state['adapted_test_sha256'].get(name, digest) or sha(ROOT / name) != digest:
                    raise RuntimeError('Frozen/current source changed')
        report['frozen_source_owner'] = state['owner']
        report['source_migrations_sha256'] = {p.name: sha(p) for p in sorted((source / 'migrations').glob('*.sql'))}
        binary = base / 'migrate'
        with (base / 'compile.log').open('wb') as out:
            built = subprocess.run(['/opt/homebrew/opt/go/bin/go', 'build', '-o', str(binary), './cmd/migrate'],
                                   cwd=source, env=clean_env, stdout=out, stderr=subprocess.STDOUT, timeout=120)
        if built.returncode:
            raise RuntimeError('Frozen migration binary compile failed')
        report['migration_binary_sha256'] = sha(binary)
        original_tables = tables(True)
        journal_before = query('SELECT version,checksum FROM schema_migrations ORDER BY version;', True)
        if len(journal_before) != 86:
            raise RuntimeError('Expected original 86 migration prefix')
        original_objects=objects(True)
        backup = base / 'preview-86-private.sql'
        dump(backup, original=True)
        before = fingerprints(backup, original_tables)
        report['backup'] = {'file': backup.name, 'mode': oct(backup.stat().st_mode & 0o777),
                            'bytes': backup.stat().st_size, 'sha256': sha(backup),
                            'tables': len(before), 'rows': sum(x['rows'] for x in before.values()),
                            'consistent_transaction': True, 'private_data_not_logged': True}
        write('backup-fingerprints.json', before)
        print(json.dumps({'owner': owner, 'backup_complete': True, 'tables': len(before)}), flush=True)
        mysqld = str(Path(MYSQL).resolve().with_name('mysqld'))
        init = subprocess.run([mysqld, '--no-defaults', '--initialize-insecure',
                               '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M',
                               '--datadir=' + str(datadir), '--log-error=' + str(base / 'mysql-initialize.log')],
                              env=clean_env, capture_output=True, timeout=90)
        if init.returncode:
            raise RuntimeError('Private MySQL initialization failed')
        with (base / 'mysql-launch.log').open('wb') as out:
            server = subprocess.Popen([mysqld, '--no-defaults', '--datadir=' + str(datadir),
                       '--socket=' + str(socket), '--pid-file=' + str(base / 'mysql.pid'),
                       '--log-error=' + str(base / 'mysql-server.log'), '--skip-networking',
                       '--mysqlx=OFF', '--skip-log-bin', '--event-scheduler=OFF',
                       '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M',
                       '--innodb-native-foreign-keys=ON'], env=clean_env, stdout=out, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            if server.poll() is not None:
                raise RuntimeError('Private MySQL terminated')
            if socket.exists():
                try:
                    runtime = query('SELECT @@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking,@@event_scheduler;', database=False)
                    if runtime == [['1', '1', '1', 'OFF']]:
                        break
                except RuntimeError:
                    pass
            time.sleep(0.2)
        else:
            raise RuntimeError('Private MySQL readiness timed out')
        query('CREATE DATABASE `' + DATABASE + '` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;', database=False)
        with backup.open('rb') as inp:
            restored = subprocess.run([MYSQL, '--no-defaults', *private_connection, DATABASE], stdin=inp,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=clean_env, timeout=180)
        if restored.returncode:
            raise RuntimeError('Private restore failed; private output suppressed')
        restored_dump = base / 'restored-86-private-data.sql'
        dump(restored_dump)
        # Tables with generated columns use an explicit INSERT column list.
        # Compare full DDL and all inserts, not only bare VALUES statements.
        def table_ddl(path):
            definitions={}
            for m in re.finditer(rb'CREATE TABLE `([a-zA-Z0-9_]+)` \(.*?\) ENGINE=[^;]+;',path.read_bytes(),re.S):
                # A collation determines its charset. SHOW CREATE may add the
                # redundant explicit charset after restoring identical SQL.
                canonical=re.sub(rb' CHARACTER SET (\w+) COLLATE (\w+)',
                    lambda x:b' COLLATE '+x[2] if x[2].startswith(x[1]+b'_') else x[0],m[0])
                definitions[m[1].decode()]=hashlib.sha256(canonical).hexdigest()
            return definitions
        if table_ddl(backup) != table_ddl(restored_dump):
            raise RuntimeError('Restored table definitions differ')
        report['restore_table_ddl_sha256_equal'] = True
        restored_facts = fingerprints(restored_dump, tables())
        if restored_facts != before or count_rows(original_tables) != {k: v['rows'] for k, v in before.items()}:
            raise RuntimeError('Restored row counts or data hashes differ')
        if query('SELECT version,checksum FROM schema_migrations ORDER BY version;') != journal_before:
            raise RuntimeError('Restored journal differs')
        if objects()!=original_objects:
            raise RuntimeError('Restored view/trigger/routine/event definitions differ')
        report['restored_objects']=original_objects
        report['foreign_keys_before_migration']=check_foreign_keys()
        report['restore_86_exact_rows_and_data_sha256'] = True
        print(json.dumps({'owner': owner, 'restore_verified': True}), flush=True)
        migration_env = {**clean_env, 'MYSQL_DSN': 'root@unix(' + str(socket) + ')/' + DATABASE + '?parseTime=true&multiStatements=false&charset=utf8mb4'}
        for i, expected in enumerate(['Applied 13, skipped 86 migrations', 'Applied 0, skipped 99 migrations']):
            run = subprocess.run([str(binary)], cwd=base, env=migration_env, text=True, capture_output=True, timeout=600)
            # The migration runner emits generic errors only. Never print SQL.
            (base / ('migration-' + str(i + 1) + '.log')).write_text(run.stdout + run.stderr)
            if run.returncode or run.stdout.strip() != expected:
                report['migration_attempt_failed'] = i + 1
                raise RuntimeError('Restored migration failed; inspect generic migration log')
        post = base / 'migrated-99-private-data.sql'
        dump(post, data_only=True)
        after = fingerprints(post, tables())
        changed = [name for name in before if before[name] != after[name]]
        expected_changes = {'schema_migrations', 'schema_migration_steps', 'order_runtime_schema',
                            'order_refund_runtime', 'authorization_actions', 'authorization_runtime_schema',
                            'catalog_command_capabilities'}
        if set(changed) != expected_changes:
            raise RuntimeError('Unexpected existing-table data changes')
        if count_rows(tables()) != {k: v['rows'] for k, v in after.items()}:
            raise RuntimeError('Post-migration row counts differ from dump')
        final_journal = query('SELECT version,checksum FROM schema_migrations ORDER BY version;')
        if dict(final_journal) != report['source_migrations_sha256']:
            raise RuntimeError('Final 99 migration checksums differ')
        gates = query("SELECT command,enabled FROM catalog_command_capabilities WHERE command IN ('product.create','product.images','product.options','variant.edit','variant.inventory','product.option-definitions','store.buy-policies') ORDER BY command;")
        if len(gates) != 7 or any(row[1] != '0' for row in gates):
            raise RuntimeError('New catalog gate unexpectedly enabled')
        if query('SELECT enabled FROM customer_cart_runtime;') != [['0']] or query('SELECT enabled FROM order_payment_runtime;') != [['0']] or query('SELECT reads_enabled,writes_enabled FROM product_review_runtime;') != [['0', '0']]:
            raise RuntimeError('New commerce gate unexpectedly enabled')
        report['foreign_keys_after_migration']=check_foreign_keys()
        write('migrated-fingerprints.json', after)
        report.update(migrations_applied=13, migrations_skipped=86, idempotent_skipped=99,
                      existing_tables_unchanged=len(before) - len(changed), expected_changed_tables=sorted(changed),
                      new_tables=sorted(set(after) - set(before)), new_gates_default_closed=True,
                      native_foreign_keys=True, networking_disabled=True, event_scheduler_disabled=True,
                      rehearsal_passed=True)
        print(json.dumps({'owner': owner, 'migration_rehearsal_passed': True}), flush=True)
    finally:
        if server is not None and server.poll() is None:
            try:
                subprocess.run([str(Path(MYSQL).resolve().with_name('mysqladmin')), '--no-defaults',
                                *private_connection, 'shutdown'], env=clean_env, capture_output=True, timeout=15)
            except subprocess.TimeoutExpired:
                pass
            try:
                server.wait(timeout=30)
            except subprocess.TimeoutExpired:
                server.terminate()
                server.wait(timeout=15)
        terminal = server is None or server.poll() is not None
        if terminal:
            shutil.rmtree(datadir)
            if socket_dir.exists() and not list(socket_dir.iterdir()):
                socket_dir.rmdir()
        report['cleanup'] = {'server_terminal': terminal, 'data_removed': not datadir.exists(),
                             'socket_removed': not socket.exists(), 'private_backup_retained': (base / 'preview-86-private.sql').exists()}
        report['finished_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        write('report.json', report)
        print(json.dumps({'owner': owner, 'cleanup': report['cleanup']}), flush=True)


if __name__ == '__main__':
    main()
