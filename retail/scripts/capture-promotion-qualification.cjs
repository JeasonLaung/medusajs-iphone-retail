// Original predicates/target filters/comparator, artificial facts only. No app,
// database, usage checks or checkout runs in this standalone evidence process.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict'),Module=require('node:module')
const root=path.resolve(__dirname,'../..'),store=path.join(root,'medusa-store/node_modules/.pnpm')
const dir=path.join(store,fs.readdirSync(store).find(p=>p.startsWith('@medusajs+promotion@')),'node_modules/@medusajs/promotion/dist/utils')
const files=['validations/promotion-rule.js','compute-actions/line-items.js','compute-actions/buy-get.js']
function expose(relative,name){
 const file=path.join(dir,relative),m=new Module(file,module)
 m.filename=file;m.paths=Module._nodeModulePaths(path.dirname(file))
 m._compile(fs.readFileSync(file,'utf8')+'\nexports.__capture='+name+';\n',file)
 return m.exports.__capture
}
const valid=require(path.join(dir,files[0])).areRulesValidForContext
const itemFilter=expose(files[1],'getValidItemsForPromotion')
const buyFilter=expose(files[2],'filterItemsByPromotionRules')
const sort=require(path.join(dir,files[2])).sortByBuyGetType
const rule=(attribute,value)=>({attribute,operator:'eq',values:[{value}]})
const item=(id,extra={})=>({id,subtotal:'10',quantity:1,product_id:'phone',...extra})
const cases=[]
function add(name,p,context){
 const m=p.method
 const original={type:p.type,application_method:m?{target_type:m.target,target_rules:m.targetRules||[],buy_rules:m.buyRules||[],buy_rules_min_quantity:m.minimumBuy}:null}
 const expected={applicable:false,items:[],shipping:[],buy:[],target:[]}
 if(m&&valid(p.rules||[],context)&&(!m.currency||m.currency===context.currency_code)){
  expected.applicable=true
  if(p.type==='buyget'){
   if(m.minimumBuy>0&&m.buyRules?.length&&context.items.length){
    const buy=new Set(buyFilter(context.items,m.buyRules).map(i=>i.id)),target=new Set(buyFilter(context.items,m.targetRules).map(i=>i.id))
    // Membership keeps input order; the allocator handles price priority.
    expected.buy=context.items.filter(i=>buy.has(i.id)).map(i=>i.id)
    expected.target=context.items.filter(i=>target.has(i.id)).map(i=>i.id)
   }
  }else if(m.target==='shipping_methods') expected.shipping=context.shipping_methods.filter(i=>valid(m.targetRules||[],i,'shipping_methods')).map(i=>i.id)
  else expected.items=itemFilter(context.items,original).map(i=>i.id)
 }
 cases.push({name,promotion:p,context,expected})
}
const standard={type:'standard',method:{target:'items',value:'1'}}
const context={currency_code:'eur',items:[item('a'),item('b',{product_id:'tablet'})]}
add('standard-all',standard,context)
add('target-product',{...standard,method:{...standard.method,targetRules:[rule('items.product_id','phone')]}},context)
add('order-rule-match',{...standard,rules:[rule('currency_code','eur')]},context)
add('order-rule-miss',{...standard,rules:[rule('currency_code','usd')]},context)
add('currency-match',{...standard,method:{...standard.method,currency:'eur'}},context)
add('currency-miss',{...standard,method:{...standard.method,currency:'usd'}},context)
add('currency-case',{...standard,method:{...standard.method,currency:'EUR'}},context)
add('missing-currency',{...standard,method:{...standard.method,currency:'eur'}},{items:context.items})
add('no-method',{type:'standard'},context)
add('empty-items',standard,{items:[]})
add('standard-filter',standard,{items:[null,item('yes'),item('disabled',{is_discountable:false}),item('zero',{subtotal:'0'}),item('negative',{subtotal:'-1'}),{id:'missing-subtotal',quantity:1},{id:'missing-quantity',subtotal:'1'}]})
add('discountable-truthiness',standard,{items:[false,null,0,'',true,1,'false',{},[]].map((v,i)=>item('i'+i,{is_discountable:v}))})
add('quantity-presence-not-validation',standard,{items:[item('zero',{quantity:0}),item('null',{quantity:null})]})
add('order-target',{...standard,method:{...standard.method,target:'order',targetRules:[rule('items.product_id','tablet')]}},context)
const shipping={type:'standard',method:{target:'shipping_methods',value:'1'}}
const methods=[{id:'express',type:'express',subtotal:'10'},{id:'slow',type:'standard',subtotal:'0',is_discountable:false},{id:'no-price',type:'express'}]
add('shipping-all',shipping,{shipping_methods:methods})
add('shipping-rules',{...shipping,method:{...shipping.method,targetRules:[rule('shipping_methods.type','express')]}},{shipping_methods:methods})
add('shipping-empty',shipping,{shipping_methods:[]})
const buyget={type:'buyget',method:{target:'items',value:'1',minimumBuy:1,buyRules:[rule('items.product_id','phone')],targetRules:[rule('items.product_id','tablet')]}}
add('buy-target-independent',buyget,context)
add('buy-no-discountable-filter',buyget,{items:[item('buy',{is_discountable:false}),item('gift',{product_id:'tablet',subtotal:'0',is_discountable:false})]})
add('buy-unrestricted-target',{...buyget,method:{...buyget.method,targetRules:[]}},context)
add('buy-no-rules',{...buyget,method:{...buyget.method,buyRules:[]}},context)
add('buy-no-minimum',{...buyget,method:{...buyget.method,minimumBuy:0}},context)
add('buy-empty',buyget,{items:[]})
const sorting=[]
function addSort(name,rows){
 const source=rows.map(p=>({code:p.code,type:p.type,application_method:{value:p.value,buy_rules_min_quantity:p.minimumBuy??null,apply_to_quantity:p.applyQuantity??null}}))
 sorting.push({name,input:rows,expected:source.sort(sort).map(p=>p.code)})
}
addSort('buy-first',[{code:'s',type:'standard',value:'100'},{code:'b',type:'buyget',value:'1'}])
addSort('same-type-value',[{code:'a',type:'standard',value:1},{code:'b',type:'standard',value:10},{code:'c',type:'standard',value:2}])
addSort('buy-quantity-ties',[{code:'a',type:'buyget',value:10,minimumBuy:1,applyQuantity:2},{code:'b',type:'buyget',value:10,minimumBuy:2,applyQuantity:1},{code:'c',type:'buyget',value:10,minimumBuy:2,applyQuantity:3}])
addSort('stable-ties',[{code:'z',type:'standard',value:10},{code:'a',type:'standard',value:10}])
const target=path.join(root,'docs/migration/fixtures/promotion-qualification.json')
const content=JSON.stringify({format_version:1,scope:'Original rule membership and sorting only; not amount, budget, trusted cart assembly or checkout',sources:files.map(f=>({file:path.relative(root,path.join(dir,f)).replaceAll('\\','/'),sha256:crypto.createHash('sha256').update(fs.readFileSync(path.join(dir,f),'utf8').replaceAll('\r\n','\n')).digest('hex')})),cases,sorting},null,2)+'\n'
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2]) assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({original_qualification_cases:cases.length,original_sort_cases:sorting.length,application_started:false}))
