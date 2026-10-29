// Execute the installed original computeActions method body with fixture-only
// list services. No decorators, application startup, DB, Redis or network calls.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),store=path.join(root,'medusa-store/node_modules/.pnpm')
const pkg=p=>path.join(store,fs.readdirSync(store).find(n=>n.startsWith(p)))
const dir=path.join(pkg('@medusajs+promotion@'),'node_modules/@medusajs/promotion/dist')
process.env.MEDUSA_EPSILON='0.0001'
const framework=require('node:module').createRequire(path.join(dir,'services/promotion-module.js'))('@medusajs/framework/utils')
const utils=require(path.join(dir,'utils'))
const file=path.join(dir,'services/promotion-module.js'),source=fs.readFileSync(file,'utf8')
const start=source.indexOf('    async computeActions('),end=source.indexOf('\n    // @ts-expect-error',start)
assert.ok(start>0&&end>start)
const body=source.slice(start,end).replace('async computeActions(','async function(')
assert.equal((body.match(/return computedActions;/g)||[]).length,1)
const compute=new Function('utils_1','_utils_1','build_promotion_rule_query_filter_from_context_1','return '+body.replace('return computedActions;','return {actions:computedActions,discounts:methodIdPromoValueMap};'))(framework,utils,{})
const rule=(attribute,value)=>({attribute,operator:'eq',values:[{value}]})
const item=(id,subtotal='10',extra={})=>({id,subtotal,quantity:1,original_total:subtotal,product_id:'phone',...extra})
const method=(extra={})=>({type:'fixed',target_type:'items',value:'1',allocation:'each',...extra})
const promo=(id,extra={})=>({id,code:id,type:'standard',used:0,is_tax_inclusive:false,application_method:method(),...extra})
const budget=(type,limit,used='0')=>({id:'budget',type,limit,used})
const cases=[]
async function add(name,promotions,context,attributeUsages={}){
 const before=JSON.stringify({promotions,context})
 // Model hydration supplies BigNumber values, not fixture decimal strings.
 const hydrated=promotions.map(p=>({...p,application_method:p.application_method?{...p.application_method,value:new framework.BigNumber(p.application_method.value)}:null}))
 const fake={listActivePromotions_:async()=>hydrated,campaignBudgetUsageService_:{list:async filter=>{
  const found=attributeUsages[filter.budget_id]
  assert.ok(found,'Attribute lookup must be explicitly supplied')
  assert.equal(found.value,filter.attribute_value)
  return found.found?[{used:found.used}]:[]
 }}}
 const result=await compute.call(fake,promotions.map(p=>p.code),context,{prevent_auto_promotions:true})
 const scalar=v=>v?.bigNumber?.toFixed?v.bigNumber.toFixed():v?.toFixed?v.toFixed():String(v)
 const expected={actions:result.actions.map(a=>({Action:a.action,Code:a.code,PromotionID:promotions.find(p=>p.code===a.code).id,ItemID:a.item_id||'',ShippingMethodID:a.shipping_method_id||'',Amount:a.amount===undefined?'':scalar(a.amount),TaxInclusive:a.is_tax_inclusive||false})),discounts:Object.fromEntries([...result.discounts].map(([id,v])=>[id,scalar(v)]))}
 assert.equal(JSON.stringify({promotions,context}),before,'Original mutated fixtures')
 cases.push({name,promotions,context,attributeUsages,expected})
}
async function main(){
 const ctx={currency_code:'eur',items:[item('a','10'),item('b','20')]}
 await add('shared-stacking',[promo('half',{application_method:method({type:'percentage',value:'50'})}),promo('fixed')],ctx)
 await add('spend-reject-then-smaller-once',[promo('p',{application_method:method({value:'5',allocation:'once',max_quantity:1}),campaign:{budget:budget('spend','4')}})],{items:[item('big','10'),item('small','3')]})
 await add('spend-each-fails',[promo('p',{campaign:{budget:budget('spend','0')}})],ctx)
 await add('spend-reject-before-accept',[promo('p',{application_method:method({value:'5'}),campaign:{budget:budget('spend','4')}})],{items:[item('big','10'),item('small','3')]})
 await add('preview-not-reservation',[promo('p',{application_method:method({value:'6'}),campaign:{budget:budget('spend','10')}})],ctx)
 await add('usage-exceeded',[promo('p',{campaign:{budget:budget('usage','1','1')}})],ctx)
 await add('usage-equality',[promo('p',{campaign:{budget:budget('usage','1')}})],ctx)
 await add('limit-before-currency',[promo('p',{limit:0,application_method:method({currency_code:'usd'})})],ctx)
 await add('order-null-allocation',[promo('p',{application_method:method({target_type:'order',allocation:null,value:'3'})})],ctx)
 await add('target-and-order-rules',[promo('p',{rules:[rule('currency_code','eur')],application_method:method({target_rules:[rule('items.product_id','phone')]})})],{currency_code:'eur',items:[item('a'),item('b','10',{product_id:'tablet'})]})
 await add('inclusive-then-exclusive',[promo('inclusive',{is_tax_inclusive:true,application_method:method({value:'6'})}),promo('exclusive')],{items:[item('a','10',{original_total:'12'})]})
 await add('shipping-once-reject-keeps-quota',[promo('p',{application_method:method({target_type:'shipping_methods',allocation:'once',max_quantity:1,value:'6'}),campaign:{budget:budget('spend','5')}})],{shipping_methods:[{id:'expensive',subtotal:'10'},{id:'cheap',subtotal:'3'}]})
 await add('shipping-across-shared',[promo('s1',{application_method:method({target_type:'shipping_methods',allocation:'across',value:'5'})}),promo('s2',{application_method:method({target_type:'shipping_methods',type:'percentage',value:'10'})})],{shipping_methods:[{id:'a',subtotal:'10'},{id:'b',subtotal:'15'}]})
 const bg=promo('gift',{type:'buyget',application_method:method({value:'2',buy_rules_min_quantity:1,apply_to_quantity:1,max_quantity:2,buy_rules:[rule('items.product_id','phone')],target_rules:[rule('items.product_id','gift')]})})
 const bgctx={items:[item('buy','20',{quantity:2}),item('gift','10',{quantity:2,product_id:'gift'})]}
 await add('buyget-before-standard',[promo('standard',{application_method:method({value:'3'})}),bg],bgctx)
 await add('buyget-budget-reject-consumes-membership',[{...bg,campaign:{budget:budget('spend','1')}},{...bg,id:'gift2',code:'gift2'}],bgctx)
 await add('attribute-existing-exceeded',[promo('p',{campaign:{budget:{...budget('use_by_attribute','1'),attribute:'customer_id'}}})],{...ctx,customer:{id:'c'}},{budget:{attribute:'customer_id',value:'c',found:true,used:'1'}})
 await add('attribute-missing-row',[promo('p',{campaign:{budget:{...budget('use_by_attribute','0'),attribute:'customer_email'}}})],{...ctx,email:'artificial@example.test'},{budget:{attribute:'customer_email',value:'artificial@example.test',found:false,used:''}})
 await add('attribute-boundary',[promo('p',{campaign:{budget:{...budget('use_by_attribute','2'),attribute:'customer_id'}}})],{...ctx,customer_id:'c'},{budget:{attribute:'customer_id',value:'c',found:true,used:'1'}})
 await add('missing-method',[promo('p',{application_method:null})],ctx)
 const sources=['services/promotion-module.js','utils/compute-actions/usage.js','utils/compute-actions/line-items.js','utils/compute-actions/shipping-methods.js','utils/compute-actions/buy-get.js']
 const content=JSON.stringify({format_version:1,scope:'Original pure computeActions body with artificial read services; no SQL, registration, adjustment persistence or checkout',sources:sources.map(f=>({file:path.relative(root,path.join(dir,f)).replaceAll('\\','/'),sha256:crypto.createHash('sha256').update(fs.readFileSync(path.join(dir,f),'utf8').replaceAll('\r\n','\n')).digest('hex')})),cases},null,2)+'\n'
 const target=path.join(root,'docs/migration/fixtures/promotion-compute.json')
 assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
 if(process.argv[2]) assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
 else fs.writeFileSync(target,content)
 console.log(JSON.stringify({original_compute_cases:cases.length,application_started:false}))
}
main().catch(e=>{console.error(e);process.exitCode=1})
