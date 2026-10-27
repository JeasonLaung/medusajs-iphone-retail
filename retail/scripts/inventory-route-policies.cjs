// Follow registered middleware arrays without executing application modules.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const { createRequire } = require('node:module')
const vm = require('node:vm')
const root = path.resolve(__dirname, '../..')
const backend = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const ts = backend('typescript')
const framework = createRequire(backend.resolve('@medusajs/framework'))
const expressRequire = createRequire(framework.resolve('express'))
const matcherFile = expressRequire.resolve('path-to-regexp')
const pathToRegexp = expressRequire('path-to-regexp')
const native = 'medusa-store/apps/backend/node_modules/@medusajs/medusa/dist/api'
const custom = 'medusa-store/apps/backend/src/api'
const plugin = 'medusa-store/apps/backend/node_modules/@agilo/medusa-analytics-plugin/.medusa/server/src/api'
const frameworkBase = 'medusa-store/apps/backend/node_modules/@medusajs/framework/dist/http/'
const relative = file => path.relative(root, file).replaceAll('\\', '/')
const hashes = {}, modules = new Map(), registrations = []
const rawMethodDeclarations = []
function read(file) {
  const bytes = fs.readFileSync(file)
  hashes[relative(file)] = crypto.createHash('sha256').update(bytes).digest('hex')
  return bytes.toString('utf8')
}
function unwrap(node) {
  while (node && (ts.isParenthesizedExpression(node) || ts.isAsExpression(node))) node = node.expression
  return node
}
function prop(node, name) {
  assert(ts.isObjectLiteralExpression(node), 'Expected object literal')
  const property = node.properties.find(p => (ts.isPropertyAssignment(p) || ts.isShorthandPropertyAssignment(p)) && p.name.getText().replaceAll('"', '').replaceAll("'", '') === name)
  return property && (ts.isShorthandPropertyAssignment(property) ? property.name : property.initializer)
}
function moduleInfo(file) {
  file = path.resolve(file)
  if (modules.has(file)) return modules.get(file)
  const tree = ts.createSourceFile(file, read(file), ts.ScriptTarget.Latest, true)
  const info = { file, tree, vars: new Map(), exports: new Map(), enums: new Map(), imports: new Map() }
  modules.set(file, info)
  function scan(node) {
    if (ts.isImportDeclaration(node) && node.importClause?.namedBindings && ts.isNamedImports(node.importClause.namedBindings))
      for (const entry of node.importClause.namedBindings.elements) info.imports.set(entry.name.text, { source: node.moduleSpecifier.text, name: entry.propertyName?.text || entry.name.text })
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer && ts.isVariableStatement(node.parent.parent) && ts.isSourceFile(node.parent.parent.parent)) info.vars.set(node.name.text, node.initializer)
    if (ts.isFunctionDeclaration(node) && node.name && ts.isSourceFile(node.parent)) info.vars.set(node.name.text, node)
    if (ts.isFunctionDeclaration(node) && node.name && node.modifiers?.some(m => m.kind === ts.SyntaxKind.ExportKeyword)) info.exports.set(node.name.text, node)
    if (ts.isVariableStatement(node) && node.modifiers?.some(m => m.kind === ts.SyntaxKind.ExportKeyword))
      for (const declaration of node.declarationList.declarations) if (ts.isIdentifier(declaration.name)) info.exports.set(declaration.name.text, declaration.initializer)
    if (ts.isBinaryExpression(node) && node.operatorToken.kind === ts.SyntaxKind.EqualsToken) {
      if (ts.isPropertyAccessExpression(node.left) && node.left.expression.getText(tree) === 'exports' && node.right.kind !== ts.SyntaxKind.VoidExpression)
        info.exports.set(node.left.name.text, node.right)
      if (ts.isElementAccessExpression(node.left) && ts.isIdentifier(node.left.expression) && ts.isStringLiteral(node.left.argumentExpression) && ts.isStringLiteral(node.right)) {
        const enumName = node.left.expression.text
        if (!info.enums.has(enumName)) info.enums.set(enumName, {})
        info.enums.get(enumName)[node.left.argumentExpression.text] = node.right.text
      }
    }
    ts.forEachChild(node, scan)
  }
  scan(tree)
  return info
}
const enumFile = path.join(path.dirname(framework.resolve('@medusajs/utils')), 'modules-sdk/define-policies.js')
const policyInfo = moduleInfo(enumFile)
const defaultOperations = policyInfo.vars.get('defaultOperations')
assert(ts.isArrayLiteralExpression(defaultOperations), 'Actual defaultOperations array missing')
const operationEnum = { ALL: policyInfo.exports.get('WILDCARD')?.text }
for (const node of defaultOperations.elements) {
  const operation = ts.isStringLiteral(node) ? node.text : node.getText(policyInfo.tree) === 'exports.WILDCARD' ? operationEnum.ALL : undefined
  assert(operation && ['read', 'create', 'update', 'delete', '*'].includes(operation), 'Unreviewed dynamic operation')
  operationEnum[operation] = operation
}
function imported(node, info) {
  node = unwrap(node)
  if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) && node.expression.text === 'require' && ts.isStringLiteral(node.arguments[0])) return node.arguments[0].text
  if (ts.isCallExpression(node) && ['__importStar', '__importDefault'].includes(node.expression.getText(info.tree))) return imported(node.arguments[0], info)
}
function resolveModule(specifier, info) {
  assert(specifier.startsWith('.'), 'Unreviewed package reference: ' + specifier)
  const base = path.resolve(path.dirname(info.file), specifier)
  const file = [base + '.js', base + '.ts', path.join(base, 'index.js')].find(f => fs.existsSync(f))
  assert(file, 'Missing module: ' + specifier)
  return moduleInfo(file)
}
function value(node, info, bindings = {}) {
  node = unwrap(node)
  assert(node, 'Missing AST expression')
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text
  if (node.kind === ts.SyntaxKind.TrueKeyword) return true
  if (node.kind === ts.SyntaxKind.FalseKeyword) return false
  if (ts.isRegularExpressionLiteral(node)) throw new Error('Regex matcher requires explicit review: ' + node.getText(info.tree))
  if (ts.isArrayLiteralExpression(node)) return node.elements.flatMap(element => ts.isSpreadElement(element) ? value(element.expression, info, bindings) : [value(element, info, bindings)])
  if (ts.isIdentifier(node)) {
    if (Object.hasOwn(bindings, node.text)) return bindings[node.text]
    if (info.imports.get(node.text)?.source === '@medusajs/framework/utils' && info.imports.get(node.text).name === 'PolicyOperation') return operationEnum
    if (info.enums.has(node.text)) return info.enums.get(node.text)
    assert(info.vars.has(node.text), 'Unresolved identifier: ' + node.text)
    return value(info.vars.get(node.text), info, bindings)
  }
  if (ts.isPropertyAccessExpression(node)) {
    // Resolve compiled imported enum namespaces and middleware array exports.
    if (ts.isIdentifier(node.expression) && info.vars.has(node.expression.text)) {
      const specifier = imported(info.vars.get(node.expression.text), info)
      if (specifier) {
        if (specifier === '@medusajs/framework/utils' && node.name.text === 'PolicyOperation') return operationEnum
        const target = resolveModule(specifier, info)
        if (target.enums.has(node.name.text)) return target.enums.get(node.name.text)
        assert(target.exports.has(node.name.text), 'Missing export: ' + specifier + ':' + node.name.text)
        return value(target.exports.get(node.name.text), target)
      }
    }
    const parent = value(node.expression, info, bindings)
    assert(Object.hasOwn(parent, node.name.text), 'Unresolved property: ' + node.getText(info.tree))
    return parent[node.name.text]
  }
  if (ts.isObjectLiteralExpression(node)) {
    if (prop(node, 'matcher')) {
      const matcher = value(prop(node, 'matcher'), info, bindings)
      const rawMethods = prop(node, 'methods') || prop(node, 'method')
      const method = rawMethods ? value(rawMethods, info, bindings) : null
      const policies = prop(node, 'policies') ? value(prop(node, 'policies'), info, bindings) : []
      const entry = { source: relative(info.file), line: info.tree.getLineAndCharacterOfPosition(node.getStart(info.tree)).line + 1,
        matcher, methods: method === null ? null : Array.isArray(method) ? method : [method], policies,
        middleware_expressions: prop(node, 'middlewares')?.getText(info.tree) || null }
      registrations.push(entry)
      rawMethodDeclarations.push({ ...(prop(node, 'method') ? { method: value(prop(node, 'method'), info, bindings) } : {}),
        ...(prop(node, 'methods') ? { methods: value(prop(node, 'methods'), info, bindings) } : {}) })
      return entry
    }
    return Object.fromEntries(node.properties.map(p => {
      assert(ts.isPropertyAssignment(p), 'Unreviewed policy shape')
      return [p.name.getText().replaceAll('"', '').replaceAll("'", ''), value(p.initializer, info, bindings)]
    }))
  }
  if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression) && node.expression.name.text === 'map') {
    const inputs = value(node.expression.expression, info, bindings), fn = node.arguments[0]
    assert(ts.isArrowFunction(fn) && fn.parameters.length === 1 && ts.isIdentifier(fn.parameters[0].name))
    return inputs.map(input => value(fn.body, info, { ...bindings, [fn.parameters[0].name.text]: input }))
  }
  throw new Error('Unreviewed expression at ' + relative(info.file) + ': ' + node.getText(info.tree).slice(0, 100))
}
function rootRegistrations(file) {
  const info = moduleInfo(path.join(root, file))
  let entry = info.exports.get('default')
  if (!entry) entry = info.tree.statements.find(s => ts.isExportAssignment(s))?.expression
  assert(ts.isCallExpression(entry), 'Middleware default must call defineMiddlewares')
  let callee = unwrap(entry.expression)
  if (ts.isBinaryExpression(callee) && callee.operatorToken.kind === ts.SyntaxKind.CommaToken) callee = callee.right
  assert((ts.isIdentifier(callee) && callee.text === 'defineMiddlewares') || (ts.isPropertyAccessExpression(callee) && callee.name.text === 'defineMiddlewares'), 'Unreviewed middleware helper')
  const argument = entry.arguments[0]
  return value(ts.isArrayLiteralExpression(argument) ? argument : prop(argument, 'routes'), info)
}
const scannerCases = []
function scannerCase(name, run) { run(); scannerCases.push({ name, passed: true }) }
function artificialExpression(expression) {
  const tree = ts.createSourceFile('artificial.ts', `const fixture = ${expression}`, ts.ScriptTarget.Latest, true)
  return value(tree.statements[0].declarationList.declarations[0].initializer,
    { file: path.join(root, 'artificial.ts'), tree, vars: new Map(), imports: new Map(), enums: new Map() })
}
scannerCase('literal-array-preserves-all-methods', () => assert.deepEqual(artificialExpression('["POST", "DELETE"]'), ['POST', 'DELETE']))
scannerCase('unknown-identifier-fails-not-silently-ignored', () => assert.throws(() => artificialExpression('unknownPolicies'), /Unresolved identifier/))
scannerCase('dynamic-call-fails-without-executing', () => assert.throws(() => artificialExpression('loadDynamicPolicies()'), /Unreviewed expression/))
scannerCase('regex-matcher-requires-review-not-string-coercion', () => assert.throws(() => artificialExpression('/^admin/'), /Regex matcher requires explicit review/))
scannerCase('global-use-prefix-matches-nested-route', () => assert(pathToRegexp('/admin/orders', [], { end: false }).test('/admin/orders/fixture')))
scannerCase('global-prefix-does-not-match-sibling-name', () => assert(!pathToRegexp('/admin/orders', [], { end: false }).test('/admin/orders-extra')))
scannerCase('method-specific-exact-route-does-not-match-nested', () => assert(!pathToRegexp('/admin/orders', [], { end: true }).test('/admin/orders/fixture')))
scannerCase('wildcard-matches-nested-but-not-bare-root', () => {
  const compiled = pathToRegexp('/admin/orders/*', [], { end: false })
  assert(compiled.test('/admin/orders/fixture')); assert(!compiled.test('/admin/orders'))
})
// Normalize through the actual pure helper, independently of our field extraction.
const helperRecord = { exports: {} }
vm.runInNewContext(`(function(exports){${read(path.join(root, frameworkBase, 'utils/define-middlewares.js'))}\n})`, {}, { timeout: 3000 })(helperRecord.exports)
const nativeEntries = rootRegistrations(native + '/middlewares.js')
const customEntries = rootRegistrations(custom + '/middlewares.ts')
let pluginEntries = []
if (fs.existsSync(path.join(root, plugin, 'middlewares.js'))) pluginEntries = rootRegistrations(plugin + '/middlewares.js')
const normalized = helperRecord.exports.defineMiddlewares(rawMethodDeclarations)
assert.equal(normalized.routes.length, registrations.length)
for (let i = 0; i < normalized.routes.length; i++) assert.deepEqual(JSON.parse(JSON.stringify(normalized.routes[i].methods ?? null)), registrations[i].methods)
// Retain exact installed loader/router sources as evidence for authentication,
// policy-only no-op middleware and source override semantics. No app is loaded.
for (const file of ['router.js', 'routes-loader.js', 'middleware-file-loader.js', 'routes-sorter.js']) read(path.join(root, frameworkBase, file))
read(matcherFile)
const methodInfo = moduleInfo(path.join(root, frameworkBase, 'types.js'))
const httpMethods = value(methodInfo.exports.get('HTTP_METHODS'), methodInfo)
const compiled = registrations.map(entry => pathToRegexp(entry.matcher, [], { end: entry.methods !== null }))
const handlers = []
function scanHandlers(directory, kind) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name, 'en'))) {
    const file = path.join(directory, entry.name)
    if (entry.isDirectory()) scanHandlers(file, kind)
    else if (entry.name === 'route.ts' || entry.name === 'route.js') {
      const info = moduleInfo(file)
      const baseDirectory = path.join(root, kind === 'native' ? native : kind === 'custom' ? custom : plugin)
      const routePath = '/' + path.relative(baseDirectory, path.dirname(file)).replaceAll('\\', '/').replace(/\[(\w+)\]/g, ':$1')
      const example = routePath.replace(/:\w+/g, 'fixture-id')
      const authenticate = info.exports.has('AUTHENTICATE') ? value(info.exports.get('AUTHENTICATE'), info) : true
      assert.equal(typeof authenticate, 'boolean', 'Dynamic auth flag requires review')
      for (const method of httpMethods.filter(name => info.exports.has(name))) {
        const node = info.exports.get(method)
        const fn = ts.isIdentifier(node) ? info.vars.get(node.text) : node
        assert(fn && (ts.isFunctionDeclaration(fn) || ts.isArrowFunction(fn) || ts.isFunctionExpression(fn)), 'Handler export is not proven function: ' + relative(file))
        const matches = registrations.flatMap((entry, index) => (entry.methods === null || entry.methods.includes(method) || entry.methods.includes('ALL')) && compiled[index].test(example) ? [index] : [])
        handlers.push({ source: relative(file), kind, method, path: routePath, representative_path: example,
          route_loader_authenticate: authenticate,
          auth_layer: routePath.startsWith('/admin/') || routePath === '/admin' ? (authenticate ? 'default-admin-user-session-bearer-api-key-plus-middleware' : 'admin-route-opted-out-check-explicit-middleware')
            : routePath.startsWith('/store/') ? 'publishable-key-plus-optional-customer-and-explicit-middleware' : 'check-explicit-auth-or-capability-middleware',
          matching_registration_indexes: matches,
          matched_policy_keys: [...new Set(matches.flatMap(index => registrations[index].policies.map(policy => `${policy.resource}:${policy.operation}`)))].sort(),
        })
      }
    }
  }
}
for (const [directory, kind] of [[native, 'native'], [custom, 'custom'], [plugin, 'plugin']]) scanHandlers(path.join(root, directory), kind)
const customHandlers = handlers.filter(entry => entry.kind === 'custom')
assert.equal(customHandlers.length, 86, 'Reviewed custom method count drifted')
const duplicateHandlers = [...Map.groupBy(handlers, entry => `${entry.method} ${entry.path}`).entries()]
  .filter(([, entries]) => entries.length > 1).map(([method_path, entries]) => ({ method_path, sources: entries.map(entry => entry.source),
    status: 'multiple-source-candidates-effective-loader-precedence-not-proven' }))
const output = { format_version: 1,
  scope: 'AST traversal of root-registered middleware arrays, actual pure defineMiddlewares normalization, and installed Express matcher applied to representative handler paths. Static candidate coverage only; no HTTP, role/session, source override/router sorting or SQL enforcement acceptance.',
  roots: { native: nativeEntries.length, custom: customEntries.length, plugin: pluginEntries.length },
  registrations, handlers, duplicate_handlers: duplicateHandlers, scanner_cases: scannerCases, source_sha256: hashes,
  remaining: ['Representative paths do not prove all dynamic path values or effective source override/order', 'Runtime sorting/authentication, actual grants, field filters and plugin/dynamic registration verification', 'Gin explicit permissions, scopes and allowlisted DTOs'] }
const out = path.join(root, 'docs/migration/route-policy-registrations.json')
const content = JSON.stringify(output, null, 2) + '\n'
if (process.argv.includes('--check')) assert.equal(fs.readFileSync(out, 'utf8').replaceAll('\r\n', '\n'), content)
else fs.writeFileSync(out, content)
console.log(JSON.stringify({ registrations: registrations.length, native: nativeEntries.length, custom: customEntries.length,
  policy_registrations: registrations.filter(e => e.policies.length).length, source_files: Object.keys(hashes).length,
  handler_methods: handlers.length, custom_methods: customHandlers.length,
  custom_with_matched_policy: customHandlers.filter(entry => entry.matched_policy_keys.length).length,
  duplicate_handler_groups: duplicateHandlers.length, scanner_cases: scannerCases.length,
  mode: process.argv.includes('--check') ? 'check' : 'generate' }))
