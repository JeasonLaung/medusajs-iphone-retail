// Exercise installed routing machinery on loopback with artificial handlers only.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const crypto = require('node:crypto')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backendDir = path.join(root, 'medusa-store/apps/backend')
const backend = createRequire(path.join(backendDir, 'package.json'))
const framework = createRequire(backend.resolve('@medusajs/framework'))
const express = framework('express')
const ts = backend('typescript')
const base = path.join(root, 'medusa-store/apps/backend/node_modules/@medusajs/framework/dist')
const manifestFile = path.join(root, 'docs/migration/route-policy-registrations.json')
const manifest = JSON.parse(fs.readFileSync(manifestFile, 'utf8'))
const hashes = {}, cases = [], reads = []
const rel = file => path.relative(root, file).replaceAll('\\', '/')
const canonicalPaths = new Map()
const canonical = file => {
  if (!canonicalPaths.has(file)) canonicalPaths.set(file, fs.existsSync(file) ? fs.realpathSync(file) : path.resolve(file))
  return canonicalPaths.get(file)
}
function read(file) {
  const bytes = fs.readFileSync(file)
  hashes[rel(file)] = crypto.createHash('sha256').update(bytes).digest('hex')
  return bytes.toString('utf8')
}
hashes[rel(manifestFile)] = crypto.createHash('sha256').update(fs.readFileSync(manifestFile)).digest('hex')
for (const [file, expected] of Object.entries(manifest.source_sha256)) {
  assert.equal(crypto.createHash('sha256').update(fs.readFileSync(path.join(root, file))).digest('hex'), expected, 'Registration manifest source drift: ' + file)
}
function load(file, ports) {
  const module = { exports: {} }
  const requirePort = name => {
    assert(Object.hasOwn(ports, name), 'Unreviewed dependency: ' + name)
    return ports[name]
  }
  requirePort.resolve = (name, options) => require.resolve(name, options)
  vm.runInNewContext(`(function(require,module,exports,__dirname){${read(file)}\n})`, { process: { cwd: () => root } }, { filename: file, timeout: 3000 })(requirePort, module, module.exports, path.dirname(file))
  return module.exports
}
const noop = (req, res, next) => next()
const logger = { debug() {}, warn() {}, error() {} }
const configText = read(path.join(backendDir, 'medusa-config.ts'))
const configAst = ts.createSourceFile('medusa-config.ts', configText, ts.ScriptTarget.Latest, true)
let configObject
for (const statement of configAst.statements) if (ts.isExpressionStatement(statement) && ts.isBinaryExpression(statement.expression) && statement.expression.left.getText(configAst) === 'module.exports') configObject = statement.expression.right.arguments[0]
const pluginsNode = configObject.properties.find(property => property.name?.getText(configAst) === 'plugins').initializer
assert(ts.isArrayLiteralExpression(pluginsNode))
const pluginNames = pluginsNode.elements.map(element => {
  const resolve = element.properties.find(property => property.name?.getText(configAst) === 'resolve')?.initializer
  assert(ts.isStringLiteral(resolve), 'Dynamic configured plugin needs review')
  return resolve.text
})
const utilsDir = path.dirname(framework.resolve('@medusajs/utils'))
const resolvedPlugins = load(path.join(utilsDir, 'common/get-resolved-plugins.js'), {
  'fs/promises': require('node:fs/promises'), path,
  './is-string': { isString: value => typeof value === 'string' },
  './read-dir-recursive': { readDir: async () => [] }, // Module enumeration unused by API source ordering.
})
class AccessError extends Error {
  static Types = { FORBIDDEN: 'forbidden' }
  constructor(type, message) { super(message); this.type = type }
}
const types = load(path.join(base, 'http/types.js'), {})
const helper = load(path.join(base, 'http/utils/define-middlewares.js'), {})
let sourceDirs
function handlersIn(dir) {
  const target = canonical(dir)
  return manifest.handlers.filter(entry => path.dirname(canonical(path.join(root, entry.source))).startsWith(target + path.sep) || path.dirname(canonical(path.join(root, entry.source))) === target)
}
function middlewareIn(dir) { return manifest.registrations.filter(entry => canonical(path.dirname(path.join(root, entry.source))) === canonical(dir)) }
// Native root registration arrays include submodule sources; preserve their
// already verified expansion order while importing the one native root file.
const nativeRoot = path.join(root, 'medusa-store/apps/backend/node_modules/@medusajs/medusa/dist/api')
function registrationSet(dir) {
  if (canonical(dir) === canonical(nativeRoot)) return manifest.registrations.filter(entry => entry.source.startsWith(rel(nativeRoot) + '/'))
  return middlewareIn(dir)
}
const utils = {
  ContainerRegistrationKeys: { LOGGER: 'logger', FEATURE_FLAG_ROUTER: 'flags', QUERY: 'query' },
  FeatureFlag: { isFeatureEnabled: flag => flag === 'rbac' },
  MedusaError: AccessError, WILDCARD: '*', isString: value => typeof value === 'string',
  isFileSkipped: () => false, isFileDisabled: () => false, parseCorsOrigins: () => [],
  useCache: async run => run(),
  readDirRecursive: async dir => {
    reads.push(rel(dir))
    const files = [...new Set(handlersIn(dir).map(entry => {
      const originalRoot = entry.kind === 'native' ? nativeRoot : entry.kind === 'custom' ? path.join(backendDir, 'src/api') : path.join(backendDir, 'node_modules/@agilo/medusa-analytics-plugin/.medusa/server/src/api')
      return path.join(dir, path.relative(originalRoot, path.join(root, entry.source)))
    }))]
    return files.map(file => ({ name: path.basename(file), path: path.dirname(file), isDirectory: () => false }))
  },
  FileSystem: class {
    constructor(dir) { this.dir = dir }
    async exists(name) {
      if (!registrationSet(this.dir).length) return false
      return name === (this.dir === path.join(backendDir, 'src/api') ? 'middlewares.ts' : 'middlewares.js')
    }
  },
  dynamicImport: async file => {
    if (/middlewares\.(js|ts)$/.test(file)) return { default: helper.defineMiddlewares({ routes: registrationSet(path.dirname(file)).map(entry => ({
      matcher: entry.matcher, ...(entry.methods ? { methods: entry.methods } : {}),
      ...(entry.policies.length ? { policies: entry.policies } : {}),
      middlewares: entry.middleware_expressions ? [noop] : [],
    })) }) }
    const entries = manifest.handlers.filter(entry => canonical(path.join(root, entry.source)) === canonical(file))
    assert(entries.length, 'Unreviewed synthetic handler import')
    return { AUTHENTICATE: entries[0].route_loader_authenticate, ...Object.fromEntries(entries.map(entry => [entry.method, (req, res) => res.json({
      fixture_handler_source: entry.kind, method: entry.method, path: entry.path,
      policies_checked: req.policies?.map(policy => `${policy.resource}:${policy.operation}`) || [],
    })])) }
  },
}
const permissionEngine = load(path.join(base, 'policies/has-permission.js'), { '@medusajs/utils': utils })
const checks = load(path.join(base, 'http/middlewares/check-permissions.js'), { '@medusajs/utils': utils, '../../policies/has-permission': permissionEngine })
const routes = load(path.join(base, 'http/routes-loader.js'), { '@medusajs/utils': utils, path, '../logger': { logger }, './types': types })
const middlewares = load(path.join(base, 'http/middleware-file-loader.js'), {
  '@medusajs/utils': utils, path, '../logger': { logger }, './types': types,
  '@medusajs/deps/zod': { z: { object: () => ({ nullish: () => ({}) }) } },
})
const sorter = load(path.join(base, 'http/routes-sorter.js'), {})
const finder = load(path.join(base, 'http/routes-finder.js'), { '@medusajs/utils': utils, 'path-to-regexp': framework('path-to-regexp') })
const wrap = load(path.join(base, 'http/utils/wrap-handler.js'), {})
const container = { resolve(name) { if (name === 'logger') return logger; throw new Error('Unreviewed app-container port: ' + name) } }
const api = load(path.join(base, 'http/router.js'), {
  '@medusajs/utils': utils, cors: () => noop, path,
  '../config': { configManager: { config: { projectConfig: { http: {} } } } },
  './middleware-file-loader': middlewares, './routes-loader': routes, './routes-sorter': sorter, './routes-finder': finder,
  './middlewares/check-permissions': checks, './utils/wrap-handler': wrap,
  './middlewares': { applyLocale: noop, authenticate: () => (req, res, next) => req.auth_context ? next() : res.status(401).json({ fixture_error: 'anonymous' }) },
  './middlewares/bodyparser': { createBodyParserMiddlewaresStack: () => noop },
  './middlewares/ensure-publishable-api-key': { ensurePublishableApiKeyMiddleware: noop },
  './middlewares/error-handler': { errorHandler: () => (error, req, res, next) => res.status(error.type === 'forbidden' ? 403 : 500).json({ fixture_error: error.type || 'internal' }) },
  './utils/restricted-fields': { RestrictedFields: class { add() {} } },
})
async function check(name, run) { await run(); cases.push({ name, passed: true }) }
async function main() {
  const plugins = await resolvedPlugins.getResolvedPlugins(backendDir, { plugins: pluginNames.map(resolve => ({ resolve, options: {} })) }, true)
  // Run the real Medusa API entry loader too; it constructs sourcePaths and
  // invokes real ApiLoader. Only plugin options and handler bodies are artificial.
  const entry = load(path.join(root, 'medusa-store/apps/backend/node_modules/@medusajs/medusa/dist/loaders/api.js'), {
    '@medusajs/framework/http': { ApiLoader: class extends api.ApiLoader { constructor(options) { sourceDirs = options.sourceDir; super(options) } } },
    '@medusajs/framework/utils': utils, path, qs: { parse: () => ({}) },
  })
  const app = express()
  app.use((req, res, next) => {
    const authenticated = req.get('x-fixture-actor') === 'staff'
    if (authenticated) req.auth_context = { app_metadata: { roles: ['role-fixture'] } }
    const granted = (req.get('x-fixture-policies') || '').split(',').filter(Boolean).map(key => { const [resource, operation] = key.split(':'); return { id: key, resource, operation } })
    req.scope = { resolve(name) {
      if (name === 'flags') return { isFeatureEnabled: () => true }
      if (name === 'query') return { graph: async () => ({ data: [{ policies: granted }] }) }
      throw new Error('Unreviewed request scope port')
    } }
    next()
  })
  const appContainer = { resolve(name) { return name === 'configModule' ? { projectConfig: { http: {} } } : container.resolve(name) } }
  await entry.default({ app, container: appContainer, plugins })
  let server
  try {
    server = await new Promise(resolve => { const handle = app.listen(0, '127.0.0.1', () => resolve(handle)) })
    const origin = 'http://127.0.0.1:' + server.address().port
    async function request(method, routePath, policies, authenticated = true) {
      const response = await fetch(origin + routePath.replace(/:\w+/g, 'fixture-id'), { method,
        headers: { ...(authenticated ? { 'x-fixture-actor': 'staff' } : {}), 'x-fixture-policies': policies.join(',') }, signal: AbortSignal.timeout(5000) })
      return { status: response.status, body: await response.json() }
    }
    await check('source-order-core-plugins-project-last', () => {
      assert.equal(sourceDirs[0], nativeRoot)
      assert.equal(sourceDirs.at(-1), path.join(backendDir, 'src/api'))
      assert.deepEqual(reads, Array.from(sourceDirs, rel))
      assert.equal(sourceDirs.length, pluginNames.length + 2)
      for (let i = 0; i < pluginNames.length; i++) assert.equal(canonical(sourceDirs[i + 1]), canonical(path.join(backendDir, 'node_modules', pluginNames[i], '.medusa/server/src/api')))
    })
    for (const group of manifest.duplicate_handlers) {
      const [method, routePath] = group.method_path.split(' ')
      const candidate = manifest.handlers.find(entry => entry.kind === 'custom' && entry.method === method && entry.path === routePath)
      await check('project-handler-overrides-' + group.method_path, async () => {
        const result = await request(method, routePath, ['*:*'])
        assert.equal(result.status, 200); assert.equal(result.body.fixture_handler_source, 'custom')
      })
      if (candidate.matched_policy_keys.length) {
        await check('inherited-policy-denies-before-handler-' + group.method_path, async () => assert.equal((await request(method, routePath, [])).status, 403))
        await check('inherited-policy-allows-required-grants-' + group.method_path, async () => {
          const result = await request(method, routePath, candidate.matched_policy_keys)
          assert.equal(result.status, 200)
          assert.deepEqual([...new Set(result.body.policies_checked)].sort(), candidate.matched_policy_keys)
        })
      }
    }
    await check('anonymous-admin-blocked-before-artificial-handler', async () => assert.equal((await request('GET', '/admin/orders', ['*:*'], false)).status, 401))
    await check('native-feature-flags-auth-optout-honored', async () => assert.equal((await request('GET', '/admin/feature-flags', [], false)).status, 200))
    await check('native-invite-accept-optout-does-not-prove-invite-validation', async () => assert.equal((await request('POST', '/admin/invites/accept', [], false)).status, 200))
    await check('after-sales-read-policy-alone-reaches-handler-scope-not-write-acceptance', async () => assert.equal((await request('POST', '/admin/orders/:id/after-sales', ['order:read'])).status, 200))
    await check('tracking-needs-order-read-and-fulfillment-update', async () => assert.equal((await request('POST', '/admin/orders/:id/fulfillments/:fulfillment_id/tracking', ['order:read'])).status, 403))
    for (const handler of manifest.handlers.filter(entry => entry.kind === 'plugin')) await check('configured-plugin-handler-loaded-' + handler.path, async () => {
      const result = await request(handler.method, handler.path, [])
      assert.equal(result.status, 200); assert.equal(result.body.fixture_handler_source, 'plugin')
      assert.deepEqual(result.body.policies_checked, [])
    })
  } finally {
    if (server) { server.closeAllConnections(); await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())) }
  }
  const report = { format_version: 1,
    scope: 'Installed Medusa API entry loader, ApiLoader, RoutesLoader, MiddlewareFileLoader, RoutesSorter, RoutesFinder, permission matcher/wrapper and Express over ephemeral loopback. Route modules/middleware bodies, authentication, DB/cache, validators/providers are artificial ports; no real business handlers or writes.',
    source_dirs: sourceDirs.map(rel), cases, source_sha256: hashes,
    cleanup: 'ephemeral loopback listener closed; no database, Redis or server deployment connected',
    remaining: ['Live service effective routes/dynamic HMR may differ; not inspected by mutating requests', 'Real authentication, role refresh/revocation, custom guards, validation/field filters and full workflows', 'Gin explicit permissions and transaction acceptance'] }
  const content = JSON.stringify(report, null, 2) + '\n'
  const output = path.join(root, 'docs/migration/fixtures/legacy-route-loading.json')
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(output, 'utf8').replaceAll('\r\n', '\n'), content)
  else fs.writeFileSync(output, content)
  console.log(JSON.stringify({ cases: cases.length, source_dirs: sourceDirs.length, source_files: Object.keys(hashes).length, cleanup: 'complete', mode: process.argv.includes('--check') ? 'check' : 'generate' }))
}
main().catch(error => { console.error(error.message); process.exitCode = 1 })
