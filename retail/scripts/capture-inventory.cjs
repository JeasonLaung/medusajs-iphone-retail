// Only invokes the installed pure inventory-input preparation helper.
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const assert = require('node:assert/strict')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backendRequire = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const medusaRequire = createRequire(backendRequire.resolve('@medusajs/medusa/core-flows'))
const flowsRoot = path.dirname(medusaRequire.resolve('@medusajs/core-flows'))
const helperPath = path.join(flowsRoot, 'cart/utils/prepare-confirm-inventory-input.js')
const { prepareConfirmInventoryInput } = require(helperPath)

const channel = 'fixture-channel'
const level = (location, stocked, reserved = '0', linkedChannel = channel, extra = {}) => ({
  location_id: location, stocked_quantity: stocked, reserved_quantity: reserved,
  stock_locations: { id: location, sales_channels: [{ id: linkedChannel }] }, ...extra,
})
const variant = (levels, extra = {}) => ({
  id: 'fixture-variant', manage_inventory: true, allow_backorder: false,
  inventory_items: [{ variant_id: 'fixture-variant', inventory_item_id: 'fixture-inventory', required_quantity: '1', inventory: { location_levels: levels } }], ...extra,
})
const input = (variants, quantity = '2', extra = {}) => ({ input: {
  sales_channel_id: channel, variants,
  items: [{ id: 'fixture-line', variant_id: 'fixture-variant', quantity }], ...extra,
} })
const expectedRow = (locations, extra = {}) => ({
  id: 'fixture-line', inventory_item_id: 'fixture-inventory', required_quantity: '1',
  allow_backorder: false, quantity: '2', location_ids: locations, ...extra,
})
const cases = [
  ['unmanaged', input([variant([], { manage_inventory: false })]), { items: [] }],
  ['available', input([variant([level('fixture-a', '5', '1')])]), { items: [expectedRow(['fixture-a'])] }],
  ['reserved-shortfall-is-preparation-only', input([variant([level('fixture-a', '5', '4')])]), { items: [expectedRow(['fixture-a'])] }],
  ['prefer-location-with-full-coverage', input([variant([level('fixture-a', '1'), level('fixture-b', '5')])]), { items: [expectedRow(['fixture-b'])] }],
  ['split-stock-falls-back-to-existing-levels', input([variant([level('fixture-a', '1'), level('fixture-b', '1')])]), { items: [expectedRow(['fixture-a', 'fixture-b'])] }],
  ['foreign-channel-denied', input([variant([level('fixture-a', '10', '0', 'fixture-foreign')])]), { error_contains: 'not associated with any stock location' }],
  ['backorder-without-channel-location', input([variant([level('fixture-a', '0', '0', 'fixture-foreign')], { allow_backorder: true })]), { items: [expectedRow([], { allow_backorder: true })] }],
  ['item-backorder-override', input([variant([level('fixture-a', '0')])], '2', { items: [{ id: 'fixture-line', variant_id: 'fixture-variant', quantity: '2', allow_backorder: true }] }), { items: [expectedRow(['fixture-a'], { allow_backorder: true })] }],
  ['raw-quantity-precedence', input([variant([level('fixture-a', '10', '0', channel, { raw_stocked_quantity: { value: '1', precision: 20 } }), level('fixture-b', '5')])]), { items: [expectedRow(['fixture-b'])] }],
  ['required-quantity', input([variant([level('fixture-a', '3'), level('fixture-b', '4')], { inventory_items: [{ variant_id: 'fixture-variant', inventory_item_id: 'fixture-inventory', required_quantity: '2', inventory: { location_levels: [level('fixture-a', '3'), level('fixture-b', '4')] } }] })]), { items: [expectedRow(['fixture-b'], { required_quantity: '2' })] }],
]
const output = { format_version: 1, scope: 'Artificial input-preparation fixtures only; not availability confirmation, reservation, concurrency or checkout acceptance',
  sources: { [path.relative(root, helperPath).replaceAll(path.sep, '/')]: crypto.createHash('sha256').update(fs.readFileSync(helperPath)).digest('hex') }, cases: [] }
for (const [name, fixture, expected] of cases) {
  let actual
  try { actual = prepareConfirmInventoryInput(structuredClone(fixture)) }
  catch (error) { actual = { error_type: error.type, message: error.message } }
  if (expected.error_contains) assert.ok(actual.message?.includes(expected.error_contains), name)
  else assert.deepEqual(actual, expected, name)
  output.cases.push({ name, input: fixture, expected: actual })
}
const target = path.join(root, 'docs/migration/fixtures/legacy-inventory-input.json')
fs.mkdirSync(path.dirname(target), { recursive: true })
const content = JSON.stringify(output, null, 2) + '\n'
if (process.argv.includes('--check')) assert.equal(fs.readFileSync(target, 'utf8').replaceAll('\r\n', '\n'), content, 'Inventory oracle changed; review before refresh')
else fs.writeFileSync(target, content)
console.log(JSON.stringify({ cases: cases.length, independent_assertions: cases.length, mode: process.argv.includes('--check') ? 'check' : 'capture' }))
