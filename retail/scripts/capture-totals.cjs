// Pure arithmetic oracle: artificial examples only; no app boot, env, DB or HTTP.
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const { createRequire } = require('node:module')

const root = path.resolve(__dirname, '../..')
const backendRequire = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const frameworkRequire = createRequire(backendRequire.resolve('@medusajs/framework/utils'))
const utilsRoot = path.dirname(frameworkRequire.resolve('@medusajs/utils'))
const { decorateCartTotals } = require(path.join(utilsRoot, 'totals/cart/index.js'))
const tax = rate => [{ rate }]
const item = (price, quantity, inclusive, rate = '25', extra = {}) => ({
  id: 'fixture-item', unit_price: price, quantity, is_tax_inclusive: inclusive,
  tax_lines: tax(rate), adjustments: [], ...extra,
})
const shipping = (amount, inclusive, extra = {}) => ({
  id: 'fixture-shipping', amount, is_tax_inclusive: inclusive, tax_lines: tax('25'), adjustments: [], ...extra,
})
const cart = (items = [], shipping_methods = [], extra = {}) => ({
  currency_code: 'eur', region: { automatic_taxes: true }, items, shipping_methods, credit_lines: [], ...extra,
})
const cases = [
  ['empty', cart(), '0.00'],
  ['no-tax', cart([item('99.95', '2', false, '0')]), '199.90'],
  ['exclusive-with-shipping', cart([item('100', '2', false)], [shipping('10', false)]), '262.50'],
  ['inclusive-with-shipping', cart([item('125', '2', true)], [shipping('12.5', true)]), '262.50'],
  ['exclusive-discount', cart([item('100', '2', false, '25', { adjustments: [{ amount: '20', is_tax_inclusive: false }] })]), '225.00'],
  ['inclusive-discount', cart([item('125', '2', true, '25', { adjustments: [{ amount: '25', is_tax_inclusive: true }] })]), '225.00'],
  ['multiple-adjustments', cart([item('100', '2', false, '25', { adjustments: [{ amount: '10', is_tax_inclusive: false }, { amount: '5', is_tax_inclusive: false }] })]), '231.25'],
  ['shipping-discount', cart([item('100', '1', false)], [shipping('10', false, { adjustments: [{ amount: '2', is_tax_inclusive: false }] })]), '135.00'],
  ['received-return', cart([item('100', '3', false, '25', { adjustments: [{ amount: '30', is_tax_inclusive: false }], detail: { return_received_quantity: '1' } })]), '225.00'],
  ['requested-return', cart([item('100', '3', false, '25', { adjustments: [{ amount: '30', is_tax_inclusive: false }], detail: { return_requested_quantity: '1' } })]), '337.50'],
  ['dismissed-return', cart([item('125', '3', true, '25', { adjustments: [{ amount: '37.5', is_tax_inclusive: true }], detail: { return_dismissed_quantity: '1' } })]), '225.00'],
  ['multiple-tax-lines', cart([item('100', '1', false, '0', { tax_lines: [{ rate: '20' }, { rate: '5' }] })]), '125.00'],
  ['credit-line', cart([item('100', '1', false)], [], { credit_lines: [{ amount: '25' }] }), '100.00'],
  ['zero-quantity', cart([item('100', '0', false)]), '0.00'],
  ['fractional-price-and-quantity', cart([item('0.33333333', '1.5', false, '20')]), '0.60'],
  ['inclusive-repeating-division', cart([item('99.99', '3', true, '21')]), '299.97'],
  ['mixed-inclusive-and-exclusive', cart([item('100', '1', false, '25', { id: 'fixture-exclusive' }), item('125', '1', true, '25', { id: 'fixture-inclusive' })]), '250.00'],
  ['all-returned', cart([item('100', '1', false, '25', { detail: { return_received_quantity: '1' } })]), '0.00'],
]
const fields = ['total', 'subtotal', 'tax_total', 'discount_total', 'discount_subtotal', 'discount_tax_total',
  'original_total', 'original_subtotal', 'original_tax_total', 'credit_line_total', 'credit_line_subtotal', 'credit_line_tax_total',
  'item_total', 'item_subtotal', 'item_tax_total', 'item_discount_total', 'shipping_total', 'shipping_subtotal', 'shipping_tax_total', 'shipping_discount_total',
  'refundable_total', 'refundable_total_per_unit', 'return_requested_total', 'return_received_total', 'return_dismissed_total']
const amounts = value => Object.fromEntries(fields.filter(key => value[key]?.bigNumber).map(key => [key, {
  decimal: value[key].bigNumber.toFixed(), raw: value[key].raw,
}]))
const output = {
  format_version: 1,
  scope: 'Artificial pure-arithmetic fixtures from installed legacy utility; not promotion selection, provider rounding or checkout integration acceptance',
  sources: {}, cases: [],
}
for (const file of ['cart/index.js', 'line-item/index.js', 'shipping-method/index.js', 'adjustment/index.js', 'tax/index.js', 'credit-lines/index.js', 'big-number.js', 'math.js', 'transform-properties-to-bignumber.js']) {
  const source = path.join(utilsRoot, 'totals', file)
  output.sources[path.relative(root, source).replaceAll(path.sep, '/')] = crypto.createHash('sha256').update(fs.readFileSync(source)).digest('hex')
}
for (const [name, input, expectedRoundedTotal] of cases) {
  const value = decorateCartTotals(structuredClone(input))
  if (value.total.bigNumber.toFixed(2) !== expectedRoundedTotal) throw new Error(`Independent total assertion failed: ${name}`)
  output.cases.push({ name, input, expected_display_total: expectedRoundedTotal, totals: amounts(value),
    items: value.items.map(row => ({ id: row.id, totals: amounts(row), adjustments: row.adjustments.map(adj => amounts(adj)) })),
    shipping_methods: value.shipping_methods.map(row => ({ id: row.id, totals: amounts(row) })),
  })
}
const target = path.join(root, 'docs/migration/fixtures/legacy-totals.json')
fs.mkdirSync(path.dirname(target), { recursive: true })
const content = JSON.stringify(output, null, 2) + '\n'
if (process.argv.includes('--check')) {
  if (!fs.existsSync(target) || fs.readFileSync(target, 'utf8').replaceAll('\r\n', '\n') !== content) throw new Error('Legacy totals evidence changed; review before refreshing fixtures')
} else fs.writeFileSync(target, content)
console.log(JSON.stringify({ cases: output.cases.length, independent_rounded_total_assertions: output.cases.length, source_hashes: Object.keys(output.sources).length, mode: process.argv.includes('--check') ? 'check' : 'capture' }))
