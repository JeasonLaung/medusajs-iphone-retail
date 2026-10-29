// Original buy-get, artificial no-budget contexts. No application or database.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),store=path.join(root,'medusa-store/node_modules/.pnpm')
const pkg=fs.readdirSync(store).find(p=>p.startsWith('@medusajs+promotion@'))
const dir=path.join(store,pkg,'node_modules/@medusajs/promotion/dist/utils')
const {getComputedActionsForBuyGet}=require(path.join(dir,'compute-actions/buy-get.js'))
const rule=ids=>[{attribute:'items.id',operator:'in',values:ids.map(value=>({value}))}]
const scalar=v=>v?.bigNumber?.toFixed?v.bigNumber.toFixed():v?.toFixed?v.toFixed():String(v)
const cases=[]
const row=(id,subtotal,quantity,buy_eligible=true,target_eligible=true)=>({id,subtotal,quantity,buy_eligible,target_eligible})
const quantities=rows=>rows.map(([id,quantity])=>({id,quantity}))
function add(name,items,minimum_buy,apply_quantity,max_quantity,value,type,expectedAmounts,expectedBuy,expectedTarget,previous={},buy_consumed={},target_consumed={},expectedDiscounts=null) {
 const code='FIXTURE',input={code,type,value,minimum_buy,apply_quantity,max_quantity,items,previous,buy_consumed,target_consumed}
 const buyIDs=items.filter(i=>i.buy_eligible).map(i=>i.id),targetIDs=items.filter(i=>i.target_eligible).map(i=>i.id)
 const promo={code,type:'buyget',application_method:{target_type:'items',type,value,buy_rules_min_quantity:minimum_buy,apply_to_quantity:apply_quantity,...(max_quantity===null?{}:{max_quantity}),buy_rules:rule(buyIDs),target_rules:rule(targetIDs)}}
 const map=new Map(Object.entries(previous)),buys=new Map(Object.entries(buy_consumed).map(([c,v])=>[c,v.map(r=>({item_id:r.id,quantity:r.quantity}))])),targets=new Map(Object.entries(target_consumed).map(([c,v])=>[c,v.map(r=>({item_id:r.id,quantity:r.quantity}))]))
 const before=JSON.stringify(input)
 const actions=getComputedActionsForBuyGet(promo,items.map(i=>({id:i.id,subtotal:i.subtotal,quantity:i.quantity})),map,buys,targets)
 const amounts=actions.map(a=>{assert.equal(a.action,'addItemAdjustment');assert.equal(a.code,code);return {id:a.item_id,amount:scalar(a.amount)}})
 assert.deepEqual(amounts,expectedAmounts.map(([id,amount])=>({id,amount})),name)
 const convert=m=>Object.fromEntries([...m].map(([c,rows])=>[c,rows.map(r=>({id:r.item_id,quantity:r.quantity}))]))
 const buy=convert(buys),target=convert(targets),discounts=Object.fromEntries([...map].map(([id,n])=>[id,scalar(n)]))
 const wantedBuy={...buy_consumed},wantedTarget={...target_consumed}
 if(items.length) {wantedBuy[code]=quantities(expectedBuy);wantedTarget[code]=quantities(expectedTarget)}
 assert.deepEqual(buy,wantedBuy,name+' buy reservations');assert.deepEqual(target,wantedTarget,name+' target reservations')
 // Test cases use exact integer balances unless explicitly specified.
 const wantedDiscounts=expectedDiscounts||{...previous,...Object.fromEntries(expectedAmounts.map(([id,amount])=>{assert.match(amount,/^[0-9]+$/);assert.match(previous[id]||'0',/^[0-9]+$/);return [id,(BigInt(previous[id]||'0')+BigInt(amount)).toString()]}))}
 assert.deepEqual(discounts,wantedDiscounts,name+' shared balance')
 assert.equal(JSON.stringify(input),before)
 cases.push({name,input,expected:{amounts,discounts,buy_consumed:buy,target_consumed:target}})
}
add('same-product-buy-two-get-one',[row('a','30',3)],2,1,null,'100','percentage',[['a','10']],[['a',2]],[['a',1]])
add('same-product-insufficient-target',[row('a','20',2)],2,1,null,'100','percentage',[],[],[])
add('prefer-buy-only-over-expensive-target',[row('a','100',1),row('b','50',1,true,false),row('c','40',1,true,false)],2,1,null,'100','percentage',[['a','100']],[['b',1],['c',1]],[['a',1]])
add('repeat-same-product',[row('a','90',9)],2,1,3,'100','percentage',[['a','30']],[['a',6]],[['a',3]])
add('maximum-limits-repeat',[row('a','90',9)],2,1,2,'100','percentage',[['a','20']],[['a',4]],[['a',2]])
add('zero-maximum',[row('a','30',3)],2,1,0,'100','percentage',[],[],[])
add('fixed-per-target-unit-cap',[row('a','10',1,true,false),row('b','6',2,false,true)],1,2,2,'10','fixed',[['b','6']],[['a',1]],[['b',2]])
add('target-descending-subtotal',[row('a','5',1,true,false),row('b','20',1,false,true),row('c','40',1,false,true)],1,1,1,'10','fixed',[['c','10']],[['a',1]],[['c',1]])
add('target-stable-tie',[row('a','5',1,true,false),row('c','20',1,false,true),row('b','20',1,false,true)],1,1,1,'10','fixed',[['c','10']],[['a',1]],[['c',1]])
add('buy-stable-tie',[row('b','20',1,true,false),row('a','20',1,true,false),row('c','10',1,false,true)],1,1,1,'5','fixed',[['c','5']],[['b',1]],[['c',1]])
add('buy-consumed-by-other-code',[row('a','30',3)],2,1,1,'100','percentage',[],[],[],{}, {OTHER:quantities([['a',2]])})
add('target-consumed-by-other-code',[row('a','5',1,true,false),row('b','10',1,false,true)],1,1,1,'100','percentage',[],[],[],{}, {},{OTHER:quantities([['b',1]])})
add('own-code-reservations-replaced',[row('a','30',3)],2,1,1,'100','percentage',[['a','10']],[['a',2]],[['a',1]],{},{FIXTURE:quantities([['a',3]])},{FIXTURE:quantities([['a',3]])})
add('zero-discount-still-consumes-quantities',[row('a','30',3)],2,1,1,'0','fixed',[],[['a',2]],[['a',1]])
add('previous-discount-does-not-cap-buyget',[row('a','30',3)],2,1,1,'100','percentage',[['a','10']],[['a',2]],[['a',1]],{a:'29'})
add('batch-allowance-not-decremented-between-lines',[row('a','5',1,true,false),row('b','10',1,false,true),row('c','20',1,false,true)],1,2,1,'100','percentage',[['c','20'],['b','10']],[['a',1]],[['c',1],['b',1]])
add('percentage-above-100-not-capped',[row('a','30',3)],2,1,1,'150','percentage',[['a','15']],[['a',2]],[['a',1]])
add('apply-zero-reserves-buy',[row('a','30',3)],2,0,1,'100','percentage',[],[['a',2]],[])
add('empty-preserves-current-maps',[],2,1,1,'100','percentage',[],[],[],{other:'7'},{FIXTURE:quantities([['old',1]])},{FIXTURE:quantities([['old',1]])})
const originalWarn=console.warn
let iterationWarnings=0
try {
 console.warn=message=>{assert.match(message,/exceeded maximum iterations \(1000\)/);iterationWarnings++}
 add('original-iteration-limit',[row('a','3003',3003)],2,1,2000,'1','fixed',[['a','1000']],[['a',2000]],[['a',1000]])
} finally {console.warn=originalWarn}
assert.equal(iterationWarnings,1)
// Explicit evidence of the source's lossy toNumber shared-map conversion.
const preciseItems=[row('a','27021597764222979',3)]
const p={code:'PRECISION',type:'buyget',application_method:{type:'fixed',value:'9007199254740993',buy_rules_min_quantity:2,apply_to_quantity:1,buy_rules:rule(['a']),target_rules:rule(['a'])}}
const m=new Map(),a=getComputedActionsForBuyGet(p,preciseItems,m,new Map(),new Map())
assert.equal(scalar(a[0].amount),'9007199254740993');assert.equal(String(m.get('a')),'9007199254740992')
const precisionException={name:'source-shared-map-toNumber-loss',item_subtotal:'27021597764222979',quantity:3,value:'9007199254740993',source_action:'9007199254740993',source_shared_balance:'9007199254740992',required_exact_shared_balance:'9007199254740993'}
const files=['compute-actions/buy-get.js','compute-actions/usage.js','validations/promotion-rule.js']
const sources=Object.fromEntries(files.map(f=>{const file=path.join(dir,f);return [path.relative(root,file).replaceAll('\\','/'),crypto.createHash('sha256').update(fs.readFileSync(file,'utf8').replaceAll('\r\n','\n')).digest('hex')]}))
const content=JSON.stringify({format_version:1,scope:'No-budget buy-get reservation and arithmetic; not full qualification or checkout',sources,cases,iteration_limit_case:'original-iteration-limit',precision_exception:precisionException},null,2)+'\n'
const target=path.join(root,'docs/migration/fixtures/buy-get.json')
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({original_buy_get_cases:cases.length,precision_exceptions:1,application_started:false}))
