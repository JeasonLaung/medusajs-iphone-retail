// Source supplier identity, middleware and command authorization on artificial ports.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const source = 'medusa-store/apps/backend/src/api/device-access.ts'
const bytes = fs.readFileSync(path.join(root, source))
const ts = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))('typescript')
const clone = value => JSON.parse(JSON.stringify(value))
class AccessError extends Error {
  static Types = { FORBIDDEN: 'forbidden' }
  constructor(type, message) { super(message); this.type = type }
}
const record = { exports: {} }
const js = ts.transpileModule(bytes.toString('utf8'), { compilerOptions: {
  target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS,
} }).outputText
vm.runInNewContext(`(function(require,module,exports){${js}\n})`, {}, { filename: source, timeout: 3000 })(name => {
  if (name === '@medusajs/framework/utils') return { MedusaError: AccessError,
    Modules: { USER: 'user', API_KEY: 'api-key', STORE: 'store' } }
  if (name === '../modules/devices') return { DEVICES_MODULE: 'devices' }
  throw new Error('Unreviewed dependency: ' + name)
}, record, record.exports)
const legacy = record.exports, cases = []
const supplierMetadata = { device_role: 'supplier', device_source_id: 'source-a', device_order_ids: ['order-a'] }
function ports(options = {}) {
  const calls = []
  const user = { id: 'staff-a', email: 'fixture@example.invalid', password_hash: 'artificial-must-not-project',
    first_name: 'Fixture', last_name: 'Supplier', avatar_url: null,
    metadata: Object.hasOwn(options, 'metadata') ? options.metadata : supplierMetadata }
  const assignment = { id: 'assignment-a', source_id: 'source-a', order_id: 'order-a' }
  const container = { resolve(name) {
    calls.push({ resolve: name })
    if (name === 'user') return {
      listUsers: async filter => { calls.push({ listUsers: clone(filter) }); if (options.failUsers) throw new Error('USER_PORT_UNAVAILABLE'); return options.missingUser ? [] : [user] },
      retrieveUser: async id => { calls.push({ retrieveUser: id }); return user },
    }
    if (name === 'api-key') return { listApiKeys: async filter => { calls.push({ apiKeys: clone(filter) }); return options.secretKey ? [{ id: 'key-fixture' }] : [] } }
    if (name === 'devices') return {
      listDeviceSources: async filter => { calls.push({ sources: clone(filter) }); return options.missingSource ? [] : [{ id: filter.id, type: 'supplier', location_id: 'location-a' }] },
      retrieveDevice: async id => { calls.push({ device: id }); return options.device || { id, source_id: 'source-a', location_id: 'location-a' } },
      retrieveDeviceSource: async id => { calls.push({ source: id }); return options.source || { id, location_id: 'location-a' } },
      retrieveDeviceAssignment: async id => { calls.push({ assignment: id }); return options.assignment || assignment },
      listDeviceAssignments: async filter => { calls.push({ assignments: clone(filter) }); return options.assignments || [assignment] },
    }
    if (name === 'store') return { listStores: async (filter, config) => {
      calls.push({ stores: clone(filter), config: clone(config) }); return [{ id: 'store-fixture', name: 'Fixture Shop', metadata: { private: true }, currency: 'eur' }]
    } }
    throw new Error('Unreviewed resolve: ' + name)
  } }
  return { container, calls }
}
async function check(name, run) { await run(); cases.push({ name, passed: true }) }
async function denied(run) { await assert.rejects(run, error => error.type === 'forbidden' && error.message === 'Supplier access denied') }
async function command(name, body, options = {}, reject = false, verify) {
  await check(name, async () => {
    const p = ports(options)
    const run = () => legacy.authorizeDeviceCommand(p.container, 'staff-a', body)
    if (reject) await denied(run); else await run()
    if (verify) verify(p.calls)
  })
}
async function middleware(name, method, url, options = {}, expected = 'next', verify) {
  await check(name, async () => {
    const p = ports(options)
    let next = 0, response
    const req = { method, originalUrl: url, body: options.body,
      auth_context: Object.hasOwn(options, 'auth') ? options.auth : { actor_id: 'staff-a', actor_type: 'user' }, scope: p.container }
    const run = () => legacy.guardSupplierAdmin(req, { json(value) { response = clone(value); return value } }, () => { next++ })
    if (expected === 'denied') await denied(run); else await run()
    assert.equal(next, expected === 'next' ? 1 : 0)
    if (expected !== 'response') assert.equal(response, undefined)
    else assert(response)
    if (verify) verify(p.calls, response)
  })
}
async function main() {
  await check('supplier-identity-requires-typed-source-and-explicit-orders', async () => {
    const p = ports()
    assert.deepEqual(clone(await legacy.deviceAccess(p.container, 'staff-a')), { role: 'supplier', source_id: 'source-a', order_ids: ['order-a'] })
    assert.deepEqual(p.calls.find(c => c.sources).sources, { id: 'source-a', type: 'supplier' })
  })
  for (const [name, metadata] of [['missing-source', { device_role: 'supplier' }], ['empty-source', { device_role: 'supplier', device_source_id: '' }], ['numeric-source', { device_role: 'supplier', device_source_id: 1 }]])
    await check(`identity-${name}-denied`, () => denied(() => legacy.deviceAccess(ports({ metadata }).container, 'staff-a')))
  await check('identity-source-no-longer-valid-denied', () => denied(() => legacy.deviceAccess(ports({ missingSource: true }).container, 'staff-a')))
  await check('identity-no-account-or-secret-key-denied', () => denied(() => legacy.deviceAccess(ports({ missingUser: true }).container, 'staff-a')))
  await check('identity-old-secret-key-is-admin-observation', async () => {
    const p = ports({ missingUser: true, secretKey: true })
    assert.deepEqual(clone(await legacy.deviceAccess(p.container, 'key-fixture')), { role: 'admin', source_id: null, order_ids: [] })
    assert.deepEqual(p.calls.find(c => c.apiKeys).apiKeys, { id: 'key-fixture', type: 'secret' })
  })
  await check('identity-ordinary-staff-is-admin-scope-not-rbac-proof', async () => assert.equal((await legacy.deviceAccess(ports({ metadata: {} }).container, 'staff-a')).role, 'admin'))
  await check('identity-legacy-order-filter-retains-duplicates-empty-strings', async () => {
    const p = ports({ metadata: { ...supplierMetadata, device_order_ids: ['order-a', '', null, 1, 'order-a'] } })
    assert.deepEqual(clone((await legacy.deviceAccess(p.container, 'staff-a')).order_ids), ['order-a', '', 'order-a'])
  })
  await check('identity-user-port-failure-not-swallowed', () => assert.rejects(() => legacy.deviceAccess(ports({ failUsers: true }).container, 'staff-a'), /USER_PORT_UNAVAILABLE/))
  await command('supply-own-source-allowed', { action: 'supply', source_id: 'source-a' })
  await command('supply-other-source-denied', { action: 'supply', source_id: 'source-b' }, {}, true)
  const supplierAssign = { action: 'supplier_assign', device: { source_id: 'source-a' }, order_id: 'order-a' }
  await command('supplier-assign-guard-allows-own-not-workflow-acceptance', supplierAssign)
  await command('supplier-assign-other-order-denied', { ...supplierAssign, order_id: 'order-b' }, {}, true)
  await command('supplier-assign-other-source-denied', { ...supplierAssign, device: { source_id: 'source-b' } }, {}, true)
  await command('supplier-assign-replacement-denied', { ...supplierAssign, replacement_of: 'assignment-old' }, {}, true)
  const assign = { action: 'assign', device_id: 'device-a', order_id: 'order-a' }
  await command('assign-existing-own-source-location-order', assign)
  await command('assign-device-other-source-denied', assign, { device: { source_id: 'source-b', location_id: 'location-a' } }, true)
  await command('assign-device-other-location-denied', assign, { device: { source_id: 'source-a', location_id: 'location-b' } }, true)
  await command('assign-order-not-granted-denied', { ...assign, order_id: 'order-b' }, {}, true)
  await command('assign-replacement-denied', { ...assign, replacement_of: 'assignment-old' }, {}, true)
  await command('assign-empty-grant-denied', assign, { metadata: { ...supplierMetadata, device_order_ids: [] } }, true)
  await command('release-own-assignment-allowed', { action: 'release', assignment_id: 'assignment-a' })
  for (const [field, value] of [['source_id', 'source-b'], ['order_id', 'order-b']])
    await command(`release-other-${field}-denied`, { action: 'release', assignment_id: 'assignment-a' }, { assignment: { source_id: 'source-a', order_id: 'order-a', [field]: value } }, true)
  await command('fulfill-own-assignments-allowed', { action: 'fulfill', assignment_ids: ['assignment-a'] }, {}, false,
    calls => assert.deepEqual(calls.find(c => c.assignments).assignments, { id: ['assignment-a'] }))
  await command('fulfill-missing-assignment-denied', { action: 'fulfill', assignment_ids: ['assignment-a'] }, { assignments: [] }, true)
  await command('fulfill-duplicate-request-ids-count-mismatch-denied', { action: 'fulfill', assignment_ids: ['assignment-a', 'assignment-a'] }, {}, true)
  await command('fulfill-scope-relies-on-query-id-filter-not-identity-recheck', { action: 'fulfill', assignment_ids: ['assignment-a'] }, {
    assignments: [{ id: 'unexpected-artificial-row', source_id: 'source-a', order_id: 'order-a' }],
  })
  await command('fulfill-scope-allows-two-granted-orders-workflow-must-separate', { action: 'fulfill', assignment_ids: ['assignment-a', 'assignment-b'] }, {
    metadata: { ...supplierMetadata, device_order_ids: ['order-a', 'order-b'] },
    assignments: [{ id: 'assignment-a', source_id: 'source-a', order_id: 'order-a' }, { id: 'assignment-b', source_id: 'source-a', order_id: 'order-b' }],
  })
  for (const field of ['source_id', 'order_id'])
    await command(`fulfill-second-cross-${field}-denied`, { action: 'fulfill', assignment_ids: ['assignment-a', 'assignment-b'] }, { assignments: [
      { id: 'assignment-a', source_id: 'source-a', order_id: 'order-a' }, { id: 'assignment-b', source_id: 'source-a', order_id: 'order-a', [field]: 'outside' },
    ] }, true)
  for (const action of ['ship', 'cancel_fulfillment']) {
    await command(`${action}-own-assignments-allowed`, { action, fulfillment_id: 'fulfillment-a' }, {}, false,
      calls => assert.deepEqual(calls.find(c => c.assignments).assignments, { fulfillment_id: 'fulfillment-a' }))
    await command(`${action}-empty-parcel-denied`, { action, fulfillment_id: 'fulfillment-a' }, { assignments: [] }, true)
    await command(`${action}-mixed-source-parcel-denied`, { action, fulfillment_id: 'fulfillment-a' }, { assignments: [
      { source_id: 'source-a', order_id: 'order-a' }, { source_id: 'source-b', order_id: 'order-a' },
    ] }, true)
  }
  for (const action of ['source', 'enroll', 'intake', 'update', 'activate', 'delete', 'retire', 'make_available', 'transfer_return', 'save_allocation', 'order_assign', 'receive', 'inspect', 'repair_ship', 'unknown'])
    await command(`supplier-default-deny-${action}`, { action }, {}, true)
  await command('non-supplier-command-bypasses-scope-not-action-auth', { action: 'unknown' }, { metadata: {} })
  for (const [method, url] of [['GET', '/admin/devices'], ['POST', '/admin/devices'], ['GET', '/admin/devices/variants'], ['GET', '/admin/warehouse-access/me'], ['GET', '/admin/feature-flags'], ['GET', '/admin/plugins'], ['GET', '/admin/layouts/orders'], ['GET', '/admin/devices/?limit=20']])
    await middleware(`allowlist-${method}-${url}`, method, url)
  for (const [method, url] of [['GET', '/admin/orders'], ['GET', '/admin/customers'], ['GET', '/admin/products'], ['GET', '/admin/devices/photos'], ['POST', '/admin/devices/photos'], ['POST', '/admin/devices/variants'], ['GET', '/admin/devices/supplier-accounts'], ['GET', '/admin/rbac/roles'], ['GET', '/admin/mail'], ['POST', '/admin/stores'], ['GET', '/admin/devices-other']])
    await middleware(`denylist-${method}-${url}`, method, url, {}, 'denied')
  await middleware('own-name-only-update-allowed', 'POST', '/admin/users/staff-a', { body: { first_name: 'Fixture', last_name: 'Supplier' } })
  await middleware('own-name-and-metadata-update-denied', 'POST', '/admin/users/staff-a', { body: { first_name: 'Fixture', metadata: { device_role: 'admin' } } }, 'denied')
  await middleware('other-staff-name-update-denied', 'POST', '/admin/users/staff-b', { body: { first_name: 'Fixture' } }, 'denied')
  await middleware('bootstrap-me-no-password-projection', 'GET', '/admin/users/me', {}, 'response', (calls, response) => {
    assert.equal(response.user.id, 'staff-a'); assert(!Object.hasOwn(response.user, 'password_hash'))
    assert.deepEqual(response.user.metadata, supplierMetadata)
  })
  await middleware('bootstrap-permissions-empty', 'GET', '/admin/rbac/me/permissions', {}, 'response', (calls, response) => assert.deepEqual(response, { permissions: [] }))
  await middleware('bootstrap-notifications-empty', 'GET', '/admin/notifications', {}, 'response', (calls, response) => assert.deepEqual(response, { notifications: [], count: 0, offset: 0, limit: 1 }))
  await middleware('bootstrap-store-only-id-name', 'GET', '/admin/stores', {}, 'response', (calls, response) => {
    assert.deepEqual(response, { stores: [{ id: 'store-fixture', name: 'Fixture Shop' }], count: 1, offset: 0, limit: 1 })
    assert.deepEqual(calls.find(c => c.stores).config, { take: 1 })
  })
  await middleware('missing-actor-delegates-not-authenticated-proof', 'GET', '/admin/orders', { auth: undefined }, 'next', calls => assert.deepEqual(calls, []))
  await middleware('api-key-delegates-not-new-admin-contract', 'GET', '/admin/orders', { auth: { actor_id: 'key-fixture', actor_type: 'api-key' } }, 'next', calls => assert.deepEqual(calls, []))
  await middleware('ordinary-staff-delegates-to-other-auth', 'GET', '/admin/orders', { metadata: {} })
  const report = { format_version: 1,
    scope: 'Actual legacy deviceAccess, authorizeDeviceCommand and guardSupplierAdmin with artificial objects/ports; not authentication, native RBAC, SQL, device workflows or Gin acceptance.',
    source_sha256: { [source]: crypto.createHash('sha256').update(bytes).digest('hex') }, cases,
    observations: ['Old secret keys and ordinary staff get admin scope, which is not proof of action permissions',
      'Missing actor/API-key middleware branches delegate and depend on later authentication',
      'supplier_assign passes scope checks but its actual workflow still rejects DEVICE_INTAKE_REQUIRED',
      'fulfill checks result count and every source/order, relying on service filter for exact assignment identity',
      'Supplier metadata order list retains duplicate/empty strings; target binding rejects invalid identities',
      'Own user bootstrap projects metadata; target requires a dedicated minimal DTO and explicit new bindings'],
    remaining: ['Supplier query/field projections and workflow state/transaction validation', 'Gin permissions with no inherited old credentials/grants', 'Native RBAC and HTTP/SQL integration'] }
  const content = JSON.stringify(report, null, 2) + '\n'
  const output = path.join(root, 'docs/migration/fixtures/legacy-supplier-access.json')
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(output, 'utf8').replaceAll('\r\n', '\n'), content)
  else fs.writeFileSync(output, content)
  console.log(JSON.stringify({ cases: cases.length, source_files: 1, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
}
main().catch(error => { console.error(error.message); process.exitCode = 1 })
