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
 const result=JSON.stringify({format_version:1,scope:'Pure original prepare-cart-to-update callback only; not refresh, tax, shipping, promo or a committed transaction',source_sha256:crypto.createHash('sha256').update(source).digest('hex'),source:'@medusajs/core-flows/dist/cart/workflows/update-cart.js',cases:results},null,2)+'\n'
 const output=path.join(root,'docs/migration/fixtures/legacy-cart-region.json')
 if(process.argv.includes('--check'))assert.equal(fs.readFileSync(output,'utf8').replaceAll('\r\n','\n'),result)
 else fs.writeFileSync(output,result)
 console.log(JSON.stringify({original_cart_region_prepare:'passed',cases:results.length,external_resources_used:false}))
})().catch(error=>{console.error(error);process.exitCode=1})
