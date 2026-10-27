// Artificial local crypto/state baselines only. No app/env/DB/provider access.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backend = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const ts = backend('typescript')
const tokenFile = 'medusa-store/apps/backend/src/lib/mobile-intake-token.ts'
const mailFile = 'medusa-store/apps/backend/src/workflows/mail-webhook.ts'
const sources = {}
function read(file) {
  const bytes = fs.readFileSync(path.join(root, file))
  sources[file] = crypto.createHash('sha256').update(bytes).digest('hex')
  return bytes.toString('utf8')
}
const tokenSource = read(tokenFile)
const moduleObject = { exports: {} }
// Error stub reproduces only construction/message, not framework classification or HTTP behavior.
class MedusaError extends Error { static Types = { UNAUTHORIZED: 'unauthorized' } ; constructor(type, message) { super(message); this.type = type } }
const requirePure = name => {
  if (name === 'node:crypto') return crypto
  if (name === '@medusajs/framework/utils') return { MedusaError }
  throw new Error('Dependency outside pure capability allowlist')
}
const compiled = ts.transpileModule(tokenSource, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
vm.runInNewContext(`(function(require,module,exports){${compiled}\n})`, { Buffer }, { timeout: 3000 })(requirePure, moduleObject, moduleObject.exports)
const { createIntakeToken, verifyIntakeToken } = moduleObject.exports
const mailSource = read(mailFile)
const ast = ts.createSourceFile(mailFile, mailSource, ts.ScriptTarget.Latest, true)
const declarations = ast.statements.filter(node =>
  (ts.isFunctionDeclaration(node) && node.name?.text === 'deliveryStatus') ||
  (ts.isVariableStatement(node) && node.declarationList.declarations.some(d => d.name.getText(ast) === 'failureStatuses')))
assert.equal(declarations.length, 2)
const pureMail = declarations.map(d => d.getText(ast)).join('\n').replace('export function', 'function')
const deliveryStatus = vm.runInNewContext(ts.transpileModule(pureMail + '\ndeliveryStatus', { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText, {}, { timeout: 3000 })
const now = 1700000000000, secret = 'artificial-capability-key-not-production'
const claims = { purpose: 'mobile-device-intake', actor_id: 'fixture_actor', source_id: 'fixture_source', category: 'sale', expires_at: now + 7200000, nonce: 'fixture_nonce' }
function sign(value, key = secret) {
  const payload = Buffer.from(JSON.stringify(value)).toString('base64url')
  const signature = crypto.createHmac('sha256', key).update(`mobile-intake:${payload}`).digest('base64url')
  return `${payload}.${signature}`
}
const cases = []
function tokenCase(name, token, at, accepted, expected, key = secret) {
  let actual, error
  try { actual = JSON.parse(JSON.stringify(verifyIntakeToken(token, key, at))) } catch (e) { error = e }
  assert.equal(!error, accepted, name)
  if (accepted) assert.deepEqual(actual, expected, name)
  else assert.equal(error.message, 'MOBILE_INTAKE_LINK_INVALID', name)
  // Never store tokens or signatures, even though these fixtures use an artificial key.
  cases.push({ domain: 'mobile-token', name, accepted, at, ...(accepted ? { expected_claims: expected } : { expected_error: 'MOBILE_INTAKE_LINK_INVALID' }) })
}
const valid = sign(claims)
tokenCase('signed-claims', valid, now, true, claims)
tokenCase('one-millisecond-before-expiry', valid, claims.expires_at - 1, true, claims)
tokenCase('expiry-is-exclusive', valid, claims.expires_at, false)
tokenCase('wrong-key', valid, now, false, undefined, 'other-artificial-key')
tokenCase('empty-key', valid, now, false, undefined, '')
tokenCase('empty-token', '', now, false)
tokenCase('overlength', 'a'.repeat(2049), now, false)
tokenCase('extra-segment', valid + '.extra', now, false)
const [payload, signature] = valid.split('.')
tokenCase('unsigned-source-change', Buffer.from(JSON.stringify({ ...claims, source_id: 'other' })).toString('base64url') + '.' + signature, now, false)
tokenCase('bad-signature-length', payload + '.a', now, false)
for (const [name, changes] of [
  ['wrong-purpose', { purpose: 'login' }], ['missing-actor', { actor_id: '' }],
  ['missing-source', { source_id: '' }], ['unsupported-category', { category: 'other' }],
  ['nonnumeric-expiry', { expires_at: String(claims.expires_at) }],
]) tokenCase(name, sign({ ...claims, ...changes }), now, false)
const oldClaims = { ...claims }; delete oldClaims.category
tokenCase('legacy-missing-category-defaults-sale', sign(oldClaims), now, true, claims)
for (const category of ['returned', 'recycled']) tokenCase('signed-category-' + category, sign({ ...claims, category }), now, true, { ...claims, category })
const generated = createIntakeToken('fixture_actor', 'fixture_source', secret, now, 'returned')
assert.equal(generated.expires_at, now + 7200000)
const generatedClaims = verifyIntakeToken(generated.token, secret, now)
assert.match(generatedClaims.nonce, /^[0-9a-f-]{36}$/)
assert.equal(generatedClaims.category, 'returned')
cases.push({ domain: 'mobile-token', name: 'generated-two-hour-session-and-nonce', accepted: true, lifetime_ms: 7200000, category: 'returned', nonce: 'random-uuid-asserted-not-recorded' })
// This pure verifier does not consume nonce or consult actor/source permission.
assert.deepEqual(verifyIntakeToken(valid, secret, now), verifyIntakeToken(valid, secret, now))
cases.push({ domain: 'mobile-token', name: 'nonce-not-consumed-by-pure-verifier', accepted: true,
  limitation: 'Current actor/source/category permission and device deletion ownership require domain access checks' })
for (const [current, incoming, expected] of [
  ['queued', 'sent', 'sent'], ['sent', 'delivered', 'delivered'], ['delivered', 'sent', 'delivered'],
  ['delivered', 'opened', 'delivered'], ['sent', 'clicked', 'sent'], ['delivered', 'bounced', 'bounced'],
  ['complained', 'delivered', 'complained'], ['failed', 'sent', 'failed'], ['suppressed', 'bounced', 'suppressed'],
]) {
  assert.equal(deliveryStatus(current, incoming), expected)
  cases.push({ domain: 'mail-delivery-status', name: `${current}-then-${incoming}`, input: { current, incoming }, expected })
}
// Execute the exact locally installed verifier used by Resend with a fixed clock.
const resendFile = backend.resolve('resend')
const resendRequire = createRequire(resendFile)
const standardFile = resendRequire.resolve('standardwebhooks')
const standardRequire = createRequire(standardFile)
const standardSource = read(path.relative(root, standardFile).replaceAll('\\', '/'))
read(path.relative(root, resendFile).replaceAll('\\', '/'))
const standardModule = { exports: {} }
class FixedDate extends Date { static now() { return now } }
vm.runInNewContext(`(function(require,module,exports){${standardSource}\n})`, { Date: FixedDate, TextEncoder, Uint8Array }, { timeout: 3000 })(standardRequire, standardModule, standardModule.exports)
const artificialKey = Buffer.from('artificial-webhook-signing-key-32')
const wh = new standardModule.exports.Webhook('whsec_' + artificialKey.toString('base64'))
const body = '{"type":"email.delivered","data":{"email_id":"fixture_mail"}}'
const eventId = 'fixture_event', seconds = now / 1000
const signatureFor = timestamp => 'v1,' + crypto.createHmac('sha256', artificialKey).update(`${eventId}.${timestamp}.${body}`).digest('base64')
const headers = { 'webhook-id': eventId, 'webhook-timestamp': String(seconds), 'webhook-signature': signatureFor(seconds) }
const webhookCases = [
  ['valid-raw-body', body, headers, true],
  ['changed-json-whitespace', body + ' ', headers, false],
  ['changed-event-id', body, { ...headers, 'webhook-id': 'other' }, false],
  ['missing-signature', body, { ...headers, 'webhook-signature': '' }, false],
  ['unsupported-signature-version', body, { ...headers, 'webhook-signature': headers['webhook-signature'].replace('v1,', 'v2,') }, false],
  ['multiple-signatures-with-valid-v1', body, { ...headers, 'webhook-signature': 'v2,ignored ' + headers['webhook-signature'] }, true],
  ['old-exactly-300-seconds', body, { ...headers, 'webhook-timestamp': String(seconds-300), 'webhook-signature': signatureFor(seconds-300) }, true],
  ['old-301-seconds', body, { ...headers, 'webhook-timestamp': String(seconds-301), 'webhook-signature': signatureFor(seconds-301) }, false],
  ['future-exactly-300-seconds', body, { ...headers, 'webhook-timestamp': String(seconds+300), 'webhook-signature': signatureFor(seconds+300) }, true],
  ['future-301-seconds', body, { ...headers, 'webhook-timestamp': String(seconds+301), 'webhook-signature': signatureFor(seconds+301) }, false],
  ['legacy-parseInt-accepts-timestamp-suffix', body, { ...headers, 'webhook-timestamp': String(seconds)+'invalid' }, true],
]
for (const [name, payload, signedHeaders, accepted] of webhookCases) {
  let parsed, error
  try { parsed = wh.verify(payload, signedHeaders) } catch (e) { error = e }
  assert.equal(!error, accepted, name)
  if (accepted) assert.deepEqual(JSON.parse(JSON.stringify(parsed)), JSON.parse(body))
  cases.push({ domain: 'resend-standardwebhooks', name, accepted,
    ...(name.includes('parseInt') ? { new_contract: 'Reject noncanonical timestamp; do not reproduce this source parsing weakness' } : {}) })
}
const output = { format_version: 1,
  scope: 'Artificial pure token/HMAC, extracted mail status and installed Standard Webhooks signature rules with fixed clock; framework error construction stubbed. Not live role revocation, HTTP raw-body integration, database dedupe, mailbox ownership or new Gin acceptance.',
  source_sha256: sources, cases }
const file = path.join(root, 'docs/migration/fixtures/legacy-capability-rules.json')
const content = JSON.stringify(output, null, 2) + '\n'
if (process.argv.includes('--check')) assert.equal(fs.readFileSync(file, 'utf8').replaceAll('\r\n', '\n'), content)
else fs.writeFileSync(file, content)
console.log(JSON.stringify({ source_files: Object.keys(sources).length, cases: cases.length, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
