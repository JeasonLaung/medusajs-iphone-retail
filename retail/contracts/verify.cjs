// Pure JSON Schema checks. No old application dependency, database or server.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const Ajv = require('ajv/dist/2020')
const addFormats = require('ajv-formats')
const root = path.resolve(__dirname, '../..')
const ajv = new Ajv({ allErrors: true, strict: true, validateFormats: true, coerceTypes: false, useDefaults: false, removeAdditional: false })
addFormats(ajv)
for (const file of ['gin-common.schema.json', 'supplier-accounts.schema.json']) {
  const schema = JSON.parse(fs.readFileSync(path.join(root, 'docs/migration/schemas', file), 'utf8'))
  assert(ajv.validateSchema(schema), `Invalid JSON Schema ${file}`)
  ajv.addSchema(schema)
}
const definitions = ['Email', 'OrderIDsInput', 'CreateRequest', 'BindingRequest', 'Account', 'DetailResponse', 'ListQuery', 'ListResponse']
for (const definition of definitions) ajv.compile({ $ref: `urn:eurorefurb:api:supplier-accounts#/$defs/${definition}` })
const operations = JSON.parse(fs.readFileSync(path.join(root, 'docs/migration/schemas/supplier-account-operations.json'), 'utf8'))
const ids = new Set(), methods = new Set()
const compileRef = reference => ajv.compile({ $ref: reference.startsWith('#') ? `urn:eurorefurb:api:supplier-accounts${reference}` : reference })
compileRef(operations.error_schema)
Object.values(operations.request_header_schemas).forEach(compileRef)
for (const operation of operations.operations) {
  assert(!ids.has(operation.id), 'Duplicate operation ID'); ids.add(operation.id)
  const key = `${operation.method} ${operation.path}`
  assert(!methods.has(key), 'Duplicate method/path'); methods.add(key)
  assert(operation.permission && operation.cache === 'private,no-store')
  compileRef(operation.response_schema)
  if (operation.query_schema) compileRef(operation.query_schema)
  const params = [...operation.path.matchAll(/\{([^}]+)\}/g)].map(match => match[1]).sort()
  assert.deepEqual(Object.keys(operation.path_parameters || {}).sort(), params, 'Path schema missing')
  Object.values(operation.path_parameters || {}).forEach(compileRef)
  if (!['GET', 'HEAD'].includes(operation.method)) {
    compileRef(operation.body_schema)
    for (const name of ['Origin', 'X-CSRF-Token', 'Idempotency-Key']) assert(operation.required_headers.includes(name))
    assert.equal(operation.pending_status, 202); compileRef(operation.pending_response_schema)
  }
}
const errorCodes = new Set()
for (const error of operations.errors) {
  assert(!errorCodes.has(error.code), 'Duplicate domain error code'); errorCodes.add(error.code)
  assert(error.status >= 400 && error.status <= 599)
  assert(compileRef(operations.error_schema)({ error: { code: error.code, message: 'Artificial message.', request_id: 'req_fixture' } }))
}
const fixtures = JSON.parse(fs.readFileSync(path.join(root, 'docs/migration/fixtures/gin-supplier-contract-cases.json'), 'utf8'))
for (const sample of fixtures.cases) {
  const validate = ajv.compile({ $ref: sample.schema })
  assert.equal(validate(sample.input), sample.accepted, `${sample.name}: ${JSON.stringify(validate.errors)}`)
}
// Exercise unbounded collections at their actual boundary without bloating the
// checked-in fixtures with hundreds of repeated IDs or private credential data.
const create = { email: 'supplier@example.invalid', password: 'fixture-only-password', source_id: 'source-fixture', order_ids: [] }
const validateCreate = ajv.compile({ $ref: 'urn:eurorefurb:api:supplier-accounts#/$defs/CreateRequest' })
const boundaries = [
  ['password-11', { ...create, password: 'x'.repeat(11) }, false],
  ['password-12', { ...create, password: 'x'.repeat(12) }, true],
  ['password-128', { ...create, password: 'x'.repeat(128) }, true],
  ['password-129', { ...create, password: 'x'.repeat(129) }, false],
  ['password-unicode-code-points-12', { ...create, password: '😀'.repeat(12) }, true],
  ['password-unicode-code-points-11', { ...create, password: '😀'.repeat(11) }, false],
  ['200-orders', { ...create, order_ids: Array.from({ length: 200 }, (_, i) => `order-${i}`) }, true],
  ['201-orders', { ...create, order_ids: Array.from({ length: 201 }, (_, i) => `order-${i}`) }, false],
  ['ID-191', { ...create, source_id: 's'.repeat(191) }, true],
  ['ID-192', { ...create, source_id: 's'.repeat(192) }, false],
]
for (const [name, input, expected] of boundaries) assert.equal(validateCreate(input), expected, name)
for (const [type, value] of [['ID', 'staff-fixture'], ['Revision', '1'], ['EntityTag', '"1"'],
  ['RequestID', 'req_fixture'], ['OperationKey', 'operation_fixture_123'], ['CSRFToken', 'x'.repeat(32)]]) {
  const validate = compileRef(`urn:eurorefurb:api:common#/$defs/${type}`)
  assert.equal(validate(value), true, type)
  assert.equal(validate(value + '\n'), false, `${type}-newline`)
}
// Schemas validate shape, not authorization or cross-field business invariants.
// Prove that the checker does not silently claim to enforce those invariants.
const account = { staff_id: 'staff-fixture', email: create.email, source_id: create.source_id,
  order_ids: [], enabled: true, access_mode: 'supplier', version: '1', permission_version: '1',
  created_at: '2026-10-27T00:00:00.000001Z', updated_at: '2026-10-27T00:00:00.000001Z' }
const accountValidator = ajv.compile({ $ref: 'urn:eurorefurb:api:supplier-accounts#/$defs/Account' })
assert.equal(accountValidator(account), true)
for (const field of ['password', 'password_hash', 'metadata', 'auth_identity_id', 'session']) {
  assert.equal(accountValidator({ ...account, [field]: 'must-never-return' }), false, `Response leaked ${field}`)
}
const listValidator = ajv.compile({ $ref: 'urn:eurorefurb:api:supplier-accounts#/$defs/ListResponse' })
assert.equal(listValidator({ accounts: [account], count: 1, limit: 20, offset: 0 }), true)
assert.equal(listValidator({ accounts: [{ ...account, order_ids: ['order-a', 'order-a'] }], count: 1, limit: 20, offset: 0 }), false)
console.log(JSON.stringify({ schema_definitions_compiled: definitions.length, fixture_cases: fixtures.cases.length,
  generated_boundary_cases: boundaries.length, response_projection_cases: 8,
  operation_definitions: ids.size, error_codes: errorCodes.size,
  header_identifier_boundary_cases: 12,
  scope: 'wire schema only; no Gin/auth/MySQL/HTTP acceptance' }))
