// Artificial, pure-function evidence only. No app/container, env, DB, network or writes to legacy data.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backend = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const resolveModule = createRequire(backend.resolve('@medusajs/medusa/core-flows'))
const framework = createRequire(backend.resolve('@medusajs/framework/utils'))
const roots = Object.fromEntries(['promotion', 'tax', 'fulfillment'].map(name => [name, path.dirname(resolveModule.resolve(`@medusajs/${name}`))]))
roots.utils = path.dirname(framework.resolve('@medusajs/utils'))
roots.coreflows = path.dirname(resolveModule.resolve('@medusajs/core-flows'))
const sourceFiles = []
function load(module, file) {
  const target = path.join(roots[module], file)
  sourceFiles.push(target)
  return require(target)
}
const { getComputedActionsForItems } = load('promotion', 'utils/compute-actions/line-items.js')
const { getComputedActionsForShippingMethods } = load('promotion', 'utils/compute-actions/shipping-methods.js')
const { areRulesValidForContext } = load('promotion', 'utils/validations/promotion-rule.js')
const TaxService = load('tax', 'services/tax-module-service.js').default
// Only non-decorated, synchronous selection methods; no constructor or injected services.
const taxService = Object.create(TaxService.prototype)
const { isContextValid, validateAndNormalizeRules } = load('fulfillment', 'utils/utils.js')
const FulfillmentService = load('fulfillment', 'services/fulfillment-module-service.js').default
for (const [module, file] of [
  ['promotion', 'utils/compute-actions/usage.js'], ['promotion', 'utils/compute-actions/sort-by-price.js'],
  ['utils', 'totals/promotion/index.js'], ['utils', 'totals/math.js'], ['utils', 'totals/big-number.js'],
]) sourceFiles.push(path.join(roots[module], file))
const decimal = value => value.bigNumber.toFixed()
const normalize = value => {
  if (value?.bigNumber?.toFixed) return decimal(value)
  if (value?.toFixed && value?.constructor?.name === 'BigNumber') return value.toFixed()
  if (Array.isArray(value)) return value.map(normalize)
  if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, row]) => [key, normalize(row)]))
  return value
}
const output = { format_version: 1,
  scope: 'Artificial selection/allocation fixtures only; not database qualification, budget registration/concurrency, provider execution or checkout acceptance',
  sources: {}, reviewed_sources_not_executed: {}, cases: [] }
function record(domain, name, input, actual, expected) {
  actual = normalize(actual)
  assert.deepEqual(actual, expected, `${domain}/${name}`)
  output.cases.push({ domain, name, input, expected: actual })
}
const line = (id, subtotal, quantity = '1', extra = {}) => ({ id, subtotal, original_total: subtotal, quantity, is_discountable: true, ...extra })
const promotion = (allocation, value, extra = {}) => ({ code: 'FIXTURE', type: 'standard', is_tax_inclusive: false,
  application_method: { target_type: 'items', type: 'fixed', allocation, value, ...extra } })
const itemAction = (id, amount, inclusive = false) => ({ action: 'addItemAdjustment', item_id: id, amount, code: 'FIXTURE', is_tax_inclusive: inclusive })
const promoCases = [
  ['fixed-each-default-quota-one', promotion('each', '10'), [line('a', '200', '2')], [], [itemAction('a', '10')]],
  ['fixed-each-explicit-quota', promotion('each', '10', { max_quantity: 2 }), [line('a', '200', '2')], [], [itemAction('a', '20')]],
  ['fixed-across-proportional', promotion('across', '40'), [line('a', '100'), line('b', '300')], [], [itemAction('a', '10'), itemAction('b', '30')]],
  ['percentage-each-default-quota-one', promotion('each', '10', { type: 'percentage' }), [line('a', '200', '2')], [], [itemAction('a', '10')]],
  ['once-cheapest-first', promotion('once', '10', { max_quantity: 1 }), [line('b', '100'), line('a', '40')], [], [itemAction('a', '10')]],
  ['once-without-quota', promotion('once', '10'), [line('a', '40')], [], []],
  ['discount-capped-at-remaining', promotion('each', '20'), [line('a', '10')], [['a', '7']], [itemAction('a', '3')]],
  ['not-discountable', promotion('each', '10'), [line('a', '100', '1', { is_discountable: false })], [], []],
  ['zero-subtotal', promotion('each', '10'), [line('a', '0')], [], []],
  ['inclusive-with-existing-exclusive-discount', { ...promotion('each', '25'), is_tax_inclusive: true }, [line('a', '100', '1', { original_total: '125' })], [['a', '90']], [itemAction('a', '12.5', true)]],
  ['spend-budget-exceeded', { ...promotion('each', '10'), campaign: { budget: { type: 'spend', used: '95', limit: '100' } } }, [line('a', '100')], [], [{ action: 'campaignBudgetExceeded', code: 'FIXTURE' }]],
  ['spend-budget-exact-boundary', { ...promotion('each', '10'), campaign: { budget: { type: 'spend', used: '90', limit: '100' } } }, [line('a', '100')], [], [itemAction('a', '10')]],
]
for (const [name, promo, items, previous, expected] of promoCases) record('promotion-items', name,
  { promotion: promo, items, previous_discounts_exclusive: previous }, getComputedActionsForItems(structuredClone(promo), structuredClone(items), new Map(previous)), expected)
const shipAction = (id, amount) => ({ action: 'addShippingMethodAdjustment', shipping_method_id: id, amount, code: 'FIXTURE' })
for (const [name, allocation, value, extra, methods, expected] of [
  ['fixed-across', 'across', '8', {}, [{ id: 'a', subtotal: '10' }, { id: 'b', subtotal: '30' }], [shipAction('a', '2'), shipAction('b', '6')]],
  ['once-cheapest', 'once', '5', { max_quantity: 1 }, [{ id: 'b', subtotal: '20' }, { id: 'a', subtotal: '10' }], [shipAction('a', '5')]],
  ['percentage', 'each', '50', { type: 'percentage' }, [{ id: 'a', subtotal: '10' }], [shipAction('a', '5')]],
]) {
  const promo = promotion(allocation, value, { target_type: 'shipping_methods', ...extra })
  record('promotion-shipping', name, { promotion: promo, shipping_methods: methods }, getComputedActionsForShippingMethods(promo, methods, new Map()), expected)
}
for (const [name, operator, values, contextValue, expected] of [
  ['eq-requires-every-value', 'eq', ['a'], ['a', 'b'], false], ['in-any-value', 'in', ['a'], ['a', 'b'], true],
  ['ne-all-excluded', 'ne', ['a'], ['b', 'c'], true], ['decimal-threshold', 'gte', ['99.95'], '99.95', true],
  ['missing-positive-attribute', 'eq', ['a'], undefined, false],
]) {
  const rules = [{ attribute: 'items.category', operator, values: values.map(value => ({ value })) }]
  const context = contextValue === undefined ? {} : { category: contextValue }
  record('promotion-rules', name, { rules, context, scope: 'items' }, areRulesValidForContext(rules, context, 'items'), expected)
}
const rate = (id, region, rule = null, extra = {}) => ({ id, rate: '25', tax_region_id: region,
  tax_region: { id: region, province_code: region === 'country' ? null : 'fixture', parent_id: region === 'country' ? null : 'country' },
  is_default: rule === null, is_combinable: false, rules: rule ? [rule] : [], ...extra })
const productRule = { reference: 'product', reference_id: 'fixture-product' }
const typeRule = { reference: 'product_type', reference_id: 'fixture-type' }
const product = { id: 'fixture-item', product_id: 'fixture-product', product_type_id: 'fixture-type' }
const rates = [rate('country-default', 'country'), rate('country-type', 'country', typeRule), rate('country-product', 'country', productRule),
  rate('province-default', 'province'), rate('province-type', 'province', typeRule), rate('province-product', 'province', productRule)]
for (const [name, item, candidates, expectedIds] of [
  ['granularity-priority', product, rates, ['province-product']],
  ['province-default-before-country-product', product, [rates[2], rates[3]], ['province-default']],
  ['combinable-adds-most-specific-parent', product, [...rates.slice(0, 3), { ...rates[5], is_combinable: true }], ['province-product', 'country-product']],
  ['missing-parent-does-not-invent-rate', product, [{ ...rates[5], is_combinable: true }], ['province-product']],
  ['no-candidates', product, [], []],
  ['shipping-specific-before-country-default', { id: 'ship', shipping_option_id: 'fixture-option' },
    [rates[0], rate('ship-rate', 'country', { reference: 'shipping_option', reference_id: 'fixture-option' })], ['ship-rate']],
  ['equal-priority-keeps-input-order', product, [rate('first', 'country'), rate('second', 'country')], ['first']],
]) record('tax-selection', name, { item, rates: candidates }, taxService.getTaxRatesForItem(item, candidates).map(row => row.id), expectedIds)
const query = taxService.getTaxRateQueryForItem(product, ['country', 'province'])
for (const [name, candidate, expected] of [
  ['wrong-region', rate('foreign', 'foreign'), false], ['matching-product', rates[2], true],
  ['different-product', rate('other-product', 'country', { reference: 'product', reference_id: 'other' }), false],
]) record('tax-query', name, { rate: candidate, query }, taxService.rateMatchesQuery(candidate, query), expected)
for (const [name, context, rules, expected] of [
  ['empty-rules', {}, [], true], ['boolean-normalized', { enabled: true }, [{ attribute: 'enabled', operator: 'eq', value: true }], true],
  ['nested-country-in', { address: { country_code: 'dk' } }, [{ attribute: 'address.country_code', operator: 'in', value: ['dk', 'de'] }], true],
  ['country-denied', { address: { country_code: 'fr' } }, [{ attribute: 'address.country_code', operator: 'in', value: ['dk', 'de'] }], false],
  ['all-rules-required', { enabled: true, total: '99' }, [{ attribute: 'enabled', operator: 'eq', value: true }, { attribute: 'total', operator: 'gte', value: '100' }], false],
  ['decimal-threshold', { total: '100.25' }, [{ attribute: 'total', operator: 'gte', value: '100.25' }], true],
  ['date-threshold', { date: '2026-10-27T00:00:00Z' }, [{ attribute: 'date', operator: 'gt', value: '2026-10-26T00:00:00Z' }], true],
  ['missing-positive-attribute', {}, [{ attribute: 'enabled', operator: 'eq', value: true }], false],
]) {
  const normalized = structuredClone(rules)
  validateAndNormalizeRules(normalized)
  record('shipping-rules', name, { context, rules, normalized_rules: normalized }, isContextValid(context, normalized), expected)
}
for (const [name, address, expected] of [
  ['empty-address', {}, []],
  ['country-only', { country_code: 'dk' }, [{ type: 'country', country_code: 'dk' }]],
  ['country-and-province', { country_code: 'dk', province_code: 'fixture' }, [{ type: 'province', country_code: 'dk', province_code: 'fixture' }, { type: 'country', country_code: 'dk' }]],
  ['postal-without-province', { country_code: 'dk', postal_expression: '1234' }, [{ type: 'zip', country_code: 'dk', postal_expression: '1234' }, { type: 'country', country_code: 'dk' }]],
  ['city-falls-back-to-country', { country_code: 'dk', city: 'fixture-city' }, [{ type: 'city', country_code: 'dk', city: 'fixture-city' }, { type: 'country', country_code: 'dk' }]],
]) record('shipping-geo-query', name, { address }, FulfillmentService.buildGeoZoneConstraintsFromAddress(address), expected)
for (const [name, rule, expectedMessage] of [
  ['boolean-false-rejected-before-normalization', { attribute: 'enabled', operator: 'eq', value: false }, 'Rule must have an attribute, an operator and a value'],
  ['numeric-zero-rejected-before-normalization', { attribute: 'total', operator: 'gte', value: 0 }, 'Rule must have an attribute, an operator and a value'],
  ['in-requires-array', { attribute: 'country', operator: 'in', value: 'dk' }, 'Rule value must be an array for in/nin operators'],
]) {
  let actual = null
  try { validateAndNormalizeRules([structuredClone(rule)]) } catch (error) { actual = error.message }
  record('shipping-rule-validation', name, { rule }, actual, expectedMessage)
}
for (const target of [...new Set(sourceFiles)].sort()) output.sources[path.relative(root, target).replaceAll(path.sep, '/')] = crypto.createHash('sha256').update(fs.readFileSync(target)).digest('hex')
for (const [module, file] of [
  ['promotion', 'services/promotion-module.js'], ['tax', 'providers/system.js'],
  ['coreflows', 'cart/workflows/list-shipping-options-for-cart.js'],
  ['coreflows', 'cart/workflows/list-shipping-options-for-cart-with-pricing.js'],
  ['coreflows', 'cart/workflows/add-shipping-method-to-cart.js'],
  ['coreflows', 'tax/steps/get-item-tax-lines.js'],
]) {
  const source = path.join(roots[module], file)
  output.reviewed_sources_not_executed[path.relative(root, source).replaceAll(path.sep, '/')] = crypto.createHash('sha256').update(fs.readFileSync(source)).digest('hex')
}
const target = path.join(root, 'docs/migration/fixtures/legacy-commerce-rules.json')
const content = JSON.stringify(output, null, 2) + '\n'
if (process.argv.includes('--check')) assert.equal(fs.readFileSync(target, 'utf8').replaceAll('\r\n', '\n'), content, 'Legacy rule evidence changed; review before refresh')
else fs.writeFileSync(target, content)
console.log(JSON.stringify({ cases: output.cases.length, independent_assertions: output.cases.length, source_hashes: Object.keys(output.sources).length, mode: process.argv.includes('--check') ? 'check' : 'capture' }))
