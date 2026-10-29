// Actual pure shipping allocator, artificial data; no application or database.
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto'), assert = require('node:assert/strict')
const root = path.resolve(__dirname, '../..')
const base = fs.readdirSync(path.join(root, 'medusa-store/node_modules/.pnpm')).find(p => p.startsWith('@medusajs+promotion@'))
assert.ok(base)
const dir = path.join(root, 'medusa-store/node_modules/.pnpm', base, 'node_modules/@medusajs/promotion/dist/utils/compute-actions')
const {getComputedActionsForShippingMethods} = require(path.join(dir, 'shipping-methods.js'))
const scalar = v => v?.bigNumber?.toFixed ? v.bigNumber.toFixed() : v?.toFixed ? v.toFixed() : String(v)
const cases = []
function add(name, allocation, type, value, max_quantity, methods, previous, expected) {
  const input = {allocation, type, value, max_quantity, methods: methods.map(([id, subtotal]) => ({id, subtotal})), previous: Object.fromEntries(previous)}
  const promotion = {code: 'FIXTURE', type: 'standard', application_method: {target_type: 'shipping_methods', allocation, type, value, ...(max_quantity === null ? {} : {max_quantity})}}
  const map = new Map(previous), before = JSON.stringify(input)
  const actions = getComputedActionsForShippingMethods(promotion, structuredClone(input.methods), map)
  const amounts = actions.map(a => {
    assert.equal(a.action, 'addShippingMethodAdjustment'); assert.equal(a.code, 'FIXTURE')
    return {id: a.shipping_method_id, amount: scalar(a.amount)}
  })
  assert.deepEqual(amounts, expected.map(([id, amount]) => ({id, amount})), name)
  const discounts = Object.fromEntries([...map].map(([id, amount]) => [id, scalar(amount)]))
  const expectedMap = {...input.previous}
  // Independent exact decimal fixtures below avoid JS float accumulation.
  for (const [id, amount] of expected) {
    const before = input.previous[id] || '0'
    if (before === '0') expectedMap[id] = amount
    else if (before === '7' && amount === '3') expectedMap[id] = '10'
    else if (before === '5' && amount === '5') expectedMap[id] = '10'
    else if (before === '2' && amount === '4') expectedMap[id] = '6'
    else throw new Error('explicit expected shared balance required')
  }
  assert.deepEqual(discounts, expectedMap, name+' shared balances')
  assert.equal(JSON.stringify(input), before)
  cases.push({name, input, expected: {amounts, discounts}})
}
add('each-fixed','each','fixed','5',null,[['b','20'],['a','10']],[],[['b','5'],['a','5']])
add('each-ignores-quota','each','fixed','5',0,[['a','10']],[],[['a','5']])
add('once-cheapest','once','fixed','5',1,[['b','20'],['a','10']],[],[['a','5']])
add('once-stable-tie','once','fixed','5',1,[['b','10'],['a','10']],[],[['b','5']])
add('once-default-zero','once','fixed','5',null,[['a','10']],[],[])
add('once-zero-explicit','once','fixed','5',0,[['a','10']],[],[])
add('once-skips-exhausted','once','fixed','5',1,[['a','5'],['b','10']],[['a','5']],[['b','5']])
add('once-skips-zero','once','fixed','5',1,[['a','0'],['b','10']],[],[['b','5']])
add('each-cap','each','fixed','20',null,[['a','10']],[['a','7']],[['a','3']])
add('across-fixed','across','fixed','8',null,[['a','10'],['b','30']],[],[['a','2'],['b','6']])
add('across-prior','across','fixed','10',null,[['a','10'],['b','5']],[['a','5']],[['a','5'],['b','5']])
add('across-percentage','across','percentage','50',null,[['a','10'],['b','30']],[],[['a','5'],['b','15']])
add('each-percentage-prior','each','percentage','50',null,[['a','10']],[['a','2']],[['a','4']])
add('percent-cap','each','percentage','150',null,[['a','10']],[],[['a','10']])
add('zero-value','across','fixed','0',null,[['a','10']],[],[])
add('empty','each','fixed','1',null,[],[['other','7']],[])
add('negative-remainder-denominator','across','fixed','4',null,[['a','1'],['b','5']],[['a','2']],[['b','5']])
add('across-total-nonpositive','across','fixed','4',null,[['a','1'],['b','1']],[['a','2']],[])
add('fractional-no-cent-rounding','across','fixed','1',null,[['a','1'],['b','2']],[],[['a','0.33333333333333333333'],['b','0.66666666666666666667']])
add('exact-large','each','percentage','50',null,[['a','9007199254740993']],[],[['a','4503599627370496.5']])
const sources = Object.fromEntries(['shipping-methods.js','sort-by-price.js','usage.js'].map(name => [path.relative(root,path.join(dir,name)).replaceAll('\\','/'),crypto.createHash('sha256').update(fs.readFileSync(path.join(dir,name),'utf8').replaceAll('\r\n','\n')).digest('hex')]))
const content = JSON.stringify({format_version:1, scope:'Shipping arithmetic with no campaign budgets; not full qualification or checkout', sources, cases},null,2)+'\n'
const target = path.join(root,'docs/migration/fixtures/shipping-allocation.json')
assert.ok(process.argv.length<=3 && (!process.argv[2] || process.argv[2]==='--check'))
if (process.argv[2]) assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({original_shipping_allocation_cases:cases.length, application_started:false}))
