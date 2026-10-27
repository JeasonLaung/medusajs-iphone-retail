// Artificial values only. Execute the reviewed actual TypeScript crypto
// functions in the restricted VM, then authenticate their output in Go.
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const { spawnSync } = require('node:child_process')
const { loadDomain } = require('./audit-protected-config.cjs')
const root = path.resolve(__dirname, '../..')
const fields = {
  mail: ['MAIL_ENCRYPTION_KEY', 'encryptSecret'], seo: ['SEO_ENCRYPTION_KEY', 'encrypt'],
  webhook: ['WEBHOOK_ENCRYPTION_KEY', 'encrypt'], mollie: ['MOLLIE_ENCRYPTION_KEY', 'encryptMollieSecret'],
}
function main() {
  const fixture = path.join(root, 'logs/gin-migration/artificial-legacy-' + crypto.randomUUID() + '.json')
  if (spawnSync('git', ['check-ignore', '--quiet', fixture], { cwd: root }).status !== 0) throw new Error('Fixture must be ignored')
  const rows = []
  for (const [Domain, [keyName, encryptName]] of Object.entries(fields)) {
    const Secret = 'artificial-source-' + Domain
    const api = loadDomain(Domain, { [keyName]: Secret })
    for (const Plaintext of ['', ' ASCII secret ', ' 中文 🚀 \u0000 ', 'x'.repeat(1024)]) {
      rows.push({ Domain, Secret, Plaintext, Ciphertext: api[encryptName](Plaintext) })
    }
  }
  let created = false
  try {
    fs.writeFileSync(fixture, JSON.stringify(rows), { flag: 'wx' }); created = true
    const result = spawnSync('go', ['test', '-count=1', '-run', '^TestActualNodeLegacy', './internal/legacysecret', './internal/rowimport'],
      { cwd: path.join(root, 'retail/backend'), env: { ...process.env, LEGACY_SECRET_NODE_FIXTURE: fixture }, encoding: 'utf8', timeout: 120000 })
    if (result.status !== 0) throw new Error('Node/Go artificial secret interop failed')
    console.log(JSON.stringify({ actual_source_domains: 4, artificial_values: rows.length, go_authentication_and_row_conversion: true }))
  } finally { if (created) fs.unlinkSync(fixture) }
}
try { main() } catch { console.error('Artificial legacy interop verification failed; no secret error details emitted.'); process.exitCode = 1 }
