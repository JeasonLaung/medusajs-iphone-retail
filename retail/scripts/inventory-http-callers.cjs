// Static AST evidence only: never execute frontend or backend application code.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backend = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const ts = backend('typescript')
function scan(text, filename) {
  const ast = ts.createSourceFile(filename, text, ts.ScriptTarget.Latest, true, filename.endsWith('.tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS)
  const rows = []
  function bindings(node, outer) {
    const scope = new Map(outer)
    for (const stmt of node.statements || []) {
      if (!ts.isVariableStatement(stmt) || !(stmt.declarationList.flags & ts.NodeFlags.Const)) continue
      for (const decl of stmt.declarationList.declarations) {
        if (ts.isIdentifier(decl.name)) scope.set(decl.name.text, decl.initializer)
      }
    }
    // Function parameters shadow file constants; their runtime values remain unknown.
    for (const param of node.parameters || []) if (ts.isIdentifier(param.name)) scope.set(param.name.text, undefined)
    return scope
  }
  function resolve(node, scope, seen = new Set()) {
    if (!node) return null
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text
    if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node)) return resolve(node.expression, scope, seen)
    if (ts.isIdentifier(node) && scope.has(node.text) && !seen.has(node.text)) {
      const next = new Set(seen); next.add(node.text)
      return resolve(scope.get(node.text), scope, next)
    }
    if (ts.isTemplateExpression(node)) {
      let result = node.head.text
      for (const span of node.templateSpans) result += (resolve(span.expression, scope, seen) ?? '{' + span.expression.getText(ast) + '}') + span.literal.text
      return result
    }
    if (ts.isBinaryExpression(node) && node.operatorToken.kind === ts.SyntaxKind.PlusToken) {
      const left = resolve(node.left, scope, seen), right = resolve(node.right, scope, seen)
      return left !== null && right !== null ? left + right : null
    }
    return null
  }
  function option(node, scope, seen = new Set()) {
    if (!node) return { methods: ['GET'], method_evidence: 'fetch default; no options', fields: [] }
    if (ts.isIdentifier(node) && scope.has(node.text) && !seen.has(node.text)) {
      const next = new Set(seen); next.add(node.text)
      if (!scope.get(node.text)) return { methods: [], method_evidence: 'runtime parameter options; review required', fields: [] }
      return option(scope.get(node.text), scope, next)
    }
    if (!ts.isObjectLiteralExpression(node)) return { methods: [], method_evidence: 'dynamic options; review required', fields: [] }
    const fields = node.properties.filter(ts.isPropertyAssignment).map(p => p.name.getText(ast).replace(/^['"]|['"]$/g, ''))
    const method = node.properties.filter(ts.isPropertyAssignment).find(p => p.name.getText(ast).replace(/^['"]|['"]$/g, '') === 'method')
    if (node.properties.some(ts.isSpreadAssignment)) return { methods: [], method_evidence: 'spread may override method; review required', fields }
    if (!method) return { methods: ['GET'], method_evidence: 'object has no method/spread', fields }
    const literal = resolve(method.initializer, scope)
    return { methods: literal ? [literal.toUpperCase()] : [], method_evidence: literal ? 'static method property' : 'dynamic method property; review required', fields }
  }
  function visit(node, outer) {
    const scope = bindings(node, outer)
    if (ts.isCallExpression(node) && ((ts.isIdentifier(node.expression) && node.expression.text === 'fetch') ||
      (ts.isPropertyAccessExpression(node.expression) && node.expression.name.text === 'fetch'))) {
      const argument = node.arguments[0]
      const resolved = resolve(argument, scope)
      rows.push({ source: filename, line: ast.getLineAndCharacterOfPosition(node.getStart()).line + 1,
        callee: node.expression.getText(ast), endpoint_expression: argument?.getText(ast) ?? '',
        endpoint_pattern: resolved, endpoint_resolution: resolved === null ? 'dynamic-review-required' : resolved.includes('{') ? 'static-prefix-with-runtime-segments' : 'static',
        ...option(node.arguments[1], scope),
        status: 'source-candidate-only; not reachability, authorization or new API mapping' })
    }
    ts.forEachChild(node, child => visit(child, scope))
  }
  visit(ast, new Map())
  return rows
}
function files(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const target = path.join(dir, entry.name)
    return entry.isDirectory() ? files(target) : /\.(ts|tsx)$/.test(entry.name) ? [target] : []
  })
}
function main() {
  const folders = ['medusa-store/apps/storefront/src', 'medusa-store/apps/backend/src/admin']
  const hashes = {}, calls = []
  for (const folder of folders) {
    for (const file of files(path.join(root, folder)).sort()) {
      const relative = path.relative(root, file).replaceAll('\\', '/')
      const rows = scan(fs.readFileSync(file, 'utf8'), relative)
      if (!rows.length) continue
      hashes[relative] = crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex')
      calls.push(...rows)
    }
  }
  const result = { format_version: 1,
    scope: 'AST candidates for calls named fetch across storefront/admin source. Includes wrappers and external URLs; excludes SDK resource methods and calls under different names. No app execution. Unknown paths/methods remain unresolved.',
    source_sha256: hashes, calls }
  const target = path.join(root, 'docs/migration/http-caller-manifest.json')
  const content = JSON.stringify(result, null, 2) + '\n'
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(target, 'utf8').replaceAll('\r\n', '\n'), content, 'HTTP caller evidence drift')
  else fs.writeFileSync(target, content)
  console.log(JSON.stringify({ files: Object.keys(hashes).length, candidates: calls.length,
    dynamic_endpoints: calls.filter(c => c.endpoint_pattern === null).length,
    unresolved_methods: calls.filter(c => !c.methods.length).length, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
}
module.exports = { scan }
if (require.main === module) main()
