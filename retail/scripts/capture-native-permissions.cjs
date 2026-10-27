// Execute installed RBAC matching and handler wrapper with fake query/cache ports.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const root = path.resolve(__dirname, '../..')
const base = 'medusa-store/apps/backend/node_modules/@medusajs/framework/'
const hashes = {}, cases = []
function read(file) {
  const bytes = fs.readFileSync(path.join(root, file))
  hashes[file] = crypto.createHash('sha256').update(bytes).digest('hex')
  return bytes.toString('utf8')
}
class PermissionError extends Error {
  static Types = { FORBIDDEN: 'forbidden' }
  constructor(type, message) { super(message); this.type = type }
}
const utils = { WILDCARD: '*', MedusaError: PermissionError,
  ContainerRegistrationKeys: { FEATURE_FLAG_ROUTER: 'flags', QUERY: 'query' },
  useCache: async (run, options) => {
    options.container.calls.push({ cache: { key: options.key, ttl: options.ttl, providers: [...options.providers] } })
    if (options.container.failCache) throw new Error('CACHE_PORT_UNAVAILABLE')
    return run()
  } }
function load(file, extra = {}) {
  const record = { exports: {} }
  vm.runInNewContext(`(function(require,module,exports){${read(file)}\n})`, {}, { filename: file, timeout: 3000 })(name => {
    if (Object.hasOwn(extra, name)) return extra[name]
    if (name === '@medusajs/utils') return utils
    throw new Error('Unreviewed dependency: ' + name)
  }, record, record.exports)
  return record.exports
}
const engine = load(base + 'dist/policies/has-permission.js')
const wrapper = load(base + 'dist/http/middlewares/check-permissions.js', { '../../policies/has-permission': engine })
const version = JSON.parse(read(base + 'package.json')).version
const action = { resource: 'order', operation: 'read' }
const policy = (resource, operation) => ({ id: `${resource}:${operation}`, resource, operation })
function ports(options = {}) {
  const calls = []
  const container = { calls, failCache: options.failCache, resolve(name) {
    calls.push({ resolve: name })
    if (name === 'flags') return { isFeatureEnabled(flag) { assert.equal(flag, 'rbac'); return options.enabled !== false } }
    if (name === 'query') return { graph: async query => {
      calls.push({ query: JSON.parse(JSON.stringify(query)) })
      if (options.failQuery) throw new Error('QUERY_PORT_UNAVAILABLE')
      assert.equal(query.entity, 'rbac_role'); assert.deepEqual([...query.fields], ['id', 'policies.*'])
      const policies = (options.policies || {})[query.filters.id]
      return { data: policies ? [{ id: query.filters.id, policies }] : [] }
    } }
    throw new Error('Unreviewed resolve: ' + name)
  } }
  return container
}
async function check(name, run) { await run(); cases.push({ name, passed: true }) }
async function matching(name, options, roles, actions, expected) {
  await check(name, async () => assert.equal(await engine.hasPermission({ container: ports(options), roles, actions }), expected))
}
async function wrapped(name, options, policies, roles, allowed) {
  await check(name, async () => {
    let handled = 0, forwarded
    const req = { scope: ports(options), auth_context: roles === null ? undefined : { app_metadata: { roles } } }
    const handler = wrapper.wrapWithPoliciesCheck(() => { handled++ }, policies)
    await handler(req, {}, error => { forwarded = error })
    assert.equal(handled, allowed ? 1 : 0)
    if (allowed) assert.equal(forwarded, undefined)
    else assert.equal(forwarded?.type, 'forbidden')
    assert.equal(req.policies.length, Array.isArray(policies) ? policies.length : 1)
  })
}
async function main() {
  await matching('no-policy-denied', {}, ['role-a'], action, false)
  await matching('exact-resource-operation-allowed', { policies: { 'role-a': [policy('order', 'read')] } }, ['role-a'], action, true)
  await matching('wrong-operation-denied', { policies: { 'role-a': [policy('order', 'update')] } }, ['role-a'], action, false)
  await matching('wrong-resource-denied', { policies: { 'role-a': [policy('product', 'read')] } }, ['role-a'], action, false)
  await matching('resource-wildcard-allowed', { policies: { 'role-a': [policy('*', 'read')] } }, ['role-a'], action, true)
  await matching('operation-wildcard-allowed', { policies: { 'role-a': [policy('order', '*')] } }, ['role-a'], action, true)
  await matching('double-wildcard-allowed', { policies: { 'role-a': [policy('*', '*')] } }, ['role-a'], action, true)
  await matching('partial-wildcard-not-pattern-matching', { policies: { 'role-a': [policy('ord*', 'read')] } }, ['role-a'], action, false)
  await matching('resource-case-sensitive', { policies: { 'role-a': [policy('Order', 'read')] } }, ['role-a'], action, false)
  await matching('operation-case-sensitive', { policies: { 'role-a': [policy('order', 'Read')] } }, ['role-a'], action, false)
  await matching('roles-union-grants', { policies: { 'role-a': [policy('product', 'read')], 'role-b': [policy('order', 'read')] } }, ['role-a', 'role-b'], action, true)
  const combined = { policies: { 'role-a': [policy('order', 'read')], 'role-b': [policy('order', 'update')] } }
  await matching('all-operations-must-be-granted-across-roles', combined, ['role-a', 'role-b'], { resource: 'order', operation: ['read', 'update'] }, true)
  await matching('missing-one-operation-denies-whole-command', combined, ['role-a'], { resource: 'order', operation: ['read', 'update'] }, false)
  await matching('all-actions-required-not-any', combined, ['role-a', 'role-b'], [action, { resource: 'product', operation: 'read' }], false)
  await matching('role-single-string-supported', combined, 'role-a', action, true)
  await matching('legacy-empty-roles-hasPermission-returns-true', {}, [], action, true)
  await matching('legacy-empty-actions-returns-true', {}, ['role-a'], [], true)
  await matching('legacy-feature-disabled-allows', { enabled: false }, ['role-a'], action, true)
  const universe = [action, { resource: 'order', operation: 'update' }, { resource: 'product', operation: 'read' }]
  await check('resolve-expands-wildcards-only-within-universe', async () => {
    const result = await engine.resolvePermissions({ roles: ['role-a'], universe, container: ports({ policies: { 'role-a': [policy('order', '*')] } }) })
    assert.deepEqual([...result].sort(), ['order:read', 'order:update'])
  })
  await check('resolve-empty-roles-returns-empty-unlike-hasPermission', async () => assert.equal((await engine.resolvePermissions({ roles: [], universe, container: ports() })).size, 0))
  await check('resolve-disabled-returns-universe', async () => assert.deepEqual([...(await engine.resolvePermissions({ roles: [], universe, container: ports({ enabled: false }) }))].sort(), ['order:read', 'order:update', 'product:read']))
  await check('cache-options-seven-days-memory-not-revocation-proof', async () => {
    const container = ports(combined)
    await engine.hasPermission({ roles: ['role-a'], actions: action, container })
    assert.deepEqual(container.calls.find(c => c.cache).cache, { key: 'role-a', ttl: 604800, providers: ['cache-memory'] })
    assert.deepEqual(container.calls.find(c => c.query).query.filters, { id: 'role-a' })
  })
  for (const [failure, text] of [['failQuery', 'QUERY_PORT_UNAVAILABLE'], ['failCache', 'CACHE_PORT_UNAVAILABLE']]) await check(`${failure}-propagates-not-allow`, () =>
    assert.rejects(() => engine.hasPermission({ roles: ['role-a'], actions: action, container: ports({ [failure]: true }) }), new RegExp(text)))
  await wrapped('wrapper-no-auth-denied', {}, action, null, false)
  await wrapped('wrapper-no-roles-denied-even-engine-empty-roles-allows', {}, action, [], false)
  await wrapped('wrapper-no-grant-denied', {}, action, ['role-a'], false)
  await wrapped('wrapper-grant-invokes-handler', combined, action, ['role-a'], true)
  await wrapped('wrapper-empty-policy-list-needs-other-auth', {}, [], null, true)
  await wrapped('wrapper-feature-disabled-with-roles-allowed', { enabled: false }, action, ['role-a'], true)
  await wrapped('wrapper-feature-disabled-without-roles-still-denied', { enabled: false }, action, [], false)
  await check('wrapper-query-failure-forwarded-handler-not-called', async () => {
    let handled = false, forwarded
    await wrapper.wrapWithPoliciesCheck(() => { handled = true }, action)({ scope: ports({ failQuery: true }), auth_context: { app_metadata: { roles: ['role-a'] } } }, {}, error => { forwarded = error })
    assert.equal(handled, false); assert.equal(forwarded.message, 'QUERY_PORT_UNAVAILABLE')
  })
  const report = { format_version: 1, installed_framework_version: version,
    scope: 'Actual installed hasPermission/resolvePermissions and policy handler wrapper with artificial feature/query/cache ports; no live database, token validation, real cache invalidation, route policy mount or Gin acceptance.',
    source_sha256: hashes, cases,
    target_requirements: ['Missing identity/roles and empty or unknown action declarations fail closed in Gin',
      'Explicit resource/operation matching; grants compose across roles but every requested action is required',
      'Scope, fields and business state are additional checks; a role grant alone is insufficient',
      'Immediate disable/revocation and policy version checks cannot rely on long lived process caches or JWT roles alone'],
    remaining: ['Native/custom/plugin route policy inventory', 'Real cache invalidation and session/current-role behavior', 'Gin persistent grants, scoped DTOs and concurrency integration'] }
  const content = JSON.stringify(report, null, 2) + '\n'
  const output = path.join(root, 'docs/migration/fixtures/legacy-native-permissions.json')
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(output, 'utf8').replaceAll('\r\n', '\n'), content)
  else fs.writeFileSync(output, content)
  console.log(JSON.stringify({ cases: cases.length, source_files: Object.keys(hashes).length, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
}
main().catch(error => { console.error(error.message); process.exitCode = 1 })
