// Pure original item allocator, artificial no-budget/no-target-rule promotions.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),store=path.join(root,'medusa-store/node_modules/.pnpm')
const pkg=prefix=>path.join(store,fs.readdirSync(store).find(p=>p.startsWith(prefix)))
const dir=path.join(pkg('@medusajs+promotion@'),'node_modules/@medusajs/promotion/dist/utils/compute-actions')
const utils=path.join(pkg('@medusajs+utils@'),'node_modules/@medusajs/utils/dist/totals')
// Explicit configuration for this standalone evidence process only.
process.env.MEDUSA_EPSILON='0.0001'
const {getComputedActionsForItems}=require(path.join(dir,'line-items.js'))
const scalar=v=>v?.bigNumber?.toFixed?v.bigNumber.toFixed():v?.toFixed?v.toFixed():String(v)
// Independent integer decimal addition for expected shared balances; no float,
// source allocator arithmetic or percentage/division code used for the oracle.
function addDecimal(a,b) {
 const scale=Math.max((a.split('.')[1]||'').length,(b.split('.')[1]||'').length)
 const integer=s=>{const [i,f='']=s.split('.');return BigInt(i+f.padEnd(scale,'0'))}
 const result=(integer(a)+integer(b)).toString().padStart(scale+1,'0')
 return scale? (result.slice(0,-scale)+'.'+result.slice(-scale)).replace(/0+$/,'').replace(/\.$/,''):result
}
const cases=[]
const line=(id,subtotal,quantity=1,original_total=subtotal,is_discountable=true)=>({id,subtotal,quantity,original_total,is_discountable})
function add(name,allocation,type,value,max_quantity,items,previous,expected,inclusive=false) {
 const input={allocation,type,value,epsilon:'0.0001',is_tax_inclusive:inclusive,max_quantity,items,previous:Object.fromEntries(previous)}
 const promo={code:'FIXTURE',type:'standard',is_tax_inclusive:inclusive,application_method:{target_type:'items',allocation,type,value,...(max_quantity===null?{}:{max_quantity})}}
 const map=new Map(previous),before=JSON.stringify(input)
 const actions=getComputedActionsForItems(promo,structuredClone(items),map)
 const amounts=actions.map(a=>{assert.equal(a.action,'addItemAdjustment');assert.equal(a.code,'FIXTURE');return {id:a.item_id,amount:scalar(a.amount),is_tax_inclusive:a.is_tax_inclusive}})
 const wanted=expected.map(([id,amount])=>({id,amount,is_tax_inclusive:inclusive}))
 assert.deepEqual(amounts,wanted,name)
 const wantedMap={...input.previous}
 for(const [id,amount,balance]of expected) {
  if(inclusive) assert.ok(balance!==undefined,'explicit untaxed expected balance required')
  wantedMap[id]=balance===undefined?addDecimal(input.previous[id]||'0',amount):balance
 }
 const discounts=Object.fromEntries([...map].map(([id,v])=>[id,scalar(v)]))
 assert.deepEqual(discounts,wantedMap,name+' shared untaxed balance')
 assert.equal(JSON.stringify(input),before)
 cases.push({name,input,expected:{amounts,discounts}})
}
add('each-default-one','each','fixed','10',null,[line('a','200',2)],[],[['a','10']])
add('each-explicit-two','each','fixed','10',2,[line('a','200',2)],[],[['a','20']])
add('each-quantity-capped','each','fixed','10',5,[line('a','200',2)],[],[['a','20']])
add('percentage-default-one','each','percentage','10',null,[line('a','200',2)],[],[['a','10']])
add('percentage-explicit-two','each','percentage','10',2,[line('a','200',2)],[],[['a','20']])
add('percentage-cap','each','percentage','150',null,[line('a','10')],[],[['a','10']])
add('each-zero-quota','each','fixed','10',0,[line('a','200',2)],[],[])
add('across-fixed','across','fixed','40',null,[line('a','100'),line('b','300')],[],[['a','10'],['b','30']])
add('across-zero-quota-absent','across','fixed','40',0,[line('a','100'),line('b','300')],[],[['a','10'],['b','30']])
add('across-max-quantity-denominator','across','fixed','30',1,[line('a','200',2),line('b','100')],[],[['a','9.9999999999999999999'],['b','9.9999999999999999999']])
add('across-percentage','across','percentage','50',null,[line('a','100'),line('b','300')],[],[['a','50'],['b','150']])
add('once-unit-price-order','once','fixed','10',3,[line('a','200',4),line('b','30')],[],[['b','10'],['a','20']])
add('once-stable-tie','once','fixed','5',1,[line('b','100',2),line('a','50')],[],[['b','5']])
add('once-default-zero','once','fixed','5',null,[line('a','100')],[],[])
add('once-consumes-eligible-units','once','fixed','10',2,[line('a','10',2),line('b','100')],[['a','9']],[['a','1']])
add('prior-cap','each','fixed','20',null,[line('a','10')],[['a','7']],[['a','3']])
add('prior-exhausted-skipped','once','fixed','10',1,[line('a','5'),line('b','20')],[['a','5']],[['b','10']])
add('not-discountable','each','fixed','10',null,[line('a','100',1,'100',false)],[],[])
add('zero-subtotal','each','fixed','10',null,[line('a','0')],[],[])
add('empty-preserves-map','each','fixed','10',null,[],[['other','7']],[])
add('epsilon-exact','each','fixed','0.0001',null,[line('a','1')],[],[])
add('epsilon-above','each','fixed','0.00010001',null,[line('a','1')],[],[['a','0.00010001']])
add('across-third','across','fixed','1',null,[line('a','1'),line('b','2')],[],[['a','0.33333333333333333333'],['b','0.66666666666666666667']])
add('negative-remainder-denominator','across','fixed','4',null,[line('a','1'),line('b','5')],[['a','2']],[['b','5']])
add('nonpositive-denominator','across','fixed','4',null,[line('a','1'),line('b','1')],[['a','2']],[])
add('inclusive-basic','each','fixed','25',null,[line('a','100',1,'125')],[],[['a','25','20']],true)
add('inclusive-prior-exclusive','each','fixed','25',null,[line('a','100',1,'125')],[['a','90']],[['a','12.5','100']],true)
add('inclusive-across','across','fixed','50',null,[line('a','100',1,'125'),line('b','300',1,'375')],[],[['a','12.5','10'],['b','37.5','30']],true)
add('inclusive-once-sorts-untaxed','once','fixed','11',1,[line('a','50',1,'55'),line('b','40',1,'80')],[],[['b','11','5.5']],true)
add('inclusive-zero-base','each','fixed','10',null,[line('a','100',1,'0')],[],[],true)
add('exact-large','each','percentage','50',null,[line('a','9007199254740993')],[],[['a','4503599627370496.5']])
add('across-over-cap-correction','across','fixed','10',null,[line('a','1'),line('b','2')],[],[['a','0.99999999999999999999'],['b','2']])
add('each-unit-third','each','fixed','1',null,[line('a','1',3)],[],[['a','0.33333333333333333333']])
const files=[path.join(dir,'line-items.js'),path.join(dir,'sort-by-price.js'),path.join(dir,'usage.js'),path.join(utils,'promotion/index.js'),path.join(utils,'big-number.js'),path.join(utils,'math.js')]
const sources=Object.fromEntries(files.map(file=>[path.relative(root,file).replaceAll('\\','/'),crypto.createHash('sha256').update(fs.readFileSync(file,'utf8').replaceAll('\r\n','\n')).digest('hex')]))
const content=JSON.stringify({format_version:1,scope:'Original no-budget item arithmetic; not campaign qualification or checkout',sources,cases},null,2)+'\n'
const target=path.join(root,'docs/migration/fixtures/item-allocation.json')
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({original_item_allocation_cases:cases.length,application_started:false}))
