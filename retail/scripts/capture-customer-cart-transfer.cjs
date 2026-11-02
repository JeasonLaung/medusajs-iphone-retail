// Execute original pure callbacks only. No application, SQL or provider starts.
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const assert = require('node:assert/strict')
const {createRequire} = require('node:module')
const root = path.resolve(__dirname, '../..')
const modules = path.join(root, 'medusa-store/node_modules/.pnpm')
const packages = fs.readdirSync(modules).filter(n => n.startsWith('@medusajs+core-flows@')).sort()
assert.ok(packages.length > 0, 'Installed original workflow required')
const digest = s => crypto.createHash('sha256').update(s).digest('hex')
const sources = {}
for (const name of ['transfer-cart-customer', 'refresh-cart-items', 'refresh-payment-collection']) {
  const copies = packages.map(p => path.join(modules, p, 'node_modules/@medusajs/core-flows/dist/cart/workflows', name + '.js'))
  const bytes = copies.map(p => fs.readFileSync(p, 'utf8'))
  assert.equal(new Set(bytes.map(digest)).size, 1, 'Installed source copies differ: ' + name)
  sources[name] = {file: copies[0], text: bytes[0], sha256: digest(bytes[0])}
}
function callback(source, marker, signature, endMarker) {
  const anchor = source.indexOf(marker)
  const start = source.indexOf(signature, anchor)
  const end = source.indexOf(endMarker, start)
  assert.ok(anchor >= 0 && start > anchor && end > start, 'Original callback shape changed')
  return new Function('return (' + source.slice(start, end) + ')')()
}
const transfer = sources['transfer-cart-customer'].text
const should = callback(transfer, 'const shouldTransfer =', '({ cart, customer }) =>', ');')
const update = callback(transfer, 'const cartInput =', '({ cart, customer }) =>', ');')
const account = {id:'new-account', email:'new-account@example.invalid'}
const cases = []
for (const [name, cart, changed] of [
  ['anonymous', {id:'cart-a', email:'guest@example.invalid'}, true],
  ['same-account-edited-checkout-email', {id:'cart-a', customer:{id:account.id}, email:'edited@example.invalid'}, false],
  ['different-account', {id:'cart-a', customer:{id:'other-account'}, email:'other@example.invalid'}, true],
  ['anonymous-with-payment-collection', {id:'cart-a', payment_collection:{id:'pc-a', payment_sessions:[{id:'ps-a'}]}}, true],
  ['same-account-with-payment-collection', {id:'cart-a', customer:{id:account.id}, payment_collection:{id:'pc-a'}}, false],
]) {
  assert.equal(should({cart, customer:account}), changed)
  const patch = changed ? update({cart, customer:account}) : []
  if (changed) assert.deepEqual(patch, [{id:cart.id, customer_id:account.id, email:account.email}])
  cases.push({name, transfer:changed, update:patch})
}
assert.match(transfer, /refreshCartItemsWorkflow\.runAsStep\([\s\S]*?cart_id: input\.id,[\s\S]*?force_refresh: true/)
assert.match(sources['refresh-cart-items'].text, /refreshPaymentCollectionForCartWorkflow\.runAsStep\(\{\s*input: \{ cart: refetchedCart \}/)
const payment = sources['refresh-payment-collection']
const marker = '"should-update-payment-collection"'
const anchor = payment.text.indexOf(marker)
const start = payment.text.indexOf('({ cart, shouldExecute }) => {', anchor)
const end = payment.text.indexOf('\n    }).then', start)
assert.ok(anchor >= 0 && start > anchor && end > start)
const utils = createRequire(payment.file)('@medusajs/framework/utils')
const changed = new Function('utils_1', 'return ' + payment.text.slice(start,end) + '\n}')(utils)
const paymentCases = []
for (const [name, amount, old, currency, expected] of [
  ['same-amount-and-currency-keeps-session','158.75','158.750','eur',false],
  ['new-account-discount-invalidates-session','148.75','158.75','eur',true],
  ['currency-change-invalidates-session','158.75','158.75','usd',true],
  ['sub-cent-change-invalidates-session','158.750000000000000000001','158.75','eur',true],
]) {
  const cart = {raw_total:{value:amount,precision:100},currency_code:currency,
    payment_collection:{raw_amount:{value:old,precision:100},currency_code:'eur'}}
  const actual = changed({cart, shouldExecute:true})
  assert.equal(actual, expected)
  paymentCases.push({name, amount, collection_amount:old, currency, invalidate_sessions:actual})
}
const report = {format_version:1, scope:'Original pure callbacks and checked workflow call sites only; no SQL/provider/application execution.',
  sources:Object.fromEntries(Object.entries(sources).map(([name,s]) => [name,{sha256:s.sha256,installed_copies:packages.length}])),
  transfer_cases:cases, payment_cases:paymentCases,
  native_gap:'Current native transfer rejects any attached collection and all frozen checkout attempts. Original refresh only invalidates sessions when exact amount or currency changes; integrating native durable provider coordination remains required.',
  workflow_runtime_verified:false, native_payment_transfer_complete:false}
const target = path.join(root,'docs/migration/fixtures/customer-cart-transfer-source.json')
const content = JSON.stringify(report,null,2)+'\n'
assert.ok(process.argv.length <= 3 && (!process.argv[2] || process.argv[2] === '--check'))
if (process.argv[2]) assert.equal(fs.readFileSync(target,'utf8'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({source_callback_cases:cases.length+paymentCases.length, provider_calls:0, sql_connections:0, native_gap_open:true}))
