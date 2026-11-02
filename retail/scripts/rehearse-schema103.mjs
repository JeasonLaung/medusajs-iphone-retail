// Windows/macOS/Linux: rehearse the supplied 102 backup on an owned, socket-only
// MySQL container. No application, private runtime configuration or real provider.
import assert from 'node:assert/strict'
import { createHash, randomBytes } from 'node:crypto'
import { createReadStream, writeFileSync } from 'node:fs'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawn, spawnSync } from 'node:child_process'
import { pipeline } from 'node:stream/promises'
import { createGunzip } from 'node:zlib'

const root = fileURLToPath(new URL('../../', import.meta.url))
const dump = process.argv[2]
assert(dump && process.argv.length === 3, 'Usage: node retail/scripts/rehearse-schema103.mjs /path/mysql-schema102.sql.gz')
const authority = JSON.parse(await readFile(path.join(root, 'docs/migration/database-handoff-20261102.json')))
const hash = data => createHash('sha256').update(data).digest('hex')
assert.equal(hash(await readFile(dump)), authority.dump_sha256, 'Backup checksum differs from handoff')
const owner = randomBytes(6).toString('hex')
const name = `eurorefurb-schema103-${owner}`
const base = path.join(root, 'logs/gin-migration', name)
await mkdir(base, { recursive: true })
const database = authority.database
const quote = name => '`' + name.replaceAll('`', '``') + '`'
const literal = value => "'" + value.replaceAll("'", "''") + "'"
const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
let container, volume, failure
const report = { owner, backup_sha256: authority.dump_sha256, source_migrations: 103,
  no_application_started: true, real_provider_called: false, private_runtime_required: false }

function run(command, args, { input, env, timeout = 120000 } = {}) {
  const result = spawnSync(command, args, { cwd: root, input, env: env || process.env,
    encoding: 'utf8', windowsHide: true, timeout, maxBuffer: 256 * 1024 * 1024 })
  if (result.error || result.status !== 0) {
    // SQL results and tool output may include business data. Keep all failures local.
    const file = path.join(base, `failure-${Date.now()}.log`)
    writeFileSync(file, String(result.error || '') + '\n' + (result.stderr || ''), { mode: 0o600 })
    throw new Error(`${command} failed; inspect private log in ${base}`)
  }
  return result.stdout.trimEnd()
}
function inspect() {
  const info = JSON.parse(run('docker', ['inspect', container]))[0]
  assert.equal(info.Config.Labels['eurorefurb.schema-rehearsal'], owner)
  assert.equal(info.HostConfig.NetworkMode, 'none')
  assert.equal(info.HostConfig.PublishAllPorts, false)
  assert.equal(Object.keys(info.HostConfig.PortBindings || {}).length, 0)
  assert.equal(info.Mounts.length, 1)
  assert.equal(info.Mounts[0].Type, 'volume')
  assert.equal(info.Mounts[0].Destination, '/var/lib/mysql')
  // The official image creates a new anonymous data volume. Never accept a
  // supplied mount, named/shared volume or host directory.
  assert.match(info.Mounts[0].Name, /^[a-f0-9]{64}$/)
  if (volume) assert.equal(info.Mounts[0].Name, volume)
  else volume = info.Mounts[0].Name
  assert.equal((info.HostConfig.Binds || []).length, 0)
  assert.equal((info.HostConfig.Mounts || []).length, 0)
  return info
}
function sql(query) {
  return run('docker', ['exec', '-i', container, 'mysql', '--no-defaults', '-uroot',
    '--batch', '--raw', '--skip-column-names', '--default-character-set=utf8mb4'], { input: "SET time_zone='+00:00';\n" + query })
}
const migrationDir = path.join(root, 'retail/backend/migrations')
async function migrationHashes() {
  const files = (await readdir(migrationDir)).filter(n => /^\d{4}_.*\.sql$/.test(n)).sort()
  return Object.fromEntries(await Promise.all(files.map(async name => [name, hash(await readFile(path.join(migrationDir, name)))])))
}
function facts(columns) {
  // Compare every original column of every business row. HEX preserves binary,
  // exact decimal and timestamp representations; JSON distinguishes NULL/empty.
  const tables = Object.keys(columns).sort().filter(t => !['schema_migrations', 'schema_migration_steps'].includes(t))
  const query = tables.map(t => `SELECT ${literal(t)},JSON_ARRAY(${columns[t].map(c => `IF(${quote(c)} IS NULL,NULL,HEX(CAST(${quote(c)} AS BINARY)))`).join(',')}) FROM ${quote(database)}.${quote(t)};`).join('\n')
  const rows = Object.fromEntries(tables.map(t => [t, []]))
  for (const line of sql(query).split('\n').filter(Boolean)) {
    const tab = line.indexOf('\t')
    assert(tab > 0 && rows[line.slice(0, tab)], 'Unexpected fingerprint output')
    rows[line.slice(0, tab)].push(line.slice(tab + 1))
  }
  return Object.fromEntries(tables.map(t => [t, { count: rows[t].length, sha256: hash(JSON.stringify(rows[t].sort())) }]))
}
function metadata() {
  const cols = sql(`SELECT table_name,column_name FROM information_schema.columns WHERE table_schema=${literal(database)} AND table_name IN (SELECT table_name FROM information_schema.tables WHERE table_schema=${literal(database)} AND table_type='BASE TABLE') ORDER BY table_name,ordinal_position;`)
  const columns = {}
  for (const line of cols.split('\n')) {
    const [table, column] = line.split('\t'); (columns[table] ??= []).push(column)
  }
  return columns
}
function ddl() {
  return run('docker', ['exec', container, 'mysqldump', '--no-defaults', '-uroot', '--no-data', '--skip-comments',
    '--set-gtid-purged=OFF', '--column-statistics=0', '--routines', '--events', database])
}
try {
  console.log('Building explicit migration command; application services remain untouched.')
  const sourceHashes = await migrationHashes()
  assert.equal(Object.keys(sourceHashes).length, 103)
  for (const file of Object.keys(sourceHashes)) {
    assert(!(await readFile(path.join(migrationDir, file), 'utf8')).includes('\r\n'),
      `${file} has CRLF line endings; restore LF bytes as specified by .gitattributes before building`)
  }
  const binary = path.join(base, 'migrate-linux')
  run('go', ['-C', path.join(root, 'retail/backend'), 'build', '-o', binary, './cmd/migrate'],
    { env: { ...process.env, GOOS: 'linux', GOARCH: 'amd64', CGO_ENABLED: '0' }, timeout: 300000 })
  container = run('docker', ['run', '-d', '--name', name, '--label', `eurorefurb.schema-rehearsal=${owner}`,
    '--network', 'none', '-e', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes', 'mysql:9.6',
    '--innodb-native-foreign-keys=ON', '--skip-networking', '--mysqlx=OFF', '--event-scheduler=OFF', '--skip-log-bin',
    '--innodb-buffer-pool-size=128M', '--innodb-redo-log-capacity=64M'])
  inspect()
  let ready = false
  for (let i = 0; i < 180; i++) {
    const startup = run('docker', ['logs', container])
    if (!startup.includes('MySQL init process done. Ready for start up.')) { await pause(1000); continue }
    const p = spawnSync('docker', ['exec', container, 'mysql', '--no-defaults', '-uroot', '-Nse', 'SELECT 1'], { windowsHide: true, timeout: 5000 })
    if (p.status === 0) { ready = true; break }
    await pause(1000)
  }
  assert(ready, 'Owned MySQL readiness timed out')
  assert.equal(sql('SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name NOT IN (\'mysql\',\'sys\',\'performance_schema\',\'information_schema\');'), '0')
  report.mysql_runtime = sql('SELECT VERSION(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking,@@event_scheduler;').split('\t')
  assert.deepEqual(report.mysql_runtime.slice(1), ['1', '1', '1', 'OFF'])
  console.log('Restoring verified 102 backup into empty isolated MySQL.')
  const importing = spawn('docker', ['exec', '-i', container, 'mysql', '--no-defaults', '-uroot', '--default-character-set=utf8mb4'], { windowsHide: true, stdio: ['pipe', 'ignore', 'pipe'] })
  const finished = new Promise((resolve, reject) => {
    let error = ''
    importing.stderr.on('data', data => { error += data })
    importing.once('error', reject)
    importing.once('close', async code => {
      if (code === 0) resolve()
      else {
        await writeFile(path.join(base, 'import-error.log'), error, { mode: 0o600 })
        reject(new Error('Backup import failed; inspect private import-error.log'))
      }
    })
  })
  await Promise.all([pipeline(createReadStream(dump), createGunzip(), importing.stdin), finished])
  assert.equal(sql(`SELECT COUNT(*) FROM ${quote(database)}.schema_migrations;`), '102')
  const columns = metadata()
  assert.equal(Object.keys(columns).length, authority.base_tables)
  assert(!columns.cart_checkout_attempts.includes('effective_cart_version'))
  const before = facts(columns), ddlBefore = ddl()
  const ledgerBefore = sql(`SELECT version,checksum FROM ${quote(database)}.schema_migrations ORDER BY version;`)
  for (const line of ledgerBefore.split('\n')) {
    const [file, checksum] = line.split('\t')
    assert.equal(sourceHashes[file], checksum, `Migration bytes differ from original database ledger: ${file}`)
  }
  await writeFile(path.join(base, 'schema102.sql'), ddlBefore, { mode: 0o600 })
  run('docker', ['cp', binary, `${container}:/tmp/migrate`])
  run('docker', ['exec', container, 'chmod', '700', '/tmp/migrate'])
  const migrate = () => run('docker', ['exec', '-w', '/tmp', '-e', `MYSQL_DSN=root@unix(/var/run/mysqld/mysqld.sock)/${database}?parseTime=true&loc=UTC`, container, '/tmp/migrate'])
  console.log('Applying 0103 and checking preserved fields, rows, constraints and repeat execution.')
  assert.equal(migrate(), 'Applied 1, skipped 102 migrations')
  assert.equal(sql(`SELECT COUNT(*) FROM ${quote(database)}.schema_migrations;`), '103')
  const ledgerAfter = sql(`SELECT version,checksum FROM ${quote(database)}.schema_migrations WHERE version NOT LIKE '0103_%' ORDER BY version;`)
  assert.equal(ledgerAfter, ledgerBefore)
  const afterColumns = metadata()
  assert.deepEqual(afterColumns, { ...columns, cart_checkout_attempts: [...columns.cart_checkout_attempts, 'effective_cart_version'] })
  assert.equal(sql(`SELECT column_type,is_nullable FROM information_schema.columns WHERE table_schema=${literal(database)} AND table_name='cart_checkout_attempts' AND column_name='effective_cart_version';`), 'bigint unsigned\tYES')
  assert.equal(sql(`SELECT COUNT(*) FROM ${quote(database)}.cart_checkout_attempts WHERE effective_cart_version IS NOT NULL;`), '0')
  const check = sql(`SELECT cc.check_clause FROM information_schema.check_constraints cc JOIN information_schema.table_constraints tc ON tc.constraint_schema=cc.constraint_schema AND tc.constraint_name=cc.constraint_name WHERE tc.table_schema=${literal(database)} AND tc.table_name='cart_checkout_attempts' AND tc.constraint_name='ck_checkout_effective_version' AND tc.enforced='YES';`)
  assert(check.includes('effective_cart_version') && check.includes('cart_version'))
  assert.deepEqual(facts(columns), before)
  const ddlAfter = ddl()
  // Apart from the requested column/CHECK, every table, view, trigger, routine
  // and event must retain its original complete mysqldump definition.
  const normalize = text => text.replace(/^.*`effective_cart_version`.*\n/gm, '').replace(/,\n\) ENGINE/g, '\n) ENGINE')
  assert.equal(normalize(ddlAfter), normalize(ddlBefore), 'Unexpected schema object changes')
  assert.equal(migrate(), 'Applied 0, skipped 103 migrations')
  assert.equal(ddl(), ddlAfter)
  assert.deepEqual(facts(columns), before)
  assert.deepEqual(await migrationHashes(), sourceHashes)
  assert.equal(hash(await readFile(dump)), authority.dump_sha256)
  Object.assign(report, { passed: true, applied: 1, skipped: 102, second_applied: 0, second_skipped: 103,
    original_base_tables: Object.keys(columns).length, business_tables_verified: Object.keys(before).length,
    business_rows_verified: Object.values(before).reduce((n, t) => n + t.count, 0),
    business_facts_sha256: hash(JSON.stringify(before)), original_rows_columns_preserved: true,
    only_expected_ddl_changed: true, existing_ledger_checksums_preserved: true,
    original_backup_unchanged: true, source_migration_sha256: sourceHashes,
    migration_binary_sha256: hash(await readFile(binary)) })
} catch (error) {
  failure = error
  report.passed = false
  console.error(error.message)
} finally {
  if (container) {
    inspect()
    run('docker', ['rm', '-f', '-v', container])
    const remaining = run('docker', ['ps', '-aq', '--filter', `label=eurorefurb.schema-rehearsal=${owner}`])
    assert.equal(remaining, '')
    assert(!run('docker', ['volume', 'ls', '-q']).split('\n').includes(volume), 'Owned data volume cleanup incomplete')
  }
  report.owned_container_removed = true
  report.owned_data_volume_removed = true
  report.finished_utc = new Date().toISOString()
  await writeFile(path.join(base, 'report.json'), JSON.stringify(report, null, 2) + '\n', { mode: 0o600 })
  console.log(`Report: ${path.join(base, 'report.json')}`)
}
if (failure) process.exitCode = 1
