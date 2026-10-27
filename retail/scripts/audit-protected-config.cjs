// Offline secret readability/reencryption rehearsal. No plaintext or ciphertext
// is written, printed, hashed into reports or sent to any external system.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { spawnSync } = require('node:child_process')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backend = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const ts = backend('typescript')
const dotenvSource = 'medusa-store/node_modules/.pnpm/dotenv@16.6.1/node_modules/dotenv/lib/main.js'
const parseEnv = require(path.join(root, dotenvSource)).parse
const digest = data => crypto.createHash('sha256').update(data).digest('hex')
const domains = {
  mail: ['src/modules/mail/security.ts', ['key', 'encryptSecret', 'decryptSecret'], 'encryptSecret', 'decryptSecret', 'MAIL_ENCRYPTION_KEY'],
  seo: ['src/lib/seo/core.ts', ['invalid', 'encryptionKey', 'encrypt', 'decrypt'], 'encrypt', 'decrypt', 'SEO_ENCRYPTION_KEY'],
  webhook: ['src/modules/webhook-notifications/security.ts', ['key', 'encrypt', 'decrypt'], 'encrypt', 'decrypt', 'WEBHOOK_ENCRYPTION_KEY'],
  mollie: ['src/modules/mollie-settings/security.ts', ['key', 'encryptMollieSecret', 'decryptMollieSecret'], 'encryptMollieSecret', 'decryptMollieSecret', 'MOLLIE_ENCRYPTION_KEY'],
}
const fields = [
  ['mail_account', 'api_key', 'mail'], ['mail_account', 'webhook_secret', 'mail'],
  ['seo_provider', 'api_key', 'seo'], ['seo_setting', 'value', 'seo-setting'],
  ['webhook_endpoint', 'url', 'webhook'], ['webhook_endpoint', 'secret', 'webhook'], ['webhook_endpoint', 'token', 'webhook'],
  ['mollie_config', 'api_key', 'mollie'], ['mail_contact', 'unsubscribe_token', 'plain-business-capability'],
  ['paypal_connection', 'auth_code', 'plain-merchant-secret'], ['paypal_connection', 'seller_client_secret', 'plain-merchant-secret'],
  ['paypal_connection', 'app_access_token', 'plain-merchant-secret'],
]
const sources = { [dotenvSource]: digest(fs.readFileSync(path.join(root, dotenvSource))) }
class LegacyError extends Error { static Types = { INVALID_DATA: 'invalid_data' }; constructor(type) { super('Protected config validation failed'); this.type = type } }
function loadDomain(name, environment) {
  const [relative, names] = domains[name]
  const file = 'medusa-store/apps/backend/' + relative
  const bytes = fs.readFileSync(path.join(root, file))
  sources[file] = digest(bytes)
  const ast = ts.createSourceFile(file, bytes.toString('utf8'), ts.ScriptTarget.Latest, true)
  const selected = new Map()
  for (const statement of ast.statements) {
    if (ts.isFunctionDeclaration(statement) && names.includes(statement.name?.text)) selected.set(statement.name.text, statement.getText(ast))
    if (ts.isVariableStatement(statement)) {
      for (const d of statement.declarationList.declarations) if (ts.isIdentifier(d.name) && names.includes(d.name.text)) {
        assert.equal(statement.declarationList.declarations.length, 1)
        selected.set(d.name.text, statement.getText(ast))
      }
    }
  }
  assert.equal(selected.size, names.length, 'Reviewed crypto declaration changed')
  const source = `const {createCipheriv,createDecipheriv,createHash,randomBytes} = require('node:crypto');\n` +
    names.map(n => selected.get(n)).join('\n') + `\nmodule.exports = {${names.join(',')}};`
  const js = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText
  const module = { exports: {} }
  const requirePure = name => { assert.equal(name, 'node:crypto'); return crypto }
  vm.runInNewContext(`(function(require,module,exports){${js}\n})`, { Buffer, MedusaError: LegacyError,
    process: { env: Object.freeze({ ...environment }) } }, { filename: file, timeout: 3000 })(requirePure, module, module.exports)
  return module.exports
}
function roundtrip(bytes, key, context) {
  const aad = Buffer.from(JSON.stringify(['eurorefurb-protected-config-rehearsal', 1, ...context]))
  const iv = crypto.randomBytes(12)
  const cipher = crypto.createCipheriv('aes-256-gcm', key, iv)
  cipher.setAAD(aad)
  const ciphertext = Buffer.concat([cipher.update(bytes), cipher.final()])
  const tag = cipher.getAuthTag()
  const decrypt = associatedData => {
    const reader = crypto.createDecipheriv('aes-256-gcm', key, iv)
    reader.setAAD(associatedData); reader.setAuthTag(tag)
    return Buffer.concat([reader.update(ciphertext), reader.final()])
  }
  const recovered = decrypt(aad)
  assert(recovered.equals(bytes), 'Protected roundtrip mismatch')
  recovered.fill(0)
  assert.throws(() => decrypt(Buffer.from('other-row-column-purpose')), 'Wrong associated data accepted')
  ciphertext.fill(0)
}
function artificialCases() {
  let count = 0
  for (const [name, [, , encryptName, decryptName, keyName]] of Object.entries(domains)) {
    const good = loadDomain(name, { [keyName]: 'synthetic-domain-key' })
    const plaintext = '人工 fixture only 😀'
    const ciphertext = good[encryptName](plaintext)
    assert.equal(good[decryptName](ciphertext), plaintext); count++
    assert.notEqual(good[encryptName](plaintext), ciphertext); count++
    assert.equal(good[decryptName](''), ''); count++
    const wrong = loadDomain(name, { [keyName]: 'wrong-synthetic-key' })
    assert.throws(() => wrong[decryptName](ciphertext)); count++
    const missing = loadDomain(name, {})
    assert.throws(() => missing[decryptName](ciphertext)); count++
    const fallback = loadDomain(name, { COOKIE_SECRET: 'synthetic-fallback-key' })
    assert.equal(fallback[decryptName](fallback[encryptName](plaintext)), plaintext); count++
    const defaultKey = loadDomain(name, { COOKIE_SECRET: 'supersecret' })
    assert.throws(() => defaultKey[decryptName](ciphertext)); count++
    const parts = ciphertext.split('.'); const content = Buffer.from(parts[2], 'base64'); content[0] ^= 1; parts[2] = content.toString('base64')
    assert.throws(() => good[decryptName](parts.join('.'))); count++
  }
  const key = crypto.randomBytes(32), bytes = Buffer.from('synthetic-protected-value')
  roundtrip(bytes, key, ['fixture-table', 'fixture-column', 'fixture-id']); count++
  bytes.fill(0); key.fill(0)
  return count
}
function main() {
  const options = {}
  for (let i = 2; i < process.argv.length; i++) {
    const flag = process.argv[i]
    if (flag === '--check') options.check = true
    else { assert(['--bundle', '--env', '--report'].includes(flag), 'Unknown audit option'); options[flag.slice(2)] = process.argv[++i] }
  }
  assert(options.bundle && options.env, 'Explicit ignored bundle and env file required')
  for (const file of [options.bundle, options.env]) assert.equal(spawnSync('git', ['check-ignore', '--quiet', path.resolve(file)], { cwd: root }).status, 0, 'Secret inputs must be Git ignored')
  const bundleBytes = fs.readFileSync(options.bundle)
  const bundleHash = digest(bundleBytes)
  assert.equal(bundleHash, 'aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132', 'Unexpected snapshot')
  const tables = JSON.parse(bundleBytes).tables
  // parse only, never dotenv.config or loading the Medusa app. Retain only the
  // four domain keys/fallback; do not fingerprint or report their actual values.
  const parsed = parseEnv(fs.readFileSync(options.env))
  const environment = {}
  for (const key of ['COOKIE_SECRET', ...Object.values(domains).map(d => d[4])]) if (parsed[key] !== undefined) environment[key] = parsed[key]
  const decoders = Object.fromEntries(Object.keys(domains).map(name => [name, loadDomain(name, environment)]))
  const newEphemeralKey = crypto.randomBytes(32)
  const counts = {}, failures = {}
  let encrypted = 0, plain = 0, roundtrips = 0, empty = 0, nulls = 0
  for (const [table, column, kind] of fields) {
    const entry = { rows: tables[table].length, encrypted_readable: 0, plaintext_protected: 0, empty: 0, null: 0, failures: 0 }
    counts[`${table}.${column}`] = entry
    for (const row of tables[table]) {
      const value = row[column]
      if (value === null) { entry.null++; nulls++; continue }
      if (value === '') { entry.empty++; empty++; continue }
      if (typeof value !== 'string') { entry.failures++; failures['non-string-protected-field'] = (failures['non-string-protected-field'] || 0) + 1; continue }
      const domain = kind === 'seo-setting' ? row.is_secret ? 'seo' : null : domains[kind] ? kind : null
      let bytes
      try {
        const plaintext = domain ? decoders[domain][domains[domain][3]](value) : value
        bytes = Buffer.from(plaintext, 'utf8')
        roundtrip(bytes, newEphemeralKey, [table, column, row.id])
        roundtrips++
        if (domain) { entry.encrypted_readable++; encrypted++ }
        else { entry.plaintext_protected++; plain++ }
      } catch {
        entry.failures++
        const code = domain ? `${domain}:decrypt-or-roundtrip-failed` : 'plaintext-roundtrip-failed'
        failures[code] = (failures[code] || 0) + 1
      } finally { bytes?.fill(0) }
    }
  }
  newEphemeralKey.fill(0)
  const artificial = artificialCases()
  const report = { format_version: 1,
    scope: 'Fixed ignored snapshot with explicitly selected ignored .env; source crypto callbacks only, no app/provider/DB writes. No plaintext/ciphertext/key fingerprints or private row IDs in report. Not production key availability or accepted importer proof.',
    source_snapshot_sha256: bundleHash, source_sha256: sources, protected_fields: counts,
    totals: { encrypted_readable: encrypted, plaintext_protected: plain, ephemeral_authenticated_roundtrips: roundtrips, empty, null: nulls },
    artificial_cases: artificial, failures,
    remaining: ['Selected .env is not proof of live process/deployment key configuration', 'No provider authentication/refresh/email/payment/webhook call',
      'Ephemeral key/ciphertexts not persisted; buffer clearing is not proof of physical memory erasure; no persistent Gin envelope, key rotation or SQL import implemented',
      'Empty tables/config values do not validate future populated rows', 'Legacy login/session/reset/verification credentials excluded entirely; merchant secrets and opt-out capability handled separately'],
    status: Object.keys(failures).length ? 'failed-needs-review' : 'offline-readability-checked-import-not-accepted' }
  const file = path.resolve(options.report || path.join(root, 'docs/migration/protected-config-report.json'))
  const content = JSON.stringify(report, null, 2) + '\n'
  if (options.check) assert.equal(fs.readFileSync(file, 'utf8').replaceAll('\r\n', '\n'), content, 'Protected config report drift')
  else fs.writeFileSync(file, content)
  console.log(JSON.stringify({ totals: report.totals, artificial_cases: artificial, failures, status: report.status }))
  if (Object.keys(failures).length) process.exitCode = 1
}
// Never forward errors from crypto/parse/runtime: they may embed protected data.
try { main() } catch { console.error('Protected configuration audit failed; inspect inputs privately. No secret error details emitted.'); process.exitCode = 1 }
