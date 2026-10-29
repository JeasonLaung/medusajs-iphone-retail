// Execute only the installed workflow's pure refresh transform, never an app.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const profile=JSON.parse(fs.readFileSync(path.join(root,'docs/migration/fixtures/shipping-profile-filter.json'),'utf8'))
const source=path.resolve(root,path.dirname(profile.source_file),'../workflows/refresh-cart-shipping-methods.js')
const code=fs.readFileSync(source,'utf8').replaceAll('\r\n','\n')
const marker='}, ({ cart, shippingOptions }) => {'
const start=code.indexOf(marker,code.indexOf('const shippingMethodsData ='))
const end=code.indexOf('\n        });\n        (0, workflows_sdk_1.parallelize)',start)
assert.ok(start>=0&&end>start)
const transform=vm.runInNewContext('(({ cart, shippingOptions }) => {'+code.slice(start+marker.length,end)+'\n})',{utils_1:backend('@medusajs/framework/utils')},{timeout:1000})
const cases=[]
const item={requires_shipping:true,variant:{product:{shipping_profile:{id:'wanted'}}}}
const wanted={id:'option',shipping_profile_id:'wanted',name:'Current',calculated_price:{calculated_amount:0,is_calculated_price_tax_inclusive:true}}
const orphan={id:'orphan',shipping_profile_id:'other',name:'Other',calculated_price:{calculated_amount:3,is_calculated_price_tax_inclusive:false}}
for(const [name,cart,options,removed,updated] of [
 ['zero',[{id:'method',shipping_option_id:'option',data:{saved:true}}],[wanted],[],['method']],
 ['single-orphan',[{id:'single',shipping_option_id:'orphan'}],[orphan],[],['single']],
 ['multiple-orphan',[{id:'method',shipping_option_id:'option'},{id:'single',shipping_option_id:'orphan'}],[wanted,orphan],['single'],['method']],
 ['missing-option',[{id:'missing',shipping_option_id:'missing'}],[wanted],['missing'],[]],
 ['missing-price',[{id:'method',shipping_option_id:'option'}],[{...wanted,calculated_price:null}],['method'],[]],
 ['empty',[],[wanted],[],[]]
]) {
 const input={cart:{items:[item],shipping_methods:cart},shippingOptions:options},before=JSON.stringify(input)
 const output=JSON.parse(JSON.stringify(transform(input)))
 assert.deepEqual(output.shippingMethodsToRemove,removed)
 assert.deepEqual(output.shippingMethodsToUpdate.map(r=>r.id),updated)
 assert.equal(JSON.stringify(input),before)
 cases.push({name,input,output})
}
const text=JSON.stringify({format_version:1,scope:'Original pure shipping refresh transform; not SQL, provider or full workflow acceptance',source_file:path.relative(root,source).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(code).digest('hex'),cases},null,2)+'\n'
const target=path.join(root,'docs/migration/fixtures/shipping-refresh.json')
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),text)
else fs.writeFileSync(target,text)
console.log(JSON.stringify({original_shipping_refresh_cases:cases.length,application_started:false}))
