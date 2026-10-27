// Capture legacy request boundaries using pure validators only. Never load app/env/services.
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backendRoot = path.join(root, 'medusa-store/apps/backend')
const backend = createRequire(path.join(backendRoot, 'package.json'))
const ts = backend('typescript')
const zod = backend('@medusajs/framework/zod')
const apiRoot = path.join(backendRoot, 'src/api')
const cache = new Map(), hashes = {}, specialRules = []
const relative = file => path.relative(root, file).replaceAll('\\', '/')
function files(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const file = path.join(dir, entry.name)
    return entry.isDirectory() ? files(file) : [file]
  })
}
const validators = files(apiRoot).filter(file => file.endsWith('validators.ts')).sort()
const allowed = new Set([...validators, ...['recycled-assessment.ts', 'buyback-pricing.ts'].map(f => path.join(backendRoot, 'src/lib', f))])
function load(file) {
  file = path.resolve(file)
  assert(allowed.has(file), 'Pure-source allowlist rejected dependency')
  if (cache.has(file)) return cache.get(file)
  const text = fs.readFileSync(file, 'utf8')
  hashes[relative(file)] = crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex')
  const ast = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true)
  function visit(node) {
    if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression)) {
      const method = node.expression.name.text
      if (['refine', 'superRefine', 'transform', 'trim', 'toUpperCase', 'toLowerCase', 'preprocess'].includes(method)) {
        specialRules.push({ source: relative(file), line: ast.getLineAndCharacterOfPosition(node.getStart()).line + 1,
          method, arguments: node.arguments.map(arg => arg.getText(ast)),
          disposition: 'Must reproduce in Gin and test independently; JSON Schema alone does not encode this behavior' })
      }
    }
    ts.forEachChild(node, visit)
  }
  visit(ast)
  const module = { exports: {} }
  cache.set(file, module.exports)
  const requirePure = name => {
    if (name === '@medusajs/framework/zod') return zod
    assert(name.startsWith('.'), 'External dependency rejected')
    return load(path.resolve(path.dirname(file), name + '.ts'))
  }
  const js = ts.transpileModule(text, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText
  vm.runInNewContext(`(function(require,module,exports){${js}\n})`, {}, { filename: relative(file), timeout: 3000 })(requirePure, module, module.exports)
  return module.exports
}
const schemas = []
for (const file of validators) {
  for (const [name, schema] of Object.entries(load(file))) {
    assert(typeof schema.safeParse === 'function', `Unexpected validator export ${name}`)
    schemas.push({ source: relative(file), export: name,
      schema: zod.z.toJSONSchema(schema, { io: 'input', target: 'draft-2020-12' }),
      status: 'legacy-input-shape-only; not Gin contract or runtime authorization' })
  }
}
const result = { format_version: 1,
  scope: 'Legacy JSON input shapes only; refinements, transforms, coercion/default output, workflow state, authorization and files require separate implementation. This is not a new API acceptance contract.',
  source_sha256: Object.fromEntries(Object.entries(hashes).sort()),
  special_rules_not_represented_by_json_schema: specialRules.sort((a,b) => a.source.localeCompare(b.source) || a.line-b.line),
  schemas }
const cases = []
function check(file, name, label, input, accepted, expected) {
  const schema = load(path.join(apiRoot, file))[name]
  const parsed = schema.safeParse(input)
  assert.equal(parsed.success, accepted, `${name}/${label}`)
  if (accepted && expected !== undefined) assert.deepEqual(JSON.parse(JSON.stringify(parsed.data)), expected, `${name}/${label}: normalized output`)
  cases.push({ validator: name, source: relative(path.join(apiRoot, file)), name: label, input,
    accepted, ...(accepted ? { output: JSON.parse(JSON.stringify(parsed.data)) } : { issues: parsed.error.issues.map(i => ({ path: i.path, code: i.code })) }) })
}
const review = { product_id: 'p', rating: 5, title: 'Good', content: 'Works very well.' }
check('review-validators.ts', 'CreateReview', 'trim-input', { ...review, title: ' Good ' }, true, review)
check('review-validators.ts', 'CreateReview', 'rating-zero', { ...review, rating: 0 }, false)
check('review-validators.ts', 'CreateReview', 'rating-string-not-integer', { ...review, rating: '5' }, false)
check('review-validators.ts', 'ReviewListQuery', 'coerce-pagination', { product_id: 'p', limit: '20', offset: '0' }, true, { product_id: 'p', limit: 20, offset: 0 })
check('review-validators.ts', 'ReviewListQuery', 'review-limit-21-rejected', { product_id: 'p', limit: '21' }, false)
check('review-validators.ts', 'CreateReview', 'legacy-strips-extra-ownership-field', { ...review, customer_id: 'other' }, true, review)
check('admin/warehouse-access/validators.ts', 'WarehouseBindingBody', 'duplicate-locations', { user_id: 'u', scope: 'bound', location_ids: ['l', 'l'] }, false)
check('admin/warehouse-access/validators.ts', 'WarehouseBindingBody', 'empty-bound-is-not-all', { user_id: 'u', scope: 'bound', location_ids: [] }, true,
  { user_id: 'u', scope: 'bound', location_ids: [] })
const device = { variant_id: 'v', source_id: 's', serial_number: ' abc ' }
check('admin/devices/validators.ts', 'IntakeDevice', 'normalization-and-defaults', device, true,
  { ...device, serial_number: 'ABC', category: 'sale', photos: [], cost_cents: 0, currency_code: 'eur', bin: '', notes: '' })
check('admin/devices/validators.ts', 'IntakeDevice', 'missing-serial-refinement', { variant_id: 'v', source_id: 's' }, false)
check('admin/devices/validators.ts', 'IntakeDevice', 'imei-preserves-leading-zero', { ...device, imei: '001234567890123' }, true)
check('admin/devices/validators.ts', 'IntakeDevice', 'imei-short', { ...device, imei: '123' }, false)
check('admin/devices/validators.ts', 'IntakeDevice', 'strict-unknown-field', { ...device, role: 'admin' }, false)
check('admin/devices/validators.ts', 'DeviceCommand', 'duplicate-availability-devices', { action: 'make_available', device_ids: ['d', 'd'] }, false)
check('buyback-validators.ts', 'BuybackReferencePrice', 'legacy-money-number', { product_id: 'p', capacity: '128GB', amount: 0 }, true)
check('buyback-validators.ts', 'BuybackReferencePrice', 'legacy-money-string-rejected', { product_id: 'p', capacity: '128GB', amount: '0' }, false)
check('buyback-validators.ts', 'BuybackCustomerCommand', 'unknown-action', { action: 'mark_paid' }, false)
check('buyback-validators.ts', 'BuybackPhotoUpload', 'invalid-base64', { item_id: 'i', photo_type: 'front', mime_type: 'image/png', content: '!!!!' }, false)
check('store/orders/[id]/after-sales/validators.ts', 'CreateAfterSales', 'reject-quantity-zero', { type: 'repair', reason: 'A broken device', items: [{ item_id: 'i', quantity: 0 }] }, false)
check('admin/mail/validators.ts', 'MailListQuery', 'pagination-over-limit', { limit: 101 }, false)
check('admin/mail/validators.ts', 'MailCommand', 'campaign-operation-boundary', { action: 'campaign', id: 'c', operation: 'delete' }, false)
check('seo-validators.ts', 'SeoCommandBody', 'internal-tick-is-not-browser-command', { action: 'tick' }, false)
function write(file, value) {
  const target = path.join(root, file), content = JSON.stringify(value, null, 2) + '\n'
  if (process.argv.includes('--check')) assert.equal(fs.readFileSync(target, 'utf8').replaceAll('\r\n', '\n'), content, `${file} changed`)
  else { fs.mkdirSync(path.dirname(target), { recursive: true }); fs.writeFileSync(target, content) }
}
write('docs/migration/schemas/legacy-request-inputs.json', result)
write('docs/migration/fixtures/legacy-request-validation.json', { format_version: 1,
  scope: 'Artificial legacy input parsing cases with independent accepted/normalized expectations; not business state, auth, real files or new Gin acceptance', source_sha256: result.source_sha256, cases })
console.log(JSON.stringify({ files: validators.length, schemas: schemas.length, special_rules: specialRules.length, cases: cases.length, mode: process.argv.includes('--check') ? 'check' : 'generate' }))
