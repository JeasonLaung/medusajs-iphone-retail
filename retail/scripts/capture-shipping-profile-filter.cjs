// Installed cart profile filter only: no app, environment, DB or provider.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const resolve=createRequire(backend.resolve('@medusajs/medusa/core-flows'))
const source=path.join(path.dirname(resolve.resolve('@medusajs/core-flows')),'cart/utils/filter-items-by-shipping-profile.js')
const {filterCartItemsByShippingProfile}=require(source)
const items=[{id:'a',quantity:2,product_id:'stale',variant:{product:{shipping_profile:{id:'one'}}}},{id:'b',quantity:3,variant:{product:{shipping_profile:{id:'two'}}}},{id:'c',variant:{product:{}}},{id:'manual',product_id:'a'},{id:'d',quantity:1,variant:{product:{shipping_profile:{id:'one'}}}}]
const cases=[]
for(const [name,input,profile,expected] of [['first',items,'one',['a','d']],['second',items,'two',['b']],['unknown',items,'unknown',[]],['missing-profile',items,null,[]],['empty-items',[],'one',[]],['missing-items',null,'one',[]]]){
 const before=JSON.stringify(input),out=filterCartItemsByShippingProfile(input,profile)
 assert.deepEqual(out.map(r=>r.id),expected)
 assert.equal(JSON.stringify(input),before)
 cases.push({name,items:input,profile,expected_ids:out.map(r=>r.id)})
}
const text=JSON.stringify({format_version:1,scope:'Original pure profile projection only; not cart authority, SQL profile uniqueness or shipping coverage acceptance',source_file:path.relative(root,source).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(fs.readFileSync(source,'utf8').replaceAll('\r\n','\n')).digest('hex'),cases},null,2)+'\n'
const target=path.join(root,'docs/migration/fixtures/shipping-profile-filter.json')
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),text)
else fs.writeFileSync(target,text)
console.log(JSON.stringify({original_shipping_profile_cases:cases.length,application_started:false}))
