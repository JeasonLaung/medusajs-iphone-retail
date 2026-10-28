// Verify original static names and route projection without importing services.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict'),vm=require('node:vm')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..')
const requireOld=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const medusaRoot=path.dirname(requireOld.resolve('@medusajs/medusa'))
const requireMedusa=createRequire(requireOld.resolve('@medusajs/medusa'))
const translationRoot=path.dirname(requireMedusa.resolve('@medusajs/translation'))
const defaults=fs.readFileSync(path.join(translationRoot,'loaders/defaults.js'),'utf8').replaceAll('\r\n','\n')
const route=fs.readFileSync(path.join(medusaRoot,'api/store/locales/route.js'),'utf8').replaceAll('\r\n','\n')
const names=Object.fromEntries([...defaults.matchAll(/\{ code: "([^"]+)", name: "([^"]+)" \}/g)].map(m=>[m[1],m[2]]))
assert.equal(Object.keys(names).length,51)
assert.deepEqual(JSON.parse(fs.readFileSync(path.join(root,'retail/backend/internal/catalog/default_locales.json'),'utf8')),names)
const start=route.indexOf('async (req, res) => {'),end=route.indexOf('\n};\nexports.GET',start)
assert.ok(start>0&&end>start)
const get=vm.runInNewContext('('+route.slice(start,end)+'\n})',{utils_1:{ContainerRegistrationKeys:{QUERY:'query'}}},{timeout:1000})
;(async()=>{
 const cases=[]
 for(const codes of [[],['en-GB','zh-CN'],['de-DE','da-DK']]){
  let output
  await get({scope:{resolve:key=>{assert.equal(key,'query');return {graph:async input=>{assert.deepEqual(JSON.parse(JSON.stringify(input)),{entity:'store',fields:['supported_locales.*','supported_locales.locale.*'],pagination:{take:1}});return {data:[{supported_locales:codes.map(code=>({locale_code:code,locale:{name:names[code]}}))}]}}}}}},{json:value=>{output=JSON.parse(JSON.stringify(value))}})
  assert.deepEqual(output,{locales:codes.map(code=>({code,name:names[code]}))});cases.push({codes,output})
 }
 const hash=value=>crypto.createHash('sha256').update(value).digest('hex')
 const evidence=JSON.stringify({format_version:1,scope:'Artificial original GET projection and 51 default names only; translation feature flag, custom locale persistence and live legacy store are not exercised',source_sha256:{'@medusajs/translation/dist/loaders/defaults.js':hash(defaults),'@medusajs/medusa/dist/api/store/locales/route.js':hash(route)},default_names:51,cases},null,2)+'\n'
 const output=path.join(root,'docs/migration/fixtures/legacy-store-locales.json')
 if(process.argv.includes('--check'))assert.equal(fs.readFileSync(output,'utf8').replaceAll('\r\n','\n'),evidence)
 else fs.writeFileSync(output,evidence)
 console.log(JSON.stringify({original_store_locales:'passed',default_names:51,projection_cases:cases.length,external_resources_used:false}))
})().catch(error=>{console.error(error);process.exitCode=1})
