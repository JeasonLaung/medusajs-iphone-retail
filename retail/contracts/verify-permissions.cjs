// Contract decision model only. This does not implement Gin authorization.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const design = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../docs/migration/schemas/operations-permissions.json'), 'utf8'))
const entries = design.entries
const ids = new Set(), signatures = new Set()
for (const entry of entries) {
  assert(!ids.has(entry.id)); ids.add(entry.id)
  const signature = entry.method + ' ' + entry.path + ' ' + JSON.stringify(entry.selector)
  assert(!signatures.has(signature)); signatures.add(signature)
  assert(/^[a-z_]+\.[a-z_.]+$/.test(entry.permission))
  assert(entry.scope && entry.effect)
  if (entry.method === 'POST') {
    for (const header of ['Origin', 'X-CSRF-Token', 'Idempotency-Key']) assert(entry.required_headers.includes(header))
    assert(entry.version_precondition)
    assert(entry.required_headers.includes('If-Match') || entry.version_precondition.absent_id)
  }
}
// Object-safe exact selector: input is already validated/normalized. This model
// deliberately rejects extra selector keys (not other valid body fields).
function select(method, routeTemplate, selector) {
  if (!selector || Object.getPrototypeOf(selector) !== Object.prototype) return null
  const matches = entries.filter(entry => entry.method === method && entry.path === routeTemplate &&
    Object.keys(entry.selector).length === Object.keys(selector).length &&
    Object.entries(entry.selector).every(([key, value]) => Object.hasOwn(selector, key) && selector[key] === value))
  return matches.length === 1 ? matches[0] : null
}
function permits(entry, actor, facts) {
  return Boolean(entry && actor.kind === 'staff' && actor.enabled === true && actor.mode === 'ordinary' &&
    actor.permission_version_current === true && Array.isArray(actor.grants) && actor.grants.includes(entry.permission) &&
    facts.object_scope_current === true && facts.parents_match === true && facts.all_members_visible === true)
}
const ordinary = { kind: 'staff', mode: 'ordinary', enabled: true, permission_version_current: true, grants: [] }
const facts = { object_scope_current: true, parents_match: true, all_members_visible: true }
const cases = []
function check(name, run) { run(); cases.push(name) }
for (const entry of entries) check('exact-entry-selection-' + entry.id, () => assert.equal(select(entry.method, entry.path, entry.selector)?.id, entry.id))
const commandPath = '/api/admin/mail/commands'
const send = select('POST', commandPath, { action: 'send' })
const retry = select('POST', commandPath, { action: 'messages', operation: 'retry' })
const organize = select('POST', commandPath, { action: 'messages', operation: 'read' })
const pause = select('POST', commandPath, { action: 'campaign', operation: 'pause' })
const resume = select('POST', commandPath, { action: 'campaign', operation: 'resume' })
check('message-retry-requires-send-not-organize', () => {
  assert.equal(permits(retry, { ...ordinary, grants: [organize.permission] }, facts), false)
  assert.equal(permits(retry, { ...ordinary, grants: [send.permission] }, facts), true)
})
check('campaign-pause-does-not-allow-resume-or-queue', () => {
  const actor = { ...ordinary, grants: [pause.permission] }
  assert.equal(permits(pause, actor, facts), true); assert.equal(permits(resume, actor, facts), false)
  assert.equal(permits(select('POST', commandPath, { action: 'campaign', operation: 'queue' }), actor, facts), false)
})
check('mail-read-does-not-grant-send-config-delete-or-attachments', () => {
  const actor = { ...ordinary, grants: ['mail.message.read'] }
  for (const entry of entries.filter(e => ['external-send', 'credential-config', 'delete'].includes(e.effect) || e.id === 'mail.attachment.download')) assert.equal(permits(entry, actor, facts), false)
})
check('buyback-read-does-not-record-payment-or-link-device', () => {
  for (const action of ['mark_paid', 'link_device', 'cancel']) assert.equal(permits(select('POST', '/api/admin/buyback/{request_id}', { action }), { ...ordinary, grants: ['buyback.read'] }, facts), false)
})
check('unknown-action-operation-and-resource-have-no-fallback', () => {
  for (const selector of [{ action: 'unknown' }, { action: 'messages' }, { action: 'messages', operation: 'unknown' }, { action: 'send', operation: 'retry' }, { action: ['send'] }, { action: 'send', resource: 'accounts' }]) assert.equal(select('POST', commandPath, selector), null)
  assert.equal(select('GET', '/api/admin/mail', { resource: 'unknown' }), null)
})
check('wrong-method-or-template-denied', () => {
  assert.equal(select('GET', commandPath, { action: 'send' }), null)
  assert.equal(select('POST', '/api/admin/mail', { action: 'send' }), null)
})
check('inherited-prototype-selector-denied', () => assert.equal(select('POST', commandPath, Object.create({ action: 'send' })), null))
for (const [name, overrides] of [
  ['customer-identity', { kind: 'customer' }], ['integration-not-staff', { kind: 'integration' }],
  ['disabled-staff', { enabled: false }], ['supplier-mode', { mode: 'supplier' }],
  ['warehouse-mode', { mode: 'warehouse' }], ['stale-permission-version', { permission_version_current: false }],
  ['implicit-admin-name-no-grant', { grants: [], role: 'admin' }], ['prefix-grant-no-expansion', { grants: ['mail.*'] }],
]) check(name + '-denied-even-with-send-grant', () => assert.equal(permits(send, { ...ordinary, grants: [send.permission], ...overrides }, facts), false))
for (const field of Object.keys(facts)) check('current-grant-insufficient-with-failed-' + field, () => assert.equal(permits(send, { ...ordinary, grants: [send.permission] }, { ...facts, [field]: false }), false))
check('same-grant-still-needs-current-enabled-actor-on-replay-and-worker', () => {
  const actor = { ...ordinary, grants: [send.permission] }
  assert.equal(permits(send, actor, facts), true)
  assert.equal(permits(send, { ...actor, enabled: false }, facts), false)
})
console.log(JSON.stringify({ permission_contract_entries: entries.length, design_cases: cases.length, scope: 'contract-model-only-not-Gin-enforcement' }))
