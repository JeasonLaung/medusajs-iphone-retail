// Real ignored snapshot -> selected legacy Node decryptors -> private stdin ->
// Go row conversion and ephemeral Ring. No private payload/key files or DB/HTTP.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const { spawnSync } = require('node:child_process')
const { loadDomain } = require('./audit-protected-config.cjs')
const root = path.resolve(__dirname, '../..')
const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex')
const domains = {
  mail: ['MAIL_ENCRYPTION_KEY', 'decryptSecret', 'src/modules/mail/security.ts'],
  seo: ['SEO_ENCRYPTION_KEY', 'decrypt', 'src/lib/seo/core.ts'],
  webhook: ['WEBHOOK_ENCRYPTION_KEY', 'decrypt', 'src/modules/webhook-notifications/security.ts'],
  mollie: ['MOLLIE_ENCRYPTION_KEY', 'decryptMollieSecret', 'src/modules/mollie-settings/security.ts'],
}
const selected = {
  mail_account: ['mail', ['api_key', 'webhook_secret']], seo_provider: ['seo', ['api_key']],
  seo_setting: ['seo', ['value']], webhook_endpoint: ['webhook', ['url', 'secret', 'token']], mollie_config: ['mollie', ['api_key']],
}
let stage = 'input-validation'
function main() {
  const options = {}
  for (let i = 2; i < process.argv.length; i++) {
    const flag = process.argv[i]
    assert(['--bundle', '--env'].includes(flag)); options[flag.slice(2)] = process.argv[++i]
  }
  assert(options.bundle && options.env)
  for (const file of [options.bundle, options.env]) {
    assert.equal(spawnSync('git', ['check-ignore', '--quiet', path.resolve(file)], { cwd: root }).status, 0)
  }
  const bundle = fs.readFileSync(options.bundle)
  const bundleHash = digest(bundle)
  assert.equal(bundleHash, 'aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132')
  const tables = JSON.parse(bundle).tables
  const dotenv = 'medusa-store/node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/lib/main.js'
  const parsed = require(path.join(root, dotenv)).parse(fs.readFileSync(options.env))
  const environment = {}, keys = {}
  for (const name of ['COOKIE_SECRET', ...Object.values(domains).map(d => d[0])]) if (parsed[name] !== undefined) environment[name] = parsed[name]
  for (const [domain, [name]] of Object.entries(domains)) {
    const value = environment[name] || environment.COOKIE_SECRET
    if (value !== undefined && value !== '' && value !== 'supersecret') keys[domain] = value
  }
  const rows = [], tableCounts = {}
  const plans = JSON.parse(fs.readFileSync(path.join(root, 'retail/backend/internal/rowimport/field_plans.json'), 'utf8')).tables
  for (const [table, [domain, columns]] of Object.entries(selected)) {
    assert(Array.isArray(tables[table]))
    tableCounts[table] = tables[table].length
    const api = loadDomain(domain, environment)
    const plan = plans.find(p => p.source === table)
    for (const row of tables[table]) {
      assert.deepEqual(Object.keys(row).sort(), plan.fields.map(f => f.source).sort())
      // These selected configuration rows have no numeric business fields.
      // Refuse extending this JSON.parse bridge into amount-bearing tables.
      assert(plan.fields.every(f => !/decimal|int32|int64|float|minor_unit/.test(f.transform)))
      for (const f of plan.fields) if (['exact_json_text', 'protected_exact_json', 'text_array_json'].includes(f.transform)) assert.notEqual(row[f.source], null)
      const expected = {}
      for (const column of columns) {
        assert.equal(typeof row[column], 'string')
        expected[column] = table === 'seo_setting' && row.is_secret === false ? row[column] : api[domains[domain][1]](row[column])
      }
      rows.push({ table, cells: row, expected })
    }
  }
  const executable = path.join(root, 'logs/gin-migration/config-audit-' + crypto.randomUUID() + (process.platform === 'win32' ? '.exe' : ''))
  assert.equal(spawnSync('git', ['check-ignore', '--quiet', executable], { cwd: root }).status, 0)
  stage = 'compile-private-pipe-runner'
  const compiled = spawnSync('go', ['test', '-c', '-o', executable, './internal/rowimport'],
    { cwd: path.join(root, 'retail/backend'), encoding: 'utf8', timeout: 120000 })
  if (compiled.status !== 0) {
    if (fs.existsSync(executable)) fs.unlinkSync(executable)
    throw new Error('Private runner unavailable')
  }
  let payload
  let result
  try {
    payload = Buffer.from(JSON.stringify({ keys, rows }))
    stage = 'go-private-pipe'
    result = spawnSync(executable, ['-test.v', '-test.run', '^TestActualMerchantConfigOfflinePipe$'],
      { cwd: path.join(root, 'retail/backend'), env: { ...process.env, ROWIMPORT_CONFIG_PIPE: '1' }, input: payload,
        encoding: 'utf8', timeout: 120000, maxBuffer: 1024 * 1024 })
  } finally { if (payload) payload.fill(0); payload = null; bundle.fill(0); fs.unlinkSync(executable) }
  // Never echo subprocess stdout/stderr on failure; only the fixed aggregate marker.
  if (result.status !== 0) {
    const reasons = ['private audit input unavailable or too large', 'invalid private audit input', 'selected legacy configuration invalid',
      'actual source row conversion failed', 'v1 cannot prove JSON SQL NULL provenance', 'private audit source columns differ',
      'actual source-to-target bytes differ']
    const reason = reasons.find(text => result.stdout?.includes(text))
    if (reason) stage += ':' + reason
  }
  assert.equal(result.status, 0)
  stage = 'aggregate-report'
  const markers = result.stdout.split(/\r?\n/).filter(line => line.includes('CONFIG_PIPE_AUDIT '))
  assert.equal(markers.length, 1)
  const counts = JSON.parse(markers[0].split('CONFIG_PIPE_AUDIT ')[1])
  assert.equal(Object.values(counts.rows_by_table).reduce((a, b) => a + b, 0), rows.length)
  const sources = [dotenv, ...Object.values(domains).map(d => 'medusa-store/apps/backend/' + d[2]),
    'retail/backend/internal/rowimport/config_secret.go', 'retail/backend/internal/rowimport/config_secret_offline_test.go',
    'retail/backend/internal/rowimport/convert.go', 'retail/scripts/audit-protected-config.cjs',
    'retail/backend/internal/legacysecret/decrypt.go', 'retail/backend/internal/protected/envelope.go',
    'retail/backend/internal/rowimport/field_plans.json', 'retail/scripts/audit-go-merchant-config.cjs']
  const report = { format_version: 1, scope: 'Actual pinned merchant string conversion with independent Node plaintext expectation; ephemeral target envelopes only, no SQL, provider or full snapshot acceptance',
    bundle_sha256: bundleHash, tables_in_scope: tableCounts, counts,
    implementation_sha256: Object.fromEntries(sources.map(file => [file, digest(fs.readFileSync(path.join(root, file)).toString('utf8').replaceAll('\r\n', '\n'))])),
    private_stdin_only: true, new_key_or_envelopes_persisted: false, source_database_connection: false,
    selected_env_proves_live_runtime_keys: false, empty_tables_not_accepted_as_real_samples: true }
  fs.writeFileSync(path.join(root, 'docs/migration/go-merchant-config-report.json'), JSON.stringify(report, null, 2) + '\n')
  console.log(JSON.stringify(counts))
}
try { main() } catch { console.error('Go merchant configuration audit failed at ' + stage + '; private inputs and error details suppressed.'); process.exitCode = 1 }
