// Execute original pure workflow callbacks over artificial facts only.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict'),{createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),store=path.join(root,'medusa-store/node_modules/.pnpm')
const file=path.join(store,fs.readdirSync(store).find(p=>p.startsWith('@medusajs+core-flows@')),'node_modules/@medusajs/core-flows/dist/cart/workflows/refresh-payment-collection.js')
const source=fs.readFileSync(file,'utf8'),utils=createRequire(file)('@medusajs/framework/utils')
function callback(marker,signature,endMarker='\n    });'){const start=source.indexOf(signature,source.indexOf(marker)),end=source.indexOf(endMarker,start);assert.ok(start>0&&end>start);return new Function('utils_1','return '+source.slice(start,end)+'\n}')(utils)}
const should=callback('const shouldExecute =','({ input }) => {')
const changed=callback('"should-update-payment-collection"','({ cart, shouldExecute }) => {','\n    }).then')
function nested(marker,signature){const start=source.indexOf(signature,source.indexOf(marker)),end=source.indexOf('\n        });',start);assert.ok(start>0&&end>start);return new Function('utils_1','return '+source.slice(start,end)+'\n}')(utils)}
const remove=nested('const deletePaymentSessionInput','(data) => {')
const update=nested('const updatePaymentCollectionInput','({ cart }) => {')
const cases=[]
function add(name,provided,cartID,amount,currency,collection){
 const payment=collection?{id:collection.ID,raw_amount:{value:collection.Amount,precision:100},currency_code:collection.Currency,payment_sessions:collection.SessionIDs.map(id=>({id}))}:null
 const cart={id:'artificial-cart',raw_total:{value:amount,precision:100},currency_code:currency,payment_collection:payment}
 const input=provided?{cart}:cartID?{cart_id:cart.id}:{}
 const execute=should({input}),change=changed({cart,shouldExecute:execute}),patch=change?update({cart}):undefined
 const expected={ShouldExecute:execute,Changed:change,UpdateCollection:!!patch,CollectionID:collection?.ID??'',Amount:utils.MathBN.convert(amount).toFixed(),Currency:currency,DeleteSessionIDs:change?remove({paymentCollection:payment}).ids:[]}
 cases.push({name,input:{InputCartProvided:provided,InputCartIDPresent:cartID,Amount:amount,Currency:currency,Collection:collection},expected})
}
const c=(amount='235',currency='eur',sessions=['session-a','session-b'])=>({ID:'collection-a',Amount:amount,Currency:currency,SessionIDs:sessions})
add('provided-no-collection',true,false,'235','eur',null)
add('id-no-collection',false,true,'235','eur',null)
add('no-input',false,false,'235','eur',null)
add('unchanged',true,false,'235','eur',c())
add('equivalent-scale',true,false,'235.000','eur',c('2.35e2'))
add('changed-amount',true,false,'225','eur',c())
add('changed-currency',true,false,'235','usd',c())
add('sub-scalar-precision-change',true,false,'235.000000000000000000001','eur',c('235.000000000000000000002'))
add('large-exact-equality',true,false,'9007199254740993.123456789012345678901','eur',c('9007199254740993.123456789012345678901'))
add('zero-total',true,false,'0','eur',c())
add('changed-no-sessions',true,false,'200','eur',c('235','eur',[]))
add('id-unchanged',false,true,'235','eur',c())
add('id-currency-change',false,true,'235','usd',c())
add('no-execute-amount-change',false,false,'200','eur',c())
add('equal-currency-change-independent-execute',false,false,'235','usd',c())
const output={format_version:1,scope:'Original pure workflow callbacks only; no provider, database, application or compensation executed',source_file:path.relative(root,file).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(source.replaceAll('\r\n','\n')).digest('hex'),cases}
const target=path.join(root,'docs/migration/fixtures/cart-payment-refresh.json'),content=JSON.stringify(output,null,2)+'\n'
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({original_payment_refresh_decision_cases:cases.length,provider_calls:0,application_started:false}))
