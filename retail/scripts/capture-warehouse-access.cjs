// Execute the actual legacy warehouse library with artificial data and ports.
// No app bootstrap, source database, network or business writes.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const source = 'medusa-store/apps/backend/src/lib/warehouse-access.ts'
const bytes = fs.readFileSync(path.join(root, source))
const ts = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))('typescript')
const clone = value => JSON.parse(JSON.stringify(value))
class AccessError extends Error {
  static Types = { FORBIDDEN: 'forbidden' }
  constructor(type, message) { super(message); this.type = type }
}
const permissionCalls = []
const moduleRecord = { exports: {} }
function requirePort(name) {
  if (name === '@medusajs/framework') return { hasPermission: async input => {
    permissionCalls.push({ roles: clone(input.roles), actions: clone(input.actions) })
    return input.container.permission === true
  } }
  if (name === '@medusajs/framework/utils') return { MedusaError: AccessError,
    ContainerRegistrationKeys: { QUERY: 'query' }, Modules: { USER: 'user', FULFILLMENT: 'fulfillment' } }
  if (name === '../modules/devices') return { DEVICES_MODULE: 'devices' }
  throw new Error('Unreviewed dependency: ' + name)
}
const js = ts.transpileModule(bytes.toString('utf8'), { compilerOptions: {
  target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS,
} }).outputText
vm.runInNewContext(`(function(require,module,exports){${js}\n})`, {},
  { filename: source, timeout: 3000 })(requirePort, moduleRecord, moduleRecord.exports)
const legacy = moduleRecord.exports
const commandSource = 'medusa-store/apps/backend/src/api/warehouse-guards.ts'
const commandBytes = fs.readFileSync(path.join(root, commandSource))
const commandRecord = { exports: {} }
const commandJS = ts.transpileModule(commandBytes.toString('utf8'), { compilerOptions: {
  target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS,
} }).outputText
vm.runInNewContext(`(function(require,module,exports){${commandJS}\n})`, {},
  { filename: commandSource, timeout: 3000 })(name => name === '../lib/warehouse-access' ? legacy : requirePort(name), commandRecord, commandRecord.exports)
const cases = []
async function check(name, run) { await run(); cases.push({ name, passed: true }) }
async function denied(run) { await assert.rejects(run, error => error.type === 'forbidden' && error.message === 'WAREHOUSE_ACCESS_DENIED') }
const scoped = { warehouse: true, location_ids: ['warehouse-a'] }
const all = { warehouse: true, location_ids: null }
const empty = { warehouse: true, location_ids: [] }
function ports(options = {}) {
  const calls = []
  const container = { permission: options.permission, resolve(name) {
    calls.push({ resolve: name })
    if (name === 'user') return { listUsers: async filter => {
      calls.push({ listUsers: clone(filter) }); return options.missingUser ? [] : [{ metadata: options.metadata || {} }]
    } }
    if (name === 'query') return { graph: async input => {
      calls.push({ graph: clone(input) })
      if (input.entity === 'user') return { data: [{ rbac_roles: (options.roles || []).map(id => ({ id })) }] }
      if (input.entity === 'order_fulfillment') return { data: options.links || [] }
      if (input.entity === 'order') return { data: options.order ? [options.order] : [] }
      throw new Error('Unreviewed query entity')
    } }
    if (name === 'devices') return {
      retrieveDeviceSource: async id => { calls.push({ source: id }); return options.source || { location_id: 'warehouse-a', type: 'self' } },
      retrieveDevice: async id => { calls.push({ device: id }); return options.devices?.[id] || { id, location_id: 'warehouse-a', category: 'sale' } },
      retrieveDeviceAssignment: async id => { calls.push({ assignment: id }); return options.assignment || { device_id: 'device-a', order_id: 'order-fixture', fulfillment_id: 'parcel-a' } },
    }
    if (name === 'fulfillment') return {
      listFulfillments: async (filter, config) => { calls.push({ fulfillments: clone(filter), config: clone(config) }); return options.fulfillments || [] },
      retrieveFulfillment: async (id, config) => { calls.push({ fulfillment: id, config: clone(config) }); return options.fulfillment || { id, location_id: 'warehouse-a', items: [] } },
    }
    throw new Error('Unreviewed resolve: ' + name)
  } }
  return { container, calls }
}
const variant = () => ({ id: 'variant-fixture', inventory_quantity: 999, internal_note: 'artificial-private',
  inventory_items: [{ required_quantity: 2, inventory: { location_levels: [null,
    { location_id: 'warehouse-a', stocked_quantity: '12', reserved_quantity: '2' },
    { location_id: 'warehouse-b', stocked_quantity: '90', reserved_quantity: '0' }], internal_note: 'artificial-inventory-private' } },
  { required_quantity: 3, inventory: { location_levels: [{ location_id: 'warehouse-a', stocked_quantity: '9', reserved_quantity: '3' }] } }] })
const order = () => ({ id: 'order-fixture', total: 160, subtotal: 140, tax_total: 20, metadata: { private: true },
  summary: { paid: 160 }, credit_lines: [1], returns: [1], claims: [1], exchanges: [1], promotions: [1], shipping_methods: [1],
  internal_unrecognized_field: 'artificial-private',
  customer: { id: 'customer-fixture', email: 'fixture@example.invalid', first_name: 'Fixture', metadata: { private: true } },
  payment_collections: [{ id: 'payment-private', amount: 160, status: 'completed', payments: [{ private: true }] }],
  fulfillments: [
    { id: 'parcel-a', location_id: 'warehouse-a', items: [{ line_item_id: 'line-a', quantity: 2 }] },
    { id: 'parcel-a-canceled', location_id: 'warehouse-a', canceled_at: '2026-01-01', items: [{ line_item_id: 'line-b', quantity: 1 }] },
    { id: 'parcel-b', location_id: 'warehouse-b', items: [{ line_item_id: 'line-a', quantity: 2 }, { line_item_id: 'line-b', quantity: 1 }] }],
  items: [{ id: 'line-a', quantity: 4, total: 120, subtotal: 100, tax_total: 20, detail: { quantity: 4, other: 'private-detail' }, variant: variant(), metadata: { private: true } },
    { id: 'line-b', quantity: 2, total: 40, subtotal: 40, tax_total: 0 }],
})
async function main() {
  const accessCases = [
    ['missing-user-legacy-nonwarehouse-not-auth-proof', { missingUser: true }, { warehouse: false, location_ids: null }],
    ['ordinary-user-nonwarehouse', {}, { warehouse: false, location_ids: null }],
    ['role-without-bindings-empty-scope', { roles: ['role_retail_warehouse'] }, empty],
    ['metadata-role-without-bindings-empty-scope', { metadata: { device_role: 'warehouse' } }, empty],
    ['empty-location-array-enables-warehouse', { metadata: { warehouse_location_ids: [] } }, empty],
    ['warehouse-all-explicit', { metadata: { device_role: 'warehouse', warehouse_scope: 'all' } }, all],
    ['scope-all-alone-does-not-enable-warehouse', { metadata: { warehouse_scope: 'all' } }, { warehouse: false, location_ids: null }],
    ['bindings-filter-nonstrings-and-empty-dedupe', { metadata: { warehouse_location_ids: ['warehouse-a', '', null, 1, 'warehouse-a'] } }, scoped],
    ['legacy-whitespace-location-not-trimmed', { metadata: { warehouse_location_ids: [' '] } }, { warehouse: true, location_ids: [' '] }],
  ]
  for (const [name, options, expected] of accessCases) await check(name, async () => {
    const p = ports(options)
    assert.deepEqual(clone(await legacy.warehouseAccess(p.container, 'actor-fixture')), expected)
    assert.deepEqual(p.calls.find(c => c.listUsers).listUsers, { id: 'actor-fixture' })
    if (options.missingUser) assert(!p.calls.some(c => c.graph))
  })
  await check('own-location-allowed', () => legacy.assertWarehouseLocation(scoped, 'warehouse-a'))
  await check('cross-location-denied', () => denied(async () => legacy.assertWarehouseLocation(scoped, 'warehouse-b')))
  await check('empty-scope-denies-location', () => denied(async () => legacy.assertWarehouseLocation(empty, 'warehouse-a')))
  await check('all-scope-allows-location', () => legacy.assertWarehouseLocation(all, 'warehouse-b'))
  for (const [name, sourceObject, reject] of [
    ['own-self-source', { location_id: 'warehouse-a', type: 'self' }, false],
    ['cross-self-source', { location_id: 'warehouse-b', type: 'self' }, true],
    ['own-supplier-source', { location_id: 'warehouse-a', type: 'supplier' }, true],
  ]) await check(name, async () => {
    const p = ports({ source: sourceObject })
    if (reject) await denied(() => legacy.assertWarehouseSource(p.container, scoped, 'source-fixture'))
    else assert.deepEqual(clone(await legacy.assertWarehouseSource(p.container, scoped, 'source-fixture')), sourceObject)
  })
  for (const [category, expected] of [['new', 'warehouse_intake'], ['returned', 'warehouse_return'], [undefined, 'warehouse_intake']])
    await check(`device-resource-${category}`, () => assert.equal(legacy.warehouseDeviceResource(category), expected))
  await check('recycled-device-category-denied', () => denied(async () => legacy.warehouseDeviceResource('recycled')))
  for (const [roles, permission, allowed] of [[[], true, false], [['role-fixture'], false, false], [['role-fixture'], true, true]])
    await check(`rbac-${roles.length}-${permission}`, async () => {
      permissionCalls.length = 0
      const p = ports({ roles, permission })
      assert.equal(await legacy.hasWarehousePermission(p.container, 'actor-fixture', 'warehouse_intake', 'update'), allowed)
      assert.equal(permissionCalls.length, roles.length ? 1 : 0)
      if (roles.length) assert.deepEqual(permissionCalls[0], { roles, actions: { resource: 'warehouse_intake', operation: 'update' } })
      if (!allowed) await denied(() => legacy.assertWarehousePermission(p.container, 'actor-fixture', 'warehouse_intake', 'update'))
    })
  await check('inventory-only-own-levels-recompute-required-quantity-minimum', () => {
    const result = legacy.scopeInventoryLevels(variant(), scoped)
    assert.equal(result.inventory_quantity, 2)
    assert.equal(result.inventory_items[0].inventory.stocked_quantity, 12)
    assert.equal(result.inventory_items[0].inventory.reserved_quantity, 2)
    assert.deepEqual(clone(result.inventory_items[0].inventory.location_levels).map(v => v.location_id), ['warehouse-a'])
  })
  await check('inventory-empty-scope-zero-stock', () => assert.equal(legacy.scopeInventoryLevels(variant(), empty).inventory_quantity, 0))
  await check('inventory-no-items-is-undefined-not-zero', () => assert.equal(legacy.scopeInventoryLevels({}, scoped).inventory_quantity, undefined))
  await check('inventory-full-scope-bypasses-projection', () => { const v = variant(); assert.equal(legacy.scopeInventoryLevels(v, all), v) })
  await check('legacy-inventory-unknown-fields-remain', () => {
    const v = legacy.scopeInventoryLevels(variant(), scoped)
    assert.equal(v.internal_note, 'artificial-private'); assert.equal(v.inventory_items[0].inventory.internal_note, 'artificial-inventory-private')
  })
  await check('order-filter-items-scale-totals-strip-known-private-fields', () => {
    const original = order(), before = clone(original)
    const result = legacy.scopeWarehouseOrder(original, scoped)
    assert.deepEqual(clone(result.items).map(i => i.id), ['line-a'])
    assert.equal(result.items[0].quantity, 2); assert.equal(result.items[0].total, 60)
    assert.equal(result.total, 60); assert.equal(result.subtotal, 140); assert.equal(result.tax_total, 10)
    assert.deepEqual(clone(result.items[0].detail), { quantity: 2, fulfilled_quantity: 2 })
    assert.equal(result.items[0].variant.inventory_quantity, 2)
    assert.deepEqual(clone(result.payment_collections), [{ status: 'completed' }])
    for (const field of ['credit_lines', 'returns', 'claims', 'exchanges', 'promotions', 'shipping_methods']) assert.deepEqual(clone(result[field]), [])
    assert.deepEqual(clone(result.metadata), {}); assert.equal(result.summary, undefined)
    assert(!Object.hasOwn(result.customer, 'metadata')); assert.equal(result.customer.email, 'fixture@example.invalid')
    assert.deepEqual(original, before)
  })
  await check('legacy-canceled-own-fulfillment-retained-but-not-counted', () => {
    const result = legacy.scopeWarehouseOrder(order(), scoped)
    assert.deepEqual(clone(result.fulfillments).map(f => f.id), ['parcel-a', 'parcel-a-canceled'])
    assert.equal(result.items.length, 1)
  })
  await check('legacy-order-unknown-and-line-metadata-fields-remain', () => {
    const result = legacy.scopeWarehouseOrder(order(), scoped)
    assert.equal(result.internal_unrecognized_field, 'artificial-private')
    assert.deepEqual(clone(result.items[0].metadata), { private: true })
  })
  await check('legacy-bare-subtotal-not-matched-by-underscore-suffix', () => {
    const result = legacy.scopeWarehouseOrder(order(), scoped)
    assert.equal(result.subtotal, 140); assert.equal(result.items[0].subtotal, 100)
    assert.equal(result.total, 60)
  })
  await check('order-empty-scope-no-items-zero-totals', () => {
    const result = legacy.scopeWarehouseOrder(order(), empty)
    assert.equal(result.items.length, 0); assert.equal(result.fulfillments.length, 0); assert.equal(result.total, 0)
  })
  await check('order-full-scope-bypasses-projection-not-payment-permission', () => { const o = order(); assert.equal(legacy.scopeWarehouseOrder(o, all), o) })
  await check('legacy-ratio-retains-floating-point-tail', () => {
    const o = order(); o.items[0].total = 0.1; o.items[0].quantity = 3
    assert.equal(legacy.scopeWarehouseOrder(o, scoped).items[0].total, 0.06666666666666667)
  })
  await check('order-same-item-multiple-own-parcels-sums-quantities', () => {
    const o = order(); o.fulfillments.push({ id: 'parcel-a2', location_id: 'warehouse-a', items: [{ line_item_id: 'line-a', quantity: 1 }] })
    assert.equal(legacy.scopeWarehouseOrder(o, scoped).items[0].quantity, 3)
  })
  for (const [name, access, expected] of [['scoped', scoped, { location_id: ['warehouse-a'] }], ['empty', empty, { location_id: ['__no_warehouse__'] }], ['all', all, {}]])
    await check(`order-query-${name}-scope-and-canceled-filter`, async () => {
      const p = ports({ fulfillments: [{ id: 'active', location_id: 'warehouse-a' }, { id: 'canceled', canceled_at: '2026-01-01' }], links: [{ order_id: 'order-fixture', fulfillment_id: 'active' }] })
      const result = await legacy.warehouseOrders(p.container, access)
      assert.deepEqual(p.calls.find(c => c.fulfillments).fulfillments, expected)
      assert.deepEqual(p.calls.find(c => c.fulfillments).config, { take: null, relations: ['items'] })
      assert.deepEqual(p.calls.find(c => c.graph).graph.filters, { fulfillment_id: ['active'] })
      assert.equal(result.length, 1)
    })
  await check('all-canceled-parcels-skip-order-link-query', async () => {
    const p = ports({ fulfillments: [{ id: 'canceled', canceled_at: '2026-01-01' }] })
    assert.deepEqual(clone(await legacy.warehouseOrders(p.container, scoped)), [])
    assert(!p.calls.some(c => c.graph))
  })
  await check('fulfillment-must-belong-to-path-order', () => denied(() => legacy.assertWarehouseFulfillment(ports({ order: { fulfillments: [{ id: 'different' }] } }).container, scoped, 'parcel-a', 'order-fixture')))
  await check('fulfillment-location-checked-before-order-lookup', async () => {
    const p = ports({ fulfillment: { id: 'parcel-b', location_id: 'warehouse-b' } })
    await denied(() => legacy.assertWarehouseFulfillment(p.container, scoped, 'parcel-b', 'order-fixture'))
    assert(!p.calls.some(c => c.graph))
  })
  await check('own-fulfillment-and-order-allowed', async () => {
    const p = ports({ order: { fulfillments: [{ id: 'parcel-a' }] } })
    assert.equal((await legacy.assertWarehouseFulfillment(p.container, scoped, 'parcel-a', 'order-fixture')).id, 'parcel-a')
  })
  await check('order-without-own-active-parcel-denied', () => denied(() => legacy.assertWarehouseOrder(ports().container, scoped, 'order-fixture')))
  const warehouseOptions = { metadata: { device_role: 'warehouse', warehouse_location_ids: ['warehouse-a'] },
    roles: ['role_retail_warehouse'], permission: true,
    order: { fulfillments: [{ id: 'parcel-a' }] },
    fulfillment: { id: 'parcel-a', location_id: 'warehouse-a', items: [{ line_item_id: 'line-a' }] },
    fulfillments: [{ id: 'parcel-a', location_id: 'warehouse-a', items: [{ line_item_id: 'line-a' }] }],
    links: [{ order_id: 'order-fixture', fulfillment_id: 'parcel-a' }] }
  async function command(name, body, extra = {}, reject = false, verify) {
    await check(name, async () => {
      const p = ports({ ...warehouseOptions, ...extra })
      permissionCalls.length = 0
      const run = () => commandRecord.exports.authorizeWarehouseCommand(p.container, 'actor-fixture', body)
      if (reject) await denied(run); else await run()
      if (verify) verify(p.calls, permissionCalls)
    })
  }
  await command('command-nonwarehouse-bypasses-guard-not-authorization', { action: 'arbitrary' }, { metadata: {}, roles: [] }, false, calls => assert(!calls.some(c => c.resolve === 'devices')))
  for (const action of ['activate', 'delete', 'retire', 'inspect']) {
    await command(`command-${action}-own-device`, { action, device_id: 'device-a' })
    await command(`command-${action}-cross-device-denied`, { action, device_id: 'device-b' }, { devices: { 'device-b': { location_id: 'warehouse-b', category: 'sale' } } }, true)
  }
  await command('command-intake-every-device-source-checked', { action: 'intake', devices: [{ category: 'sale', source_id: 'source-a' }, { category: 'returned', source_id: 'source-b' }] }, {}, false,
    (calls, permissions) => { assert.deepEqual(calls.filter(c => c.source).map(c => c.source), ['source-a', 'source-b']); assert.deepEqual(permissions.map(c => c.actions.resource), ['warehouse_intake', 'warehouse_return']) })
  await command('command-intake-supplier-source-denied', { action: 'intake', devices: [{ category: 'sale', source_id: 'source-a' }] }, { source: { location_id: 'warehouse-a', type: 'supplier' } }, true)
  await command('command-intake-recycled-denied', { action: 'intake', devices: [{ category: 'recycled', source_id: 'source-a' }] }, {}, true)
  await command('command-make-available-second-device-cross-scope-denied', { action: 'make_available', device_ids: ['device-a', 'device-b'] }, { devices: { 'device-b': { location_id: 'warehouse-b', category: 'sale' } } }, true,
    calls => assert.deepEqual(calls.filter(c => c.device).map(c => c.device), ['device-a', 'device-b']))
  await command('command-update-new-location-denied', { action: 'update', device_id: 'device-a', category: 'sale', location_id: 'warehouse-b', source_id: 'source-a' }, {}, true)
  await command('command-update-requires-both-old-and-new-category-permission', { action: 'update', device_id: 'device-a', category: 'returned', location_id: 'warehouse-a', source_id: 'source-a' }, {}, false,
    (calls, permissions) => assert.deepEqual(permissions.map(c => c.actions.resource), ['warehouse_intake', 'warehouse_return']))
  await command('command-transfer-return-requires-both-resources', { action: 'transfer_return', device_id: 'device-a' }, {}, false,
    (calls, permissions) => assert.deepEqual(permissions.map(c => c.actions.resource), ['warehouse_intake', 'warehouse_return']))
  for (const action of ['assign', 'order_assign', 'unknown']) await command(`command-${action}-denied`, { action }, {}, true)
  await command('command-release-checks-assignment-device-and-order', { action: 'release', assignment_id: 'assignment-a' })
  await command('command-release-other-order-denied', { action: 'release', assignment_id: 'assignment-a' }, { order: { fulfillments: [{ id: 'another-parcel' }] } }, true)
  await command('command-receive-supplier-destination-denied', { action: 'receive', assignment_id: 'assignment-a', source_id: 'source-a' }, { source: { location_id: 'warehouse-a', type: 'supplier' } }, true)
  await command('command-ship-no-action-permission-denied-before-device-read', { action: 'ship', fulfillment_id: 'parcel-a' }, { permission: false }, true,
    calls => assert(!calls.some(c => c.resolve === 'devices')))
  await command('command-ship-own-parcel-allowed', { action: 'ship', fulfillment_id: 'parcel-a' })
  const allocation = { fulfillment_id: 'parcel-a', device_id: 'device-a', item_id: 'line-a' }
  await command('command-save-allocation-own-parcel-and-line', { action: 'save_allocation', order_id: 'order-fixture', allocations: [allocation] })
  await command('command-save-allocation-requires-parcel', { action: 'save_allocation', order_id: 'order-fixture', allocations: [{ ...allocation, fulfillment_id: undefined }] }, {}, true)
  await command('command-save-allocation-rejects-other-line', { action: 'save_allocation', order_id: 'order-fixture', allocations: [{ ...allocation, item_id: 'line-b' }] }, {}, true)
  await command('command-save-allocation-second-cross-device-denied', { action: 'save_allocation', order_id: 'order-fixture', allocations: [allocation, { ...allocation, device_id: 'device-b' }] }, { devices: { 'device-b': { location_id: 'warehouse-b', category: 'sale' } } }, true)
  const report = { format_version: 1, scope: 'Actual legacy warehouse library and authorizeWarehouseCommand with artificial objects and explicit query/permission ports; not native RBAC engine, HTTP routing, SQL, full workflows or new Gin acceptance.',
    source_sha256: { [source]: crypto.createHash('sha256').update(bytes).digest('hex'), [commandSource]: crypto.createHash('sha256').update(commandBytes).digest('hex') }, cases,
    observed_legacy_gaps: ['Missing staff returns nonwarehouse scope; caller authentication is still required',
      'Full scope returns original object; scope alone is not action or field authorization',
      'Object spreads retain unknown order/line/inventory fields; target needs allowlisted DTOs',
      'Bare subtotal is not matched by _subtotal suffix and remains unscoped at order and line levels',
      'Canceled own fulfillments remain in projected payload while excluded from quantities',
      'Proportional money uses Number and can retain fractional floating-point tails'],
    remaining: ['Actual route/command authorization and every bulk member', 'Gin exact decimal projection, permissions and allowlisted DTOs', 'Full native RBAC and HTTP/DB integration'] }
  const content = JSON.stringify(report, null, 2) + '\n'
  const output = path.join(root, 'docs/migration/fixtures/legacy-warehouse-access.json')
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(output, 'utf8').replaceAll('\r\n', '\n'), content)
  else fs.writeFileSync(output, content)
  console.log(JSON.stringify({ cases: cases.length, source_files: 2, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
}
main().catch(error => { console.error(error.message); process.exitCode = 1 })
