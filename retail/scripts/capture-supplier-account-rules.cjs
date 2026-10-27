// Execute selected legacy step callbacks against isolated in-memory ports.
// No app, framework workflows, database, environment or provider is loaded.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backend = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const ts = backend('typescript')
const source = 'medusa-store/apps/backend/src/workflows/device-supplier-account.ts'
const bytes = fs.readFileSync(path.join(root, source))
const ast = ts.createSourceFile(source, bytes.toString('utf8'), ts.ScriptTarget.Latest, true)
const callbacks = new Map()
for (const statement of ast.statements) {
  if (!ts.isVariableStatement(statement)) continue
  for (const declaration of statement.declarationList.declarations) {
    if (!ts.isIdentifier(declaration.name) || !['prepareAccount', 'createSupplierAccount'].includes(declaration.name.text)) continue
    const call = declaration.initializer
    assert(ts.isCallExpression(call) && call.expression.getText(ast) === 'createStep')
    const name = declaration.name.text
    for (let index = 1; index < call.arguments.length; index++) {
      assert(ts.isArrowFunction(call.arguments[index]), 'Only reviewed callback declarations accepted')
      const js = ts.transpileModule(`module.exports = ${call.arguments[index].getText(ast)}`, {
        compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
      }).outputText
      callbacks.set(`${name}:${index}`, js)
    }
  }
}
assert.deepEqual([...callbacks.keys()].sort(), ['createSupplierAccount:1', 'createSupplierAccount:2', 'prepareAccount:1'])
const input = { actor_id: 'staff-fixture', email: 'supplier@example.invalid', source_id: 'source-fixture', order_ids: ['order-a', 'order-a', 'order-b'] }
const clone = value => JSON.parse(JSON.stringify(value))
const cases = []
function setup(options = {}) {
  const events = []
  const saved = {}
  const users = {
    async listUsers() { events.push('users.lookup'); return options.user === false ? [] : [{ id: options.self ? input.actor_id : 'supplier-fixture', metadata: { retained: 'yes', device_role: 'old' } }] },
    async createUsers(data) { events.push('users.create'); saved.user = clone(data); if (options.failCreate) throw new Error('CREATE_FAILED'); return { id: 'created-fixture' } },
    async deleteUsers(ids) { events.push('users.delete'); saved.deletedUsers = clone(ids); if (options.failUserDelete) throw new Error('USER_DELETE_FAILED') },
  }
  const devices = { async listDeviceSources(filter) { events.push('sources.lookup'); assert.deepEqual(clone(filter), { id: input.source_id, type: 'supplier' }); return options.source === false ? [] : [{ id: input.source_id }] } }
  const orders = { async retrieveOrder(id) { events.push('orders.lookup'); if (options.badOrder === id) throw new Error('ORDER_MISSING'); return { id } } }
  const auth = {
    async register(provider, request) {
      events.push('auth.register'); assert.equal(provider, 'emailpass')
      assert.equal(request.body.email, input.email)
      assert.equal(request.body.password, 'fixture-only-password')
      if (options.registration === 'throw') throw new Error('REGISTER_FAILED')
      return { success: options.registration !== 'failed', ...(options.registration !== 'no-identity' ? { authIdentity: { id: 'auth-fixture' } } : {}) }
    },
    async updateAuthIdentities(data) { events.push('auth.link'); saved.link = clone(data); if (options.failLink) throw new Error('LINK_FAILED') },
    async deleteAuthIdentities(ids) { events.push('auth.delete'); saved.deletedAuth = clone(ids); if (options.failAuthDelete) throw new Error('AUTH_DELETE_FAILED') },
  }
  class LegacyError extends Error { static Types = { INVALID_DATA: 'invalid_data' }; constructor(type, message) { super(message); this.type = type } }
  const container = { resolve(name) { assert(['users', 'devices', 'orders', 'auth'].includes(name)); return { users, devices, orders, auth }[name] } }
  const globals = {
    Modules: { USER: 'users', ORDER: 'orders', AUTH: 'auth' }, DEVICES_MODULE: 'devices',
    MedusaError: LegacyError, supplierDenied: () => new Error('SUPPLIER_DENIED'),
    deviceAccess: async () => { events.push('actor.access'); return { role: options.role || 'admin' } },
    StepResponse: class { constructor(result, compensation) { this.result = result; this.compensation = compensation } },
  }
  function get(key) {
    const module = { exports: {} }
    vm.runInNewContext(callbacks.get(key), { ...globals, module }, { filename: source, timeout: 3000 })
    return module.exports
  }
  return { events, saved, container, get }
}
async function check(name, kind, options, expectedError, verify) {
  const state = setup(options)
  let result, error
  try {
    result = await state.get(kind)(kind.endsWith(':2') ? { userId: 'created-fixture', authId: 'auth-fixture' } :
      { ...input, ...(kind.startsWith('create') ? { password: 'fixture-only-password' } : {}) }, { container: state.container })
  } catch (caught) { error = caught.message }
  assert.equal(error, expectedError, name)
  verify?.(state, result)
  // No login secrets or source row values are written to the fixture.
  cases.push({ name, callback: kind, artificial_options: options, outcome: error ? { error } : { success: true },
    observed_port_calls: state.events, independent_assertions_passed: true })
}
async function main() {
  await check('binding-preserves-metadata-deduplicates-order-ids', 'prepareAccount:1', {}, undefined, (s, result) => {
    assert.deepEqual(clone(result.result), { id: 'supplier-fixture', metadata: { retained: 'yes', device_role: 'supplier', device_source_id: input.source_id, device_order_ids: ['order-a', 'order-b'] } })
    assert.equal(s.events.filter(e => e === 'orders.lookup').length, 3)
  })
  for (const kind of ['prepareAccount:1', 'createSupplierAccount:1']) {
    await check(`${kind}-non-admin-denied-before-reads`, kind, { role: 'supplier' }, 'SUPPLIER_DENIED', s => assert.deepEqual(s.events, ['actor.access']))
  }
  await check('binding-missing-account-rejected', 'prepareAccount:1', { user: false }, 'Supplier account not found')
  await check('binding-self-rejected', 'prepareAccount:1', { self: true }, 'Cannot bind your own account as supplier')
  await check('binding-wrong-source-rejected', 'prepareAccount:1', { source: false }, 'Select a supplier source')
  await check('binding-missing-order-rejected', 'prepareAccount:1', { badOrder: 'order-b' }, 'ORDER_MISSING')
  await check('create-existing-account-rejected-before-registration', 'createSupplierAccount:1', {}, 'Supplier account already exists; leave password empty to update permissions', s => assert(!s.events.includes('auth.register')))
  await check('create-wrong-source-rejected', 'createSupplierAccount:1', { source: false }, 'Select a supplier source')
  await check('create-missing-order-before-registration', 'createSupplierAccount:1', { user: false, badOrder: 'order-b' }, 'ORDER_MISSING', s => assert(!s.events.includes('auth.register')))
  await check('create-role-restrictions-before-login-link', 'createSupplierAccount:1', { user: false }, undefined, (s, result) => {
    assert.deepEqual(s.events, ['actor.access', 'sources.lookup', 'users.lookup', 'orders.lookup', 'orders.lookup', 'orders.lookup', 'auth.register', 'users.create', 'auth.link'])
    assert.deepEqual(s.saved.user.metadata, { device_role: 'supplier', device_source_id: input.source_id, device_order_ids: ['order-a', 'order-b'] })
    assert.deepEqual(clone(result.compensation), { userId: 'created-fixture', authId: 'auth-fixture' })
    assert.deepEqual(s.saved.link, { id: 'auth-fixture', app_metadata: { user_id: 'created-fixture' } })
  })
  for (const registration of ['failed', 'no-identity', 'throw']) {
    await check(`registration-${registration}-no-user-created`, 'createSupplierAccount:1', { user: false, registration }, registration === 'throw' ? 'REGISTER_FAILED' : 'Unable to create supplier login', s => assert(!s.events.includes('users.create')))
  }
  await check('user-create-failure-cleans-auth', 'createSupplierAccount:1', { user: false, failCreate: true }, 'CREATE_FAILED', s => {
    assert.deepEqual(s.saved.deletedAuth, ['auth-fixture']); assert(!s.events.includes('users.delete'))
  })
  await check('link-failure-cleans-user-then-auth', 'createSupplierAccount:1', { user: false, failLink: true }, 'LINK_FAILED', s => assert.deepEqual(s.events.slice(-2), ['users.delete', 'auth.delete']))
  await check('legacy-user-cleanup-failure-skips-auth-cleanup', 'createSupplierAccount:1', { user: false, failLink: true, failUserDelete: true }, 'USER_DELETE_FAILED', s => assert(!s.events.includes('auth.delete')))
  await check('legacy-auth-cleanup-failure-overwrites-initial-error', 'createSupplierAccount:1', { user: false, failCreate: true, failAuthDelete: true }, 'AUTH_DELETE_FAILED')
  await check('framework-compensation-deletes-auth-then-user', 'createSupplierAccount:2', {}, undefined, s => assert.deepEqual(s.events, ['auth.delete', 'users.delete']))
  await check('legacy-compensation-auth-failure-skips-user-cleanup', 'createSupplierAccount:2', { failAuthDelete: true }, 'AUTH_DELETE_FAILED', s => assert.deepEqual(s.events, ['auth.delete']))
  const result = { format_version: 1,
    scope: 'Artificial port-level execution of selected actual legacy step callbacks; framework/SQL/auth-provider/RBAC/transaction integration not executed. Cleanup gaps are observations, not target requirements.',
    source_sha256: { [source]: crypto.createHash('sha256').update(bytes).digest('hex') }, cases }
  const file = path.join(root, 'docs/migration/fixtures/legacy-supplier-account-rules.json')
  const content = JSON.stringify(result, null, 2) + '\n'
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(file, 'utf8').replaceAll('\r\n', '\n'), content)
  else fs.writeFileSync(file, content)
  console.log(JSON.stringify({ callbacks: callbacks.size, cases: cases.length, source_files: 1, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
}
main().catch(error => { console.error(error.message); process.exitCode = 1 })
