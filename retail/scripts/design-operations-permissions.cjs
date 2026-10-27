// Explicit proposed Gin permissions; old schema enumeration is coverage evidence,
// never an automatic grant imported from legacy accounts.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const root = path.resolve(__dirname, '../..')
const hashes = {}
function read(file) {
  const bytes = fs.readFileSync(path.join(root, file))
  hashes[file] = crypto.createHash('sha256').update(bytes).digest('hex')
  return bytes.toString('utf8')
}
const legacy = JSON.parse(read('docs/migration/schemas/legacy-request-inputs.json'))
for (const file of [
  'medusa-store/apps/backend/src/api/admin/mail/validators.ts',
  'medusa-store/apps/backend/src/api/buyback-validators.ts',
]) assert.equal(crypto.createHash('sha256').update(fs.readFileSync(path.join(root, file))).digest('hex'), legacy.source_sha256[file], 'Legacy input baseline drift')
for (const file of [
  'medusa-store/apps/backend/src/api/admin/mail/route.ts',
  'medusa-store/apps/backend/src/api/admin/mail/commands/route.ts',
  'medusa-store/apps/backend/src/workflows/steps/mail-command.ts',
  'medusa-store/apps/backend/src/api/admin/buyback/route.ts',
  'medusa-store/apps/backend/src/api/admin/buyback/[id]/route.ts',
  'medusa-store/apps/backend/src/workflows/buyback.ts',
]) read(file)
const mail = {
  save_account: ['mail.account.configure', 'mail-account', 'credential-config'],
  delete_account: ['mail.account.delete', 'mail-account', 'delete'],
  save_draft: ['mail.draft.write', 'mail-message-and-account', 'local-write'],
  send: ['mail.message.send', 'mail-message-and-account', 'external-send'],
  'messages.read': ['mail.message.organize', 'each-mail-message-and-account', 'local-write'],
  'messages.unread': ['mail.message.organize', 'each-mail-message-and-account', 'local-write'],
  'messages.star': ['mail.message.organize', 'each-mail-message-and-account', 'local-write'],
  'messages.unstar': ['mail.message.organize', 'each-mail-message-and-account', 'local-write'],
  'messages.trash': ['mail.message.organize', 'each-mail-message-and-account', 'local-write'],
  'messages.restore': ['mail.message.organize', 'each-mail-message-and-account', 'local-write'],
  'messages.archive': ['mail.message.organize', 'each-mail-message-and-account', 'local-write'],
  'messages.delete': ['mail.message.delete', 'each-mail-message-and-account', 'delete'],
  'messages.retry': ['mail.message.send', 'each-mail-message-and-account', 'external-send'],
  contacts: ['mail.contact.write', 'each-mail-contact', 'local-write'],
  unsubscribe_contact: ['mail.contact.unsubscribe', 'mail-contact', 'local-write'],
  save_campaign: ['mail.campaign.write', 'mail-campaign-and-account', 'local-write'],
  'campaign.queue': ['mail.campaign.send', 'mail-campaign-account-and-each-contact', 'external-send'],
  'campaign.pause': ['mail.campaign.pause', 'mail-campaign-and-account', 'stop-send'],
  'campaign.resume': ['mail.campaign.send', 'mail-campaign-account-and-each-contact', 'external-send'],
  'campaign.retry': ['mail.campaign.send', 'mail-campaign-account-and-each-contact', 'external-send'],
  save_template: ['mail.template.write', 'mail-template', 'local-write'],
  delete_template: ['mail.template.delete', 'mail-template', 'delete'],
}
const buyback = {
  review_photo: ['buyback.photo.review', 'buyback-request-and-photo', 'local-write'],
  approve_shipping: ['buyback.shipping.approve', 'buyback-request', 'local-write'],
  mark_received: ['buyback.receipt.confirm', 'buyback-request', 'local-write'],
  link_device: ['buyback.device.link', 'buyback-request-item-and-device', 'local-write'],
  final_offer: ['buyback.offer.write', 'buyback-request-and-each-item', 'local-write'],
  mark_paid: ['buyback.payment.record', 'buyback-request', 'payment-evidence'],
  request_return: ['buyback.return.request', 'buyback-request', 'local-write'],
  mark_returning: ['buyback.return.ship', 'buyback-request', 'local-write'],
  mark_returned: ['buyback.return.complete', 'buyback-request', 'local-write'],
  cancel: ['buyback.cancel', 'buyback-request', 'local-write'],
}
function covered(name, mapping) {
  const schema = legacy.schemas.find(entry => entry.export === name)?.schema
  assert(schema?.oneOf, 'Reviewed discriminated union missing')
  const selectors = schema.oneOf.flatMap(branch => {
    const action = branch.properties.action.const
    assert.equal(typeof action, 'string')
    return branch.properties.operation?.enum ? branch.properties.operation.enum.map(operation => action + '.' + operation) : [action]
  })
  assert.deepEqual(Object.keys(mapping).sort(), selectors.sort(), 'Permission branches must cover the actual input union exactly: ' + name)
}
covered('MailCommand', mail); covered('BuybackAdminCommand', buyback)
const mailResources = {
  messages: ['mail.message.read', 'mail-message-and-account'], summary: ['mail.message.read', 'mail-account'],
  accounts: ['mail.account.read', 'mail-account'], contacts: ['mail.contact.read', 'mail-contact'],
  campaigns: ['mail.campaign.read', 'mail-campaign-and-account'], deliveries: ['mail.delivery.read', 'mail-delivery-campaign-and-account'],
  templates: ['mail.template.read', 'mail-template'], events: ['mail.event.read', 'mail-event-and-account'],
}
const listSchema = legacy.schemas.find(entry => entry.export === 'MailListQuery')?.schema
assert.deepEqual(Object.keys(mailResources).sort(), [...listSchema.properties.resource.enum].sort())
const entries = []
for (const [domain, mapping, commandPath] of [['mail', mail, '/api/admin/mail/commands'], ['buyback', buyback, '/api/admin/buyback/{request_id}']])
  for (const [selector, [permission, scope, effect]] of Object.entries(mapping)) {
    const [action, operation] = selector.split('.')
    const mayCreate = domain === 'mail' && ['save_account', 'save_draft', 'save_campaign', 'save_template'].includes(action)
    entries.push({ id: domain + '.command.' + selector, method: 'POST', path: commandPath,
      selector: { action, ...(operation ? { operation } : {}) }, permission, scope, effect,
      required_headers: ['Origin', 'X-CSRF-Token', 'Idempotency-Key', ...(mayCreate ? [] : ['If-Match'])],
      version_precondition: mayCreate ? { existing_id: 'If-Match: strong current aggregate ETag', absent_id: 'If-None-Match: *; reject If-Match and use canonical creation uniqueness', reject_both: true }
        : { existing_aggregate: 'If-Match: strong current aggregate ETag; bulk commands use an authorized collection revision covering every selected member' },
      version_subject: 'Locked aggregate with current object authorization; contacts bulk upsert uses collection revision; never a fake existing version',
      outcome: effect === 'external-send' ? 'durable operation; execution reauthorizes and reconciles external outcome' : 'transactional versioned result or durable operation for dependent notifications',
    })
  }
for (const [resource, [permission, scope]] of Object.entries(mailResources)) entries.push({ id: 'mail.list.' + resource,
  method: 'GET', path: '/api/admin/mail', selector: { resource }, permission, scope, effect: 'read', default_selector: resource === 'messages' })
for (const [id, route, permission, scope] of [
  ['mail.message.retrieve', '/api/admin/mail/messages/{message_id}', 'mail.message.read', 'mail-message-and-account'],
  ['mail.attachment.download', '/api/admin/mail/messages/{message_id}/attachments/{attachment_id}', 'mail.attachment.download', 'mail-message-account-and-exact-attachment'],
  ['buyback.list', '/api/admin/buyback', 'buyback.read', 'buyback-request'],
  ['buyback.retrieve', '/api/admin/buyback/{request_id}', 'buyback.read', 'buyback-request'],
  ['buyback.photo.download', '/api/admin/buyback/{request_id}/photos/{photo_id}', 'buyback.photo.read', 'buyback-request-and-photo'],
]) entries.push({ id, method: 'GET', path: route, selector: {}, permission, scope, effect: 'read' })
const design = { format_version: 1, status: 'proposed-branch-permissions-not-Gin-enforcement',
  identity: 'new-staff-session; restricted supplier and warehouse denied for all entries',
  permission_semantics: 'Exact named grants; no implicit prefix, admin-role-name or read-to-write expansion. This is target design, not a claim about legacy grants.',
  selection: 'Normalize and validate route/query/body before selection; match exactly one entry; reject unknown action/resource/suboperation and repeated/conflicting selectors. No broad route-level permission fallback.',
  authorization_order: ['current authenticated enabled staff and permission version', 'exact entry grant', 'each current object and parent relationship', 'version/idempotency rules inside transaction', 'domain state/invariants', 'allowlisted response'],
  scope_semantics: 'Account/source/object action scope checked against current persistent grants; list count/filter/page after authorization. Request-provided account_id never grants access. New accounts inherit no old email ownership or personal grants.',
  response: 'private,no-store; secrets never echoed; no raw internal payload/metadata projection; errors use stable common envelope',
  secret_handling: 'Credential edits use protected versioned keyed request HMAC for dedupe; never log/store plain secret or unkeyed secret hash in operation/audit/request payload',
  entries, source_sha256: hashes,
  remaining: ['Exact per-command field/response schemas and absence-precondition header schema', 'Native and remaining custom domains, role seed choices and actor data scope storage', 'Gin/SQL/worker enforcement, account revocation, state and failure/concurrency tests', 'Ant Design views consume granted action IDs; UI hiding is not enforcement'] }
const content = JSON.stringify(design, null, 2) + '\n'
const output = path.join(root, 'docs/migration/schemas/operations-permissions.json')
if (process.argv.includes('--check')) assert.equal(fs.readFileSync(output, 'utf8').replaceAll('\r\n', '\n'), content)
else fs.writeFileSync(output, content)
console.log(JSON.stringify({ command_branches: Object.keys(mail).length + Object.keys(buyback).length,
  list_branches: Object.keys(mailResources).length, detail_downloads: 5, entries: entries.length, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
