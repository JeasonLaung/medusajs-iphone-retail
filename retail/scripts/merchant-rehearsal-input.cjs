// Private child-process stdout protocol. Invoke only through the owning Python
// rehearsal, which pipes the payload into Go stdin and never echoes it.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const { spawnSync } = require('node:child_process')
const { loadDomain } = require('./audit-protected-config.cjs')
const root = path.resolve(__dirname, '../..')
const domains = {
  mail: ['MAIL_ENCRYPTION_KEY', 'decryptSecret'], seo: ['SEO_ENCRYPTION_KEY', 'decrypt'],
  webhook: ['WEBHOOK_ENCRYPTION_KEY', 'decrypt'], mollie: ['MOLLIE_ENCRYPTION_KEY', 'decryptMollieSecret'],
}
const selected = {
  mail_account: ['mail', ['api_key', 'webhook_secret']], seo_provider: ['seo', ['api_key']],
  seo_setting: ['seo', ['value']], webhook_endpoint: ['webhook', ['url', 'secret', 'token']],
  mollie_config: ['mollie', ['api_key']],
  paypal_connection: [null, ['auth_code', 'seller_client_secret', 'app_access_token', 'metadata']],
  paypal_settings: [null, ['data']],
}
function main() {
  assert.equal(process.argv.length, 4)
  const snapshot = path.resolve(process.argv[2]), envFile = path.resolve(process.argv[3])
  for (const file of [snapshot, envFile]) assert.equal(spawnSync('git', ['check-ignore', '--quiet', file], { cwd: root }).status, 0)
  const bytes = fs.readFileSync(snapshot)
  assert.equal(crypto.createHash('sha256').update(bytes).digest('hex'), 'b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985')
  const parsed = require(path.join(root, 'medusa-store/node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/lib/main.js')).parse(fs.readFileSync(envFile))
  const environment = {}, keys = {}
  for (const name of ['COOKIE_SECRET', ...Object.values(domains).map(d => d[0])]) if (parsed[name] !== undefined) environment[name] = parsed[name]
  for (const [domain, [name]] of Object.entries(domains)) {
    const value = environment[name] || environment.COOKIE_SECRET
    if (value !== undefined && value !== '' && value !== 'supersecret') keys[domain] = value
  }
  const apis = Object.fromEntries(Object.keys(domains).map(domain => [domain, loadDomain(domain, environment)]))
  const expected = []
  for (const line of bytes.toString('utf8').split('\n')) {
    if (!line) continue
    const record = JSON.parse(line)
    if (!record.cells || !Object.hasOwn(selected, record.table)) continue
    const [domain, columns] = selected[record.table], cells = record.cells
    const id = JSON.parse(cells.id.value_json), fields = {}
    assert.equal(typeof id, 'string')
    for (const column of columns) {
      const cell = cells[column]
      if (cell.sql_null) { fields[column] = null; continue }
      assert.equal(typeof cell.value_json, 'string')
      if (!domain && ['metadata', 'data'].includes(column)) {
        // The fixed source contains reviewed baseline JSON only. Go still
        // independently validates it; preserve its lexical bytes here.
        fields[column] = cell.value_json
      } else {
        const value = JSON.parse(cell.value_json)
        assert.equal(typeof value, 'string')
        const plainSEO = record.table === 'seo_setting' && JSON.parse(cells.is_secret.value_json) === false
        if (domain && !plainSEO) fields[column] = apis[domain][domains[domain][1]](value)
        else {
          // No real encrypted PayPal string exists in this pinned snapshot.
          // Refuse silently treating another encrypted format as plaintext.
          assert(!value.startsWith('enc:') && !value.startsWith('ers1:'))
          fields[column] = value
        }
      }
    }
    expected.push({ table: record.table, id, fields })
  }
  assert.equal(expected.length, 5)
  const payload = Buffer.from(JSON.stringify({ keys, expected }))
  bytes.fill(0)
  process.stdout.write(payload, () => payload.fill(0))
}
try { main() } catch { process.stderr.write('Private merchant rehearsal input unavailable.\n'); process.exitCode = 1 }
