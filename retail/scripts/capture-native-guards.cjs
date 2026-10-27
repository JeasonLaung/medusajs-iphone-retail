// Read actual middleware mounts and execute only pure guard modules with fake ports.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const base = 'medusa-store/apps/backend/src/api/'
const backend = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const ts = backend('typescript')
const clone = value => JSON.parse(JSON.stringify(value))
const hashes = {}
function read(name) {
  const source = base + name
  const bytes = fs.readFileSync(path.join(root, source))
  hashes[source] = crypto.createHash('sha256').update(bytes).digest('hex')
  return bytes.toString('utf8')
}
const guards = new Set(['guardDeviceOrder', 'guardDeviceInventory', 'guardDeviceReturn',
  'guardManualPaymentSelection', 'guardMarkAsPaid', 'guardManualCartCompletion',
  'guardManualPaymentAuthorization', 'guardManualPaymentCapture', 'listProductionPaymentProviders'])
function unwrap(node) {
  while (ts.isAsExpression(node) || ts.isParenthesizedExpression(node)) node = node.expression
  return node
}
function property(node, name) {
  assert(ts.isObjectLiteralExpression(node))
  const found = node.properties.find(p => ts.isPropertyAssignment(p) && p.name.getText().replaceAll('"', '') === name)
  return found?.initializer
}
function literal(node) { node = unwrap(node); assert(ts.isStringLiteral(node), 'Unreviewed nonliteral mount'); return node.text }
const mountAst = ts.createSourceFile(base + 'middlewares.ts', read('middlewares.ts'), ts.ScriptTarget.Latest, true)
const declaration = mountAst.statements.find(s => ts.isExportAssignment(s))
assert(declaration && ts.isCallExpression(declaration.expression) && declaration.expression.expression.getText(mountAst) === 'defineMiddlewares')
const routes = property(declaration.expression.arguments[0], 'routes')
assert(ts.isArrayLiteralExpression(routes))
const mounts = []
function inspect(node, boundMatcher) {
  node = unwrap(node)
  if (ts.isSpreadElement(node)) {
    const call = unwrap(node.expression)
    assert(ts.isCallExpression(call) && ts.isPropertyAccessExpression(call.expression) && call.expression.name.text === 'map')
    const values = unwrap(call.expression.expression)
    const fn = call.arguments[0]
    assert(ts.isArrayLiteralExpression(values) && ts.isArrowFunction(fn) && fn.parameters[0].name.getText() === 'matcher')
    for (const value of values.elements) inspect(fn.body, literal(value))
    return
  }
  const middlewares = property(node, 'middlewares')
  if (!middlewares) return
  assert(ts.isArrayLiteralExpression(middlewares))
  const selected = middlewares.elements.filter(e => ts.isIdentifier(e) && guards.has(e.text)).map(e => e.text)
  if (!selected.length) return
  const matcher = property(node, 'matcher') || node.properties.find(p => ts.isShorthandPropertyAssignment(p) && p.name.text === 'matcher')?.name
  const pattern = ts.isIdentifier(matcher) && matcher.text === 'matcher' ? boundMatcher : literal(matcher)
  assert(pattern, 'Unresolved matcher')
  const rawMethods = unwrap(property(node, 'method'))
  const methods = ts.isArrayLiteralExpression(rawMethods) ? rawMethods.elements.map(literal) : [literal(rawMethods)]
  for (const method of methods) for (const guard of selected) mounts.push({ method, path: pattern, guard,
    source: base + 'middlewares.ts', line: mountAst.getLineAndCharacterOfPosition(node.getStart()).line + 1 })
}
routes.elements.forEach(node => inspect(node))
assert.deepEqual([...new Set(mounts.map(m => m.guard))].sort(), [...guards].sort())
assert.equal(new Set(mounts.map(m => `${m.method} ${m.path} ${m.guard}`)).size, mounts.length)
// A middleware registration is not evidence that a native route exists.
// Inspect installed compiled files without requiring them or their workflows.
const packageRoot = 'medusa-store/apps/backend/node_modules/@medusajs/medusa/'
const packageBytes = fs.readFileSync(path.join(root, packageRoot, 'package.json'))
hashes[packageRoot + 'package.json'] = crypto.createHash('sha256').update(packageBytes).digest('hex')
for (const mount of mounts) {
  const segment = mount.path.replace(/:([A-Za-z_]+)/g, '[$1]')
  const custom = 'medusa-store/apps/backend/src/api' + segment + '/route.ts'
  const native = packageRoot + 'dist/api' + segment + '/route.js'
  const customExists = fs.existsSync(path.join(root, custom))
  const nativeExists = fs.existsSync(path.join(root, native))
  mount.handler_evidence = { custom_file: customExists ? custom : null, native_file: nativeExists ? native : null,
    custom_method_exported: false, native_method_exported: false, status: 'no-exact-handler-file-found-not-a-removal-decision' }
  for (const file of [customExists ? custom : null, nativeExists ? native : null].filter(Boolean)) {
    const bytes = fs.readFileSync(path.join(root, file))
    hashes[file] = crypto.createHash('sha256').update(bytes).digest('hex')
    if (file === custom) {
      const tree = ts.createSourceFile(file, bytes.toString('utf8'), ts.ScriptTarget.Latest, true)
      mount.handler_evidence.custom_method_exported = tree.statements.some(statement =>
        statement.modifiers?.some(modifier => modifier.kind === ts.SyntaxKind.ExportKeyword) &&
        (ts.isFunctionDeclaration(statement) && statement.name?.text === mount.method ||
         ts.isVariableStatement(statement) && statement.declarationList.declarations.some(d => ts.isIdentifier(d.name) && d.name.text === mount.method)))
    }
    if (file === native) {
      const tree = ts.createSourceFile(file, bytes.toString('utf8'), ts.ScriptTarget.Latest, true, ts.ScriptKind.JS)
      let exported = false
      function find(node) {
        if (ts.isBinaryExpression(node) && node.operatorToken.kind === ts.SyntaxKind.EqualsToken &&
          ts.isPropertyAccessExpression(node.left) && node.left.expression.getText(tree) === 'exports' &&
          node.left.name.text === mount.method && ts.isIdentifier(node.right) && node.right.text === mount.method) exported = true
        ts.forEachChild(node, find)
      }
      find(tree)
      mount.handler_evidence.native_method_exported = exported
    }
  }
  if (mount.handler_evidence.custom_method_exported || mount.handler_evidence.native_method_exported) mount.handler_evidence.status = 'handler-source-found-runtime-not-proven'
  else if (nativeExists) mount.handler_evidence.status = 'native-file-found-method-not-exported'
}
const followups = [
  ['POST', '/admin/order-edits', 'beginOrderEditOrderWorkflow', 'Actual edit creation uses body order_id, not /orders/:id/edits; bind order and device guards here'],
  ['POST', '/admin/order-edits/:id/request', 'requestOrderEditRequestWorkflow', 'Resolve change/order before requesting customer confirmation; exact workflow export reviewed separately'],
  ['POST', '/admin/order-edits/:id/confirm', 'confirmOrderEditRequestWorkflow', 'Resolve change/order and recheck state and devices at commit, not just draft creation'],
  ['POST', '/admin/exchanges/:id/request', 'confirmExchangeRequestWorkflow', 'Actual exchange confirmation has no /confirm suffix; guard exact route and nested order'],
  ['POST', '/admin/inventory-items/:id/location-levels/batch', 'batchInventoryItemLevelsWorkflow', 'Actual batch delete is validatedBody.delete on POST; scope every member and force semantics'],
  ['DELETE', '/admin/inventory-items/:id/location-levels/:location_id', 'deleteInventoryLevelsWorkflow', 'Actual single-location delete path includes location_id; verify reservation and history behavior'],
].map(([method, pattern, workflow, requirement]) => {
  const file = packageRoot + 'dist/api' + pattern.replace(/:([A-Za-z_]+)/g, '[$1]') + '/route.js'
  const bytes = fs.readFileSync(path.join(root, file))
  hashes[file] = crypto.createHash('sha256').update(bytes).digest('hex')
  const text = bytes.toString('utf8')
  assert(text.includes(`exports.${method} = ${method}`), 'Reviewed native method export disappeared')
  // Workflow names are exact observed calls, not inferred from route names.
  assert(text.includes(`.${workflow})`), 'Reviewed native workflow changed')
  return { method, path: pattern, source: file, workflow, migration_requirement: requirement,
    status: 'installed-source-only-not-runtime-or-target-schema-acceptance' }
})
for (const relative of ['order-edits/middlewares.js', 'exchanges/middlewares.js']) {
  const file = packageRoot + 'dist/api/admin/' + relative
  hashes[file] = crypto.createHash('sha256').update(fs.readFileSync(path.join(root, file))).digest('hex')
}
class GuardError extends Error { static Types = { NOT_ALLOWED: 'not_allowed' }; constructor(type, message) { super(message); this.type = type } }
const moduleText = { device: read('device-guards.ts'), payment: read('payment-guards.ts') }
function load(kind, mode) {
  const module = { exports: {} }
  const requirePure = name => {
    if (name === '@medusajs/framework/utils') return { MedusaError: GuardError,
      ContainerRegistrationKeys: { QUERY: 'query', REMOTE_QUERY: 'remote' }, remoteQueryObjectFromString: value => value }
    if (kind === 'device' && name === '../modules/devices') return { DEVICES_MODULE: 'devices' }
    throw new Error('Unreviewed dependency: ' + name)
  }
  const js = ts.transpileModule(moduleText[kind], { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText
  vm.runInNewContext(`(function(require,module,exports){${js}\n})`, { process: { env: Object.freeze({ NODE_ENV: mode }) } },
    { filename: base + `${kind}-guards.ts`, timeout: 3000 })(requirePure, module, module.exports)
  return module.exports
}
const cases = []
async function check(name, guard, options, denied, verify) {
  const calls = [], queries = []
  let next = 0, response, failure
  const devices = {
    async listDeviceVariants(filter) { calls.push('variants'); queries.push(clone(filter)); return options.managed ? [{ variant_id: 'variant-fixture' }] : [] },
    async listDeviceAssignments(filter) { calls.push('assignments'); queries.push(clone(filter)); return options.assigned ? [{ id: 'assignment-fixture' }] : [] },
  }
  const query = { async graph(spec) { calls.push('query'); queries.push(clone(spec)); return { data: options.data || [] } } }
  const remote = async spec => { calls.push('remote'); queries.push(clone(spec)); return {
    rows: options.remoteRows || [], metadata: { count: 2, skip: 1, take: 20 },
  } }
  const request = {
    path: options.path || '/admin/orders/order-fixture/edits', params: options.params || { id: 'order-fixture' },
    body: options.body || {}, filterableFields: options.filters || {},
    queryConfig: { fields: ['id'], pagination: { skip: 1, take: 20 } },
    scope: { resolve(key) { assert(['devices', 'query', 'remote'].includes(key)); return { devices, query, remote }[key] } },
  }
  const mod = load(guard.startsWith('guardDevice') ? 'device' : 'payment', options.mode || 'production')
  try { await mod[guard](request, { json(value) { response = clone(value) } }, () => { next++ }) }
  catch (error) { failure = { type: error.type, message: error.message } }
  assert.equal(Boolean(failure), denied, name)
  if (denied) assert.equal(failure.type, 'not_allowed', name)
  assert.equal(next, denied || guard === 'listProductionPaymentProviders' && (options.mode || 'production') === 'production' ? 0 : 1, name)
  verify?.({ calls, queries, response, failure })
  cases.push({ name, guard, artificial_input: options, outcome: failure || { next, ...(response ? { response } : {}) },
    observed_port_calls: calls, independent_assertions_passed: true })
}
async function main() {
  const order = [{ id: 'order-fixture', items: [{ variant_id: 'variant-fixture' }] }]
  for (const path of ['/admin/orders/order-fixture/fulfillments', '/admin/orders/order-fixture/edits']) {
    await check(`${path}-ordinary-allowed`, 'guardDeviceOrder', { path, data: order }, false)
    await check(`${path}-managed-rejected`, 'guardDeviceOrder', { path, data: order, managed: true }, true)
    await check(`${path}-assignment-alone-rejected`, 'guardDeviceOrder', { path, data: order, assigned: true }, true)
  }
  await check('cancel-managed-no-active-assignment-allowed', 'guardDeviceOrder', { path: '/admin/orders/order-fixture/cancel', data: order, managed: true }, false)
  await check('cancel-active-assignment-rejected', 'guardDeviceOrder', { path: '/admin/orders/order-fixture/cancel', data: order, assigned: true }, true,
    s => assert.deepEqual(s.queries.at(-1), { order_id: 'order-fixture', status: ['reserved', 'packed', 'shipped'] }))
  await check('dedicated-shipment-delegates-without-device-reads', 'guardDeviceOrder', { path: '/admin/orders/order-fixture/fulfillments/f/shipments', managed: true, assigned: true }, false, s => assert.deepEqual(s.calls, []))
  await check('dedicated-fulfillment-cancel-delegates', 'guardDeviceOrder', { path: '/admin/orders/order-fixture/fulfillments/f/cancel', params: { id: 'order-fixture', fulfillment_id: 'f' }, managed: true, assigned: true }, false, s => assert.deepEqual(s.calls, []))
  await check('quantity-inventory-guard-is-noop', 'guardDeviceInventory', { managed: true, assigned: true }, false, s => assert.deepEqual(s.calls, []))
  await check('ordinary-return-allowed', 'guardDeviceReturn', { body: { order_id: 'order-fixture' } }, false)
  await check('any-device-assignment-blocks-native-return', 'guardDeviceReturn', { body: { order_id: 'order-fixture' }, assigned: true }, true,
    s => assert.deepEqual(s.queries, [{ order_id: 'order-fixture' }]))
  for (const entity of ['return', 'exchange']) {
    await check(`${entity}-resolves-order-before-denial`, 'guardDeviceReturn', { path: `/admin/${entity}s/record-fixture/receive/confirm`,
      params: { id: 'record-fixture' }, data: [{ order_id: 'resolved-order' }], assigned: true }, true,
    s => { assert.equal(s.queries[0].entity, entity); assert.deepEqual(s.queries[1], { order_id: 'resolved-order' }) })
  }
  await check('missing-order-delegates-not-authorized-proof', 'guardDeviceReturn', { params: {} }, false, s => assert.deepEqual(s.calls, []))
  await check('legacy-body-order-precedes-return-path-order', 'guardDeviceReturn', { body: { order_id: 'body-order' }, path: '/admin/returns/r/receive/confirm', assigned: true }, true,
    s => { assert(!s.calls.includes('query')); assert.deepEqual(s.queries, [{ order_id: 'body-order' }]) })
  for (const mode of ['production', 'development', 'test']) {
    await check(`manual-selection-${mode}`, 'guardManualPaymentSelection', { mode, body: { provider_id: 'pp_system_default' } }, mode === 'production')
    await check(`mark-paid-${mode}`, 'guardMarkAsPaid', { mode }, mode === 'production')
  }
  await check('online-selection-allowed', 'guardManualPaymentSelection', { body: { provider_id: 'online-fixture' } }, false)
  for (const status of ['pending', 'requires_more', 'authorized', 'captured', 'pending_authorization', 'canceled', 'error']) {
    await check(`cart-manual-${status}`, 'guardManualCartCompletion', { data: [{ payment_collection: { payment_sessions: [{ provider_id: 'pp_system_default', status }] } }] }, !['canceled', 'error'].includes(status))
  }
  await check('cart-online-allowed', 'guardManualCartCompletion', { data: [{ payment_collection: { payment_sessions: [{ provider_id: 'online-fixture', status: 'authorized' }] } }] }, false)
  await check('legacy-missing-cart-delegates', 'guardManualCartCompletion', {}, false)
  await check('development-cart-skips-query', 'guardManualCartCompletion', { mode: 'development' }, false, s => assert.deepEqual(s.calls, []))
  for (const guard of ['guardManualPaymentAuthorization', 'guardManualPaymentCapture']) {
    await check(`${guard}-manual-blocked`, guard, { body: { payment_session_id: 'session-fixture' }, data: [{ provider_id: 'pp_system_default' }] }, true,
      s => { assert.equal(s.queries[0].entity, guard.endsWith('Authorization') ? 'payment_session' : 'payment'); assert.equal(s.queries[0].filters.id, guard.endsWith('Authorization') ? 'session-fixture' : 'order-fixture') })
    await check(`${guard}-online-allowed`, guard, { body: { payment_session_id: 'session-fixture' }, data: [{ provider_id: 'online-fixture' }] }, false)
    await check(`${guard}-missing-delegates`, guard, { body: { payment_session_id: 'session-fixture' } }, false)
    await check(`${guard}-development-skips-query`, guard, { mode: 'development', body: { payment_session_id: 'session-fixture' } }, false, s => assert.deepEqual(s.calls, []))
  }
  await check('provider-store-region-required', 'listProductionPaymentProviders', { path: '/store/payment-providers' }, true)
  await check('provider-store-only-region-online-projection', 'listProductionPaymentProviders', { path: '/store/payment-providers', filters: { region_id: 'region-fixture', ignored: 'not-forwarded' }, remoteRows: [{ payment_provider: { id: 'online-fixture' } }] }, false, s => {
    assert.deepEqual(s.queries[0], { entryPoint: 'region_payment_provider', variables: { filters: { region_id: 'region-fixture', payment_provider_id: { $ne: 'pp_system_default' } }, skip: 1, take: 20 }, fields: ['payment_provider.id'] })
    assert.deepEqual(s.response, { payment_providers: [{ id: 'online-fixture' }], count: 2, offset: 1, limit: 20 })
  })
  await check('provider-admin-ands-existing-filter', 'listProductionPaymentProviders', { path: '/admin/payments/payment-providers', filters: { is_enabled: true } }, false,
    s => assert.deepEqual(s.queries[0].variables.filters, { $and: [{ is_enabled: true }, { id: { $ne: 'pp_system_default' } }] }))
  await check('provider-development-delegates', 'listProductionPaymentProviders', { mode: 'development', path: '/store/payment-providers' }, false, s => assert.deepEqual(s.calls, []))
  const result = { format_version: 1,
    scope: 'Actual project middleware mount syntax plus isolated actual guard execution against artificial ports; native validation/auth/SQL/provider/transaction and handler execution not proven. Delegation is not authorization.',
    installed_medusa_version: JSON.parse(packageBytes).version, source_sha256: hashes, mounts,
    traced_native_entrypoints: followups, cases }
  const file = path.join(root, 'docs/migration/fixtures/legacy-native-guards.json')
  const content = JSON.stringify(result, null, 2) + '\n'
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(file, 'utf8').replaceAll('\r\n', '\n'), content)
  else fs.writeFileSync(file, content)
  console.log(JSON.stringify({ guard_functions: guards.size, method_mounts: mounts.length,
    handler_source_missing_or_method_absent: mounts.filter(m => m.handler_evidence.status !== 'handler-source-found-runtime-not-proven').length,
    cases: cases.length, source_files: Object.keys(hashes).length, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
}
main().catch(error => { console.error(error.stack); process.exitCode = 1 })
