// Installed manual provider only: artificial inputs, no app/DB/network/files.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const resolve=createRequire(backend.resolve('@medusajs/medusa/core-flows'))
const file=path.join(path.dirname(resolve.resolve('@medusajs/fulfillment-manual')),'services/manual-fulfillment.js')
const {ManualFulfillmentService}=require(file)
async function main(){
 const provider=new ManualFulfillmentService()
 const options=await provider.getFulfillmentOptions()
 assert.deepEqual(options,[{id:'manual-fulfillment'},{id:'manual-fulfillment-return',is_return:true}])
 const cases=[]
 for(const data of [{}, {tracking:'000123',nested:{label:'人工样例'}},null]){
  const input=structuredClone(data),result=await provider.validateFulfillmentData({id:'manual-fulfillment'},input,{id:'artificial-cart'})
  assert.deepEqual(result,data)
  cases.push({input:data,result})
 }
 assert.equal(await provider.canCalculate(),false)
 assert.equal(await provider.validateOption({id:'artificial'}),true)
 let message
 try{await provider.calculatePrice({},{},{})}catch(error){message=error.message}
 assert.equal(message,'Manual fulfillment does not support price calculation')
 const text=JSON.stringify({format_version:1,scope:'Artificial installed manual provider validation and capability only; no fulfillment/payment/order created',source_file:path.relative(root,file).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(fs.readFileSync(file,'utf8').replaceAll('\r\n','\n')).digest('hex'),identifier:ManualFulfillmentService.identifier,options,cases,can_calculate:false,calculation_error:message},null,2)+'\n'
 const target=path.join(root,'docs/migration/fixtures/manual-shipping.json')
 assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
 if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),text)
 else fs.writeFileSync(target,text)
 console.log(JSON.stringify({original_manual_validation_cases:cases.length,can_calculate:false,provider_network_called:false}))
}
main().catch(e=>{console.error(e.message);process.exitCode=1})
