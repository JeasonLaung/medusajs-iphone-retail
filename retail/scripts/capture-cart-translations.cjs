// Execute the installed source translation step with artificial collaborators.
// No application startup, environment loading, database or provider access.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),vm=require('node:vm'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..')
const old=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const flow=createRequire(old.resolve('@medusajs/medusa/core-flows'))
const dir=path.dirname(flow.resolve('@medusajs/core-flows'))
const files={step:path.join(dir,'cart/steps/update-cart-items-translations.js'),helper:path.join(dir,'common/utils/apply-translations-to-items.js'),refresh:path.join(dir,'cart/workflows/refresh-cart-items.js')}
const text=Object.fromEntries(Object.entries(files).map(([k,p])=>[k,fs.readFileSync(p,'utf8').replaceAll('\r\n','\n')]))
const apply=flow(files.helper).applyTranslationsToItems
const clone=v=>JSON.parse(JSON.stringify(v))
const item={id:'line-sample',variant_id:'variant-sample',product_id:'product-sample',title:'Saved title',subtitle:'Saved subtitle',product_title:'Saved product',product_description:null,product_subtitle:'Saved product subtitle',product_type:'Saved type',product_collection:'Saved collection',product_handle:'saved-handle',variant_title:'Saved variant'}
const variant={id:'variant-sample',title:'Translated variant',product:{id:'product-sample',title:'Translated product',description:'Translated description',subtitle:'Translated subtitle',handle:'translated-handle',type:{value:'Translated type'},collection:{title:'Translated collection'}}}
function load(enabled){
 const exports={}
 vm.runInNewContext(text.step,{exports,require:name=>{
  if(name==='@medusajs/framework/utils')return {FeatureFlag:{isFeatureEnabled:key=>{assert.equal(key,'translation');return enabled}},Modules:{CART:'cart'},ContainerRegistrationKeys:{QUERY:'query'},deduplicate:v=>[...new Set(v)]}
  if(name==='@medusajs/framework/workflows-sdk')return {createStep:(id,run,compensate)=>({id,run,compensate}),StepResponse:class{constructor(output,compensation){this.output=output;this.compensation=compensation}}}
  if(name==='../../common/utils/apply-translations-to-items')return {applyTranslationsToItems:apply}
  if(name==='../utils/fields')return flow(path.join(dir,'cart/utils/fields.js'))
  throw Error('Unexpected source dependency: '+name)
 }},{timeout:1000})
 return exports.updateCartItemsTranslationsStep
}
;(async()=>{
 const cases=[]
 for(const [name,items] of [['with-variant',[item]],['without-variant',[{...item,variant_id:null}]],['empty',[]]]){
  let resolutions=0
  const result=await load(false).run({cart_id:'cart-sample',locale:'zh-CN',items:clone(items)},{container:{resolve(){resolutions++;throw Error('Disabled translation must not resolve services')}}})
  assert.equal(resolutions,0);assert.equal(result.output,undefined);assert.deepEqual(clone(result.compensation),[])
  cases.push({name:'disabled-'+name,service_resolutions:resolutions,compensation:clone(result.compensation)})
 }
 let writes=[],queries=[]
 const container={resolve(key){
  if(key==='cart')return {updateLineItems:async batch=>writes.push(clone(batch))}
  assert.equal(key,'query');return {graph:async(query,options)=>{queries.push({query:clone(query),options:clone(options)});return {data:[clone(variant)]}}}
 }}
 const enabled=load(true),result=await enabled.run({cart_id:'cart-sample',locale:'zh-CN',items:[clone(item)]},{container})
 assert.equal(queries.length,1);assert.deepEqual(queries[0].options,{locale:'zh-CN'});assert.deepEqual(queries[0].query.filters,{id:['variant-sample']})
 assert.equal(writes[0][0].title,'Translated product');assert.equal(writes[0][0].variant_title,'Translated variant')
 // Original type guards preserve null and string type/collection snapshots;
 // product.type/collection are objects. Unprefixed subtitle remains saved.
 assert.equal(writes[0][0].product_description,null);assert.equal(writes[0][0].subtitle,'Saved subtitle');assert.equal(writes[0][0].product_type,'Saved type');assert.equal(writes[0][0].product_collection,'Saved collection')
 await enabled.compensate(result.compensation,{container});assert.equal(writes[1][0].title,item.title)
 cases.push({name:'enabled-translated-snapshot-and-compensation',queries,writes})
 // Missing variants preserve the source snapshots; no variant does not query.
 assert.deepEqual(apply([clone(item)],[]),[item]);assert.deepEqual(apply([{...item,variant_id:null}],[variant]),[{...item,variant_id:null}])
 cases.push({name:'missing-variant-preserves-snapshot'});
 const source_sha256=Object.fromEntries(Object.entries(text).map(([k,v])=>[path.relative(root,files[k]),crypto.createHash('sha256').update(v).digest('hex')]))
 const out=JSON.stringify({format_version:1,scope:'Installed source callback with controlled translation flag and artificial query/cart services; not effective legacy runtime flag, real translated rows or Gin translation implementation',source_sha256,cases},null,2)+'\n'
 const dest=path.join(root,'docs/migration/fixtures/legacy-cart-translations.json')
 if(process.argv.includes('--check'))assert.equal(fs.readFileSync(dest,'utf8').replaceAll('\r\n','\n'),out)
 else fs.writeFileSync(dest,out)
 console.log(JSON.stringify({original_cart_translation_cases:cases.length,passed:true,external_resources_used:false}))
})().catch(e=>{console.error(e);process.exitCode=1})
