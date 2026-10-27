// Actual installed PayPal crypto source; artificial keys/values only, no HTTP.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const { spawnSync } = require('node:child_process')
const root = path.resolve(__dirname, '../..')
const ts = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))('typescript')
const relative = 'medusa-store/apps/backend/node_modules/@easypayment/medusa-payment-paypal/src/modules/paypal/utils/secret-crypto.ts'
const compiledRelative = 'medusa-store/apps/backend/node_modules/@easypayment/medusa-payment-paypal/.medusa/server/src/modules/paypal/utils/secret-crypto.js'
function load(key, compiled) {
  const source = fs.readFileSync(path.join(root, compiled ? compiledRelative : relative), 'utf8')
  const js = compiled ? source : ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText
  const module = { exports: {} }
  vm.runInNewContext(`(function(require,module,exports){${js}\n})`, { Buffer, console: { warn() {} },
    process: { env: Object.freeze({ PAYPAL_ENCRYPTION_KEY: key }) } }, { timeout: 3000 })(
    name => { assert(['crypto', 'node:crypto'].includes(name)); return crypto }, module, module.exports)
  return module.exports
}
function main() {
  const Key = '\ufeff synthetic-paypal-key \u00a0'
  const source = load(Key, false), runtime = load(Key, true)
  const rows = []
  for (const Plaintext of [' ASCII secret ', ' 中文 merchant 🚀 \u0000 ', 'x'.repeat(1024)]) {
    const iv = crypto.randomBytes(12)
    const cipher = crypto.createCipheriv('aes-256-gcm', crypto.createHash('sha256').update(Key.trim()).digest(), iv)
    const encrypted = Buffer.concat([cipher.update(Plaintext), cipher.final()])
    const v1 = 'enc:v1:' + [iv, cipher.getAuthTag(), encrypted].map(b => b.toString('base64')).join(':')
    for (const [Format, Stored] of [['plaintext', Plaintext], ['v1', v1], ['v2', source.encryptSecret(Plaintext)]]) {
      assert.equal(source.decryptSecret(Stored), Plaintext)
      assert.equal(runtime.decryptSecret(Stored), Plaintext)
      rows.push({ Key, Plaintext, Stored, Format })
    }
    assert.equal(source.decryptSecret(runtime.encryptSecret(Plaintext)), Plaintext)
  }
  const fixture = path.join(root, 'logs/gin-migration/paypal-node-' + crypto.randomUUID() + '.json')
  assert.equal(spawnSync('git', ['check-ignore', '--quiet', fixture], { cwd: root }).status, 0)
  let owned = false
  try {
    fs.writeFileSync(fixture, JSON.stringify(rows), { flag: 'wx' }); owned = true
    const tested = spawnSync('go', ['test', '-count=1', '-run', '^TestPayPalActualNode', './internal/legacysecret', './internal/rowimport'],
      { cwd: path.join(root, 'retail/backend'), env: { ...process.env, PAYPAL_NODE_FIXTURE: fixture }, encoding: 'utf8', timeout: 120000 })
    assert.equal(tested.status, 0)
    const files = [relative, compiledRelative, 'retail/backend/internal/legacysecret/paypal.go',
      'retail/backend/internal/rowimport/config_secret.go', 'retail/backend/internal/rowimport/paypal_secret_test.go',
      'retail/backend/internal/legacysecret/paypal_test.go', 'retail/scripts/test-paypal-secret-interop.cjs']
    const report = { format_version: 1, scope: 'Artificial plaintext/v1/v2 PayPal source and installed runtime to Go decryption and target row sealing only',
      artificial_values: 9, target_sealed_fields: 18, source_and_compiled_runtime_checked: true,
      actual_credentials_checked: false, provider_called: false, source_database_connection: false,
      source_sha256: Object.fromEntries(files.map(f => [f, crypto.createHash('sha256').update(fs.readFileSync(path.join(root, f)).toString('utf8').replaceAll('\r\n', '\n')).digest('hex')])) }
    fs.writeFileSync(path.join(root, 'docs/migration/paypal-secret-interop-report.json'), JSON.stringify(report, null, 2) + '\n')
    console.log(JSON.stringify({ artificial_values: 9, target_sealed_fields: 18, source_and_compiled_runtime_checked: true }))
  } finally { if (owned) fs.unlinkSync(fixture) }
}
try { main() } catch { console.error('PayPal secret interop failed; private details suppressed'); process.exitCode = 1 }
