// Pure original prepare-cart-to-update callback, artificial inputs only.
// No app startup, env, database, account, provider or workflow execution.
const fs=require('node:fs')
const path=require('node:path')
const crypto=require('node:crypto')
const vm=require('node:vm')
const assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..')
const original=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const flowsRequire=createRequire(original.resolve('@medusajs/medusa/core-flows'))
const packageRoot=path.dirname(flowsRequire.resolve('@medusajs/core-flows'))
const sourceFile=path.join(packageRoot,'cart/workflows/update-cart.js')
const source=fs.readFileSync(sourceFile,'utf8').replaceAll('\r\n','\n')
const start=source.indexOf('async (data) => {'),end=source.indexOf('\n});\nexports.updateCartWorkflowId',start)
assert.ok(start>0&&end>start,'Original callback boundary changed')
class MedusaError extends Error {static Types={INVALID_DATA:'invalid_data'};constructor(type,message){super(message);this.type=type}}
class StepResponse {constructor(output){this.output=output}}
const prepare=vm.runInNewContext('('+source.slice(start,end)+'\n})',{utils_1:{MedusaError,isDefined:v=>v!==undefined},workflows_sdk_1:{StepResponse}},{timeout:1000})
function transform(marker){
 const at=source.indexOf(marker),begin=source.indexOf('=> {',at),finish=source.indexOf('\n    });',begin)
 assert.ok(at>0&&begin>at&&finish>begin,'Original transform boundary changed')
 const prefix=source.slice(at,begin),args=prefix.slice(prefix.lastIndexOf('}, (')+3)
 return vm.runInNewContext('('+args+source.slice(begin,finish)+'\n})',{utils_1:{isDefined:v=>v!==undefined}},{timeout:1000})
}
const taxChanged=transform('const taxRelevantAddressChanged =')
const regionUpdated=transform('const regionUpdated =')
const newLocale=transform('const newLocaleCode =')
const refresh=transform('const refreshCartItemsInput =')
const oldRegion={id:'region-eu',name:'Europe',currency_code:'eur',countries:[{iso_2:'dk'},{iso_2:'de'}]}
const other={id:'region-gb',name:'UK',currency_code:'gbp',countries:[{iso_2:'gb'}]}
const cases=[
 ['same-region-locale',{input:{id:'cart-test',locale:'zh-CN'},region:oldRegion,cartToUpdate:{region:oldRegion}}],
 ['same-region-with-promo',{input:{id:'cart-test',region_id:'region-eu',promo_codes:['TEST'],additional_data:{private:'sample'}},region:oldRegion,cartToUpdate:{region:oldRegion}}],
 ['new-single-country',{input:{id:'cart-test',region_id:'region-gb'},region:other,cartToUpdate:{region:oldRegion,shipping_address:{country_code:'dk',address_1:'sample'}}}],
 ['new-multiple-countries',{input:{id:'cart-test',region_id:'region-eu'},region:oldRegion,cartToUpdate:{region:other,shipping_address:{country_code:'gb'}}}],
 ['new-valid-explicit-address',{input:{id:'cart-test',region_id:'region-eu',shipping_address:{country_code:'de',city:'sample'}},region:oldRegion,cartToUpdate:{region:other}}],
 ['foreign-address',{input:{id:'cart-test',region_id:'region-gb',shipping_address:{country_code:'dk'}},region:other,cartToUpdate:{region:oldRegion}}],
 ['same-region-invalid-address',{input:{id:'cart-test',shipping_address:{country_code:'gb'}},region:oldRegion,cartToUpdate:{region:oldRegion}}],
 ['single-country-replaces-explicit-fields',{input:{id:'cart-test',region_id:'region-gb',shipping_address:{country_code:'gb',city:'discarded',address_1:'discarded'}},region:other,cartToUpdate:{region:oldRegion}}],
 ['new-multiple-without-country',{input:{id:'cart-test',region_id:'region-eu',shipping_address:{city:'sample'}},region:oldRegion,cartToUpdate:{region:other}}],
 ['same-region-preserves-partial-address',{input:{id:'cart-test',shipping_address:{city:'sample'}},region:oldRegion,cartToUpdate:{region:oldRegion}}],
 ['explicit-case-sensitive-country',{input:{id:'cart-test',shipping_address:{country_code:'DK'}},region:oldRegion,cartToUpdate:{region:oldRegion}}],
 ['clear-locale',{input:{id:'cart-test',locale:null},region:oldRegion,cartToUpdate:{region:oldRegion,locale:'en'}}],
]
const workflowCases=[
 ['unchanged',{},{}],['same-region-request',{region_id:oldRegion.id},{}],
 ['region-change',{region_id:other.id},{}],['locale-change',{locale:'zh-CN'},{locale:'en'}],
 ['same-locale',{locale:'en'},{locale:'en'}],['locale-null',{locale:null},{locale:'en'}],
 ['locale-empty',{locale:''},{locale:'en'}],
 ['unchanged-address',{shipping_address:{country_code:'dk',city:'sample'}},{shipping_address:{country_code:'DK',city:'sample'}}],
 ['address-city-change',{shipping_address:{city:'new'}},{shipping_address:{city:'old'}}],
 ['address-postal-change',{shipping_address:{postal_code:'2'}},{shipping_address:{postal_code:'1'}}],
 ['address-province-change',{shipping_address:{province:'new'}},{shipping_address:{province:'old'}}],
 ['address-id-change',{shipping_address:{id:'new'}},{shipping_address:{id:'old'}}],
 ['address-nontax-change',{shipping_address:{first_name:'new'}},{shipping_address:{first_name:'old'}}],
 ['address-explicit-null',{shipping_address:{city:null}},{shipping_address:{}}],
 ['address-unchanged-null',{shipping_address:{city:null}},{shipping_address:{city:null}}],
 ['country-null-undefined',{shipping_address:{country_code:null}},{shipping_address:{}}],
]
;(async()=>{
 const results=[]
 for(const [name,input] of cases){
  try{const result=await prepare(structuredClone(input));results.push({name,input,output:JSON.parse(JSON.stringify(result.output))})}
  catch(error){if(!(error instanceof MedusaError))throw error;results.push({name,input,error:{type:error.type,message:error.message}})}
 }
 assert.deepEqual(results[0].output,{id:'cart-test',locale:'zh-CN',currency_code:'eur',region_id:'region-eu'})
 assert.deepEqual(results[1].output,{id:'cart-test',region_id:'region-eu',currency_code:'eur'})
 assert.deepEqual(results[2].output.shipping_address,{country_code:'gb'})
 assert.equal(results[3].output.shipping_address,null)
 assert.deepEqual(results[4].output.shipping_address,{country_code:'de',city:'sample'})
 assert.equal(results[5].error.type,'invalid_data');assert.equal(results[6].error.type,'invalid_data')
 const decisions=workflowCases.map(([name,input,current])=>{
  const cartToUpdate={region:oldRegion,...current},data={input,cartToUpdate}
  const changed=regionUpdated(data),locale=newLocale(data),address=taxChanged(data)
  const refreshed=refresh({cartInput:{id:'cart-test'},input,newRegion:input.region_id?(input.region_id===other.id?other:oldRegion):undefined,newLocaleCode:locale,taxRelevantAddressChanged:address})
  return {name,input,cartToUpdate,output:{region_changed:changed,remove_custom_prices:!!changed,force_refresh:refreshed.force_refresh}}
 })
 assert.equal(decisions[1].output.force_refresh,true)
 assert.equal(decisions[1].output.remove_custom_prices,false)
 assert.equal(decisions[2].output.remove_custom_prices,true)
 assert.equal(decisions[5].output.force_refresh,false)
 assert.equal(decisions[7].output.force_refresh,false)
 assert.equal(decisions[13].output.force_refresh,true)
 const result=JSON.stringify({format_version:2,scope:'Pure original preparation and refresh decisions only; not refresh execution, tax, shipping, promo or a committed transaction',source_sha256:crypto.createHash('sha256').update(source).digest('hex'),source:'@medusajs/core-flows/dist/cart/workflows/update-cart.js',cases:results,workflow_cases:decisions},null,2)+'\n'
 const output=path.join(root,'docs/migration/fixtures/legacy-cart-region.json')
 if(process.argv.includes('--check'))assert.equal(fs.readFileSync(output,'utf8').replaceAll('\r\n','\n'),result)
 else fs.writeFileSync(output,result)
 console.log(JSON.stringify({original_cart_region_prepare:'passed',cases:results.length,workflow_cases:decisions.length,external_resources_used:false}))
})().catch(error=>{console.error(error);process.exitCode=1})
