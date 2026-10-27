// Pure installed legacy oracle; artificial values only. No application startup,
// environment files, database connections, network, or account data.
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const backendRequire = createRequire(path.join(root, 'medusa-store/apps/backend/package.json'))
const frameworkRequire = createRequire(backendRequire.resolve('@medusajs/framework/utils'))
const utilsRoot = path.dirname(frameworkRequire.resolve('@medusajs/utils'))
const { decorateCartTotals } = require(path.join(utilsRoot, 'totals/cart/index.js'))
const mathRequire = createRequire(path.join(utilsRoot, 'totals/math.js'))
const B = mathRequire('bignumber.js').BigNumber
if (B.config().DECIMAL_PLACES !== 20 || B.config().ROUNDING_MODE !== B.ROUND_HALF_UP) throw new Error('Legacy arithmetic configuration changed')
const fields = ['total','subtotal','tax_total','discount_total','discount_subtotal','discount_tax_total','original_total','original_subtotal','original_tax_total','credit_line_total','credit_line_subtotal','credit_line_tax_total','item_total','item_subtotal','item_tax_total','item_discount_total','original_item_total','original_item_subtotal','original_item_tax_total','shipping_total','shipping_subtotal','shipping_tax_total','shipping_discount_total','original_shipping_total','original_shipping_subtotal','original_shipping_tax_total','refundable_total','refundable_total_per_unit','fulfilled_total','shipped_total','return_requested_total','return_received_total','return_dismissed_total','write_off_total']
const item = extra => ({id:'boundary-item',unit_price:'100',quantity:'3',is_tax_inclusive:false,tax_lines:[{rate:'25'}],adjustments:[],...extra})
const cart = (items=[],shipping_methods=[],credit_lines=[],currency_code='eur') => ({currency_code,region:{automatic_taxes:true},items,shipping_methods,credit_lines})
const cases = [
  ['mixed-adjustment-tax-flags',cart([item({adjustments:[{amount:'12.5',is_tax_inclusive:true},{amount:'7',is_tax_inclusive:false},{}]})]),2],
  ['all-lifecycle-fields',cart([item({adjustments:[{amount:'30',is_tax_inclusive:false}],detail:{fulfilled_quantity:'2',shipped_quantity:'1',return_requested_quantity:'0.5',return_received_quantity:'0.5',return_dismissed_quantity:'0.25',written_off_quantity:'0.125'}})]),2],
  ['inclusive-refund-discount',cart([item({unit_price:'125',is_tax_inclusive:true,adjustments:[{amount:'37.5',is_tax_inclusive:true}],detail:{return_requested_quantity:'1',return_received_quantity:'1'}})]),2],
  ['negative-current-quantity-preserved',cart([item({quantity:'1',detail:{return_received_quantity:'2'}})]),2],
  ['over-discount-negative-total',cart([item({quantity:'1',adjustments:[{amount:'120',is_tax_inclusive:false}]})]),2],
  ['negative-credit',cart([item({quantity:'1'})],[],[{amount:'-2.5'}]),2],
  ['credit-at-eur-epsilon',cart([item({quantity:'1'})],[],[{amount:'0.01'}]),2],
  ['credit-above-eur-epsilon',cart([item({quantity:'1'})],[],[{amount:'0.010000000000000001'}]),2],
  ['credit-sum-epsilon',cart([],[],[{amount:'0.006'},{amount:'0.004'},{}]),2],
  ['jpy-one-unit-credit',cart([item({quantity:'1'})],[],[{amount:'1'}],'jpy'),0],
  ['kwd-one-unit-credit',cart([item({quantity:'1'})],[],[{amount:'0.001'}],'kwd'),3],
  ['raw-unit-quantity-precedence',cart([item({unit_price:'1',raw_unit_price:{value:'9007199254740993.000000000000000001',precision:38},quantity:'1',raw_quantity:{value:'1.000000000000000001',precision:38},tax_lines:[]})]),2],
  ['raw-adjustment-tax-credit',cart([item({unit_price:'1',quantity:'1',tax_lines:[{rate:'1',raw_rate:{value:'25',precision:20}}],adjustments:[{amount:'1',raw_amount:{value:'0.125',precision:20},is_tax_inclusive:true}]})],[],[{amount:'2',raw_amount:{value:'0.02',precision:20}}]),2],
  ['raw-lifecycle-quantity',cart([item({detail:{return_received_quantity:'0',raw_return_received_quantity:{value:'0.5',precision:20},return_requested_quantity:'0',raw_return_requested_quantity:{value:'0.25',precision:20}}})]),2],
  ['raw-scientific-money',cart([item({raw_unit_price:{value:'1.234567890123456789e+5',precision:20},quantity:'1',tax_lines:[]})]),2],
  ['inherited-tax-flag',cart([item({unit_price:'125',quantity:'1',is_tax_inclusive:undefined,detail:{return_requested_quantity:'0.5'}})]),2],
  ['explicit-false-overrides-region',cart([item({unit_price:'125',quantity:'1',is_tax_inclusive:false})]),2],
  ['shipping-inherited-and-raw',cart([], [{id:'boundary-shipping',amount:'1',raw_amount:{value:'12.5',precision:20},tax_lines:[{rate:'25'}],adjustments:[{amount:'1.25',is_tax_inclusive:true}]}]),2],
  ['fractional-adjustment-proration',cart([item({unit_price:'0.123456789012345678',quantity:'7',is_tax_inclusive:true,tax_lines:[{rate:'21'}],adjustments:[{amount:'0.1',is_tax_inclusive:true}],detail:{return_received_quantity:'0.3',return_requested_quantity:'0.2'}})]),2],
  ['per-tax-line-division-rounding',cart([item({unit_price:'10000000000000000000',quantity:'1',tax_lines:[{rate:'0.0000000000000000005'},{rate:'0.0000000000000000005'}]})]),2],
  ['negative-tax-and-adjustment',cart([item({quantity:'1',tax_lines:[{rate:'-5'}],adjustments:[{amount:'-1',is_tax_inclusive:false}]})]),2],
  ['zero-quantity-with-discount',cart([item({quantity:'0',adjustments:[{amount:'5',is_tax_inclusive:false}]})]),2],
  ['pending-difference-with-requested-return',{...cart([item({quantity:'3',detail:{return_requested_quantity:'1'}})]),summary:{transaction_total:'250'}},2],
  ['pending-difference-at-positive-epsilon',{...cart([item({quantity:'1'})]),summary:{transaction_total:'124.99'}},2],
  ['pending-difference-at-negative-epsilon',{...cart([item({quantity:'1'})]),summary:{transaction_total:'125.01'}},2],
  ['pending-difference-beyond-epsilon',{...cart([item({quantity:'1'})]),summary:{transaction_total:'125.010000000000000001'}},2],
  ['pending-difference-default-transaction',{...cart([item({quantity:'1'})]),summary:{}},2],
  ['pending-difference-raw-transaction',{...cart([item({quantity:'1'})]),summary:{transaction_total:'0',raw_transaction_total:{value:'100',precision:20}}},2],
  ['negative-credit-at-epsilon',cart([],[],[{amount:'-0.01'}]),2],
]
const amounts = value => Object.fromEntries(fields.filter(key=>value[key]?.bigNumber).map(key=>[key,{decimal:value[key].bigNumber.toFixed(),raw:value[key].raw}]))
const output = {format_version:1,scope:'Artificial arithmetic boundaries from installed legacy code; not provider settlement, pricing selection or checkout acceptance',division_decimal_places:20,division_rounding:'half_up_away_from_zero',sources:{},cases:[]}
for (const file of ['cart/index.js','line-item/index.js','shipping-method/index.js','adjustment/index.js','tax/index.js','credit-lines/index.js','big-number.js','math.js','transform-properties-to-bignumber.js']) {
 const source=path.join(utilsRoot,'totals',file)
 output.sources[path.relative(root,source).replaceAll(path.sep,'/')]=crypto.createHash('sha256').update(fs.readFileSync(source)).digest('hex')
}
const arithmeticDependency = mathRequire.resolve('bignumber.js')
output.sources[path.relative(root,arithmeticDependency).replaceAll(path.sep,'/')]=crypto.createHash('sha256').update(fs.readFileSync(arithmeticDependency)).digest('hex')
for (const [name,input,currencyDecimals] of cases) {
 const value=decorateCartTotals(structuredClone(input))
 const totals=amounts(value)
 if(Object.values(totals).some(x=>!/^-?\d+(\.\d+)?$/.test(x.decimal)))throw new Error('Nonfinite boundary fixture')
 const pending=value.summary?.pending_difference
 output.cases.push({name,input,currency_decimals:currencyDecimals,totals,items:value.items.map(row=>({id:row.id,totals:amounts(row),adjustments:row.adjustments.map(amounts)})),shipping_methods:value.shipping_methods.map(row=>({id:row.id,totals:amounts(row)})),...(value.summary?{summary:{pending_difference:{decimal:(pending.bigNumber??pending).toFixed(),raw:null}}}:{})})
}
const target=path.join(root,'docs/migration/fixtures/legacy-totals-boundaries.json')
const content=JSON.stringify(output,null,2)+'\n'
if(process.argv.includes('--check')) {
 if(!fs.existsSync(target)||fs.readFileSync(target,'utf8').replaceAll('\r\n','\n')!==content)throw new Error('Boundary evidence drift; review before refreshing')
} else fs.writeFileSync(target,content)
console.log(JSON.stringify({cases:output.cases.length,source_hashes:Object.keys(output.sources).length,mode:process.argv.includes('--check')?'check':'capture'}))
