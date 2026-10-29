// Original installed model + arithmetic only; no app, database, env or HTTP.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),store=path.join(root,'medusa-store/node_modules/.pnpm')
const entry=(prefix,file)=>path.join(store,fs.readdirSync(store).find(p=>p.startsWith(prefix)),'node_modules',file)
const modelFile=entry('@medusajs+cart@','@medusajs/cart/dist/models/cart.js')
const fieldsFile=entry('@medusajs+core-flows@','@medusajs/core-flows/dist/cart/utils/fields.js')
const totalsFile=entry('@medusajs+utils@','@medusajs/utils/dist/totals/cart/index.js')
const numberFile=entry('@medusajs+utils@','@medusajs/utils/dist/totals/big-number.js')
const schema=require(modelFile).default.parse().schema
const fields=require(fieldsFile).cartFieldsForRefreshSteps
const {decorateCartTotals}=require(totalsFile),{getEpsilonFromDecimalPrecision}=require(numberFile)
const input={currency_code:'eur',region:{automatic_taxes:true},items:[{id:'amount-cart-line',quantity:2,unit_price:'100',is_tax_inclusive:false,tax_lines:[{rate:25}],adjustments:[{amount:'20',is_tax_inclusive:false}]}],shipping_methods:[{id:'amount-cart-shipping',amount:'12.5',is_tax_inclusive:true,tax_lines:[{rate:25}],adjustments:[{amount:'2'}]}]}
const cart=decorateCartTotals(input)
assert.ok(fields.includes('quantity'));assert.equal(Object.hasOwn(schema,'quantity'),false);assert.equal(Object.hasOwn(cart,'quantity'),false)
const keys=['subtotal','original_item_total','item_total','total','original_item_subtotal','item_subtotal','shipping_subtotal']
const baseline_amounts=Object.fromEntries(keys.map(k=>[k,cart[k].bigNumber.toFixed()]))
assert.equal(baseline_amounts.total,'235');assert.equal(baseline_amounts.subtotal,'210')
const epsilon=Object.fromEntries([0,2,18].map(d=>[d,getEpsilonFromDecimalPrecision(d).bigNumber.toFixed()]))
const sources=Object.fromEntries([modelFile,fieldsFile,totalsFile,numberFile].map(f=>[path.relative(root,f).replaceAll('\\','/'),crypto.createHash('sha256').update(fs.readFileSync(f,'utf8').replaceAll('\r\n','\n')).digest('hex')]))
const output={format_version:1,scope:'Original model and pure arithmetic over artificial cart only; not QueryGraph or checkout acceptance',sources,quantity_requested:true,model_has_quantity:false,decorated_cart_has_quantity:false,baseline_amounts,epsilon}
const target=path.join(root,'docs/migration/fixtures/cart-promotion-context.json'),content=JSON.stringify(output,null,2)+'\n'
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({original_order_amounts:keys.length,currency_epsilon_cases:3,top_quantity_absent:true,application_started:false}))
