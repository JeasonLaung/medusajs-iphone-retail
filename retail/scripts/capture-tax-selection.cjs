// Artificial original tax selection/provider evidence; no application or I/O
// beyond reading installed code and writing/checking the public fixture.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const resolve=createRequire(backend.resolve('@medusajs/medusa/core-flows'))
const packageRoot=path.dirname(resolve.resolve('@medusajs/tax'))
const serviceFile=path.join(packageRoot,'services/tax-module-service.js'),providerFile=path.join(packageRoot,'providers/system.js')
const Service=require(serviceFile).default,service=Object.create(Service.prototype),Provider=require(providerFile).default
const product={id:'item',product_id:'product',product_type_id:'type'},shipping={id:'shipping',shipping_option_id:'option'}
const rule=(reference,id)=>({reference,reference_id:id})
const rate=(id,province=false,rules=[],extra={})=>({id,rate:'25',name:'Sample tax',code:'SAMPLE',tax_region_id:province?'province':'country',tax_region:{id:province?'province':'country',province_code:province?'sample':null,parent_id:province?'country':null},is_default:rules.length===0,is_combinable:false,rules,...extra})
const defaults=[rate('country'),rate('country-type',false,[rule('product_type','type')]),rate('country-product',false,[rule('product','product')]),rate('province',true),rate('province-type',true,[rule('product_type','type')]),rate('province-product',true,[rule('product','product')])]
const cases=[]
function selection(name,item,rates,regions=['country','province'],expected){
 const query=service.getTaxRateQueryForItem(item,regions)
 const candidates=rates.filter(rate=>service.rateMatchesQuery(rate,query))
 const ids=service.getTaxRatesForItem(item,candidates).map(rate=>rate.id)
 assert.deepEqual(ids,expected,name)
 cases.push({name,item,rates,region_ids:regions,expected:ids})
}
selection('all-priorities',product,defaults,undefined,['province-product'])
selection('province-type-before-country-product',product,[defaults[2],defaults[4]],undefined,['province-type'])
selection('province-default-before-country-product',product,[defaults[2],defaults[3]],undefined,['province'])
selection('country-type-before-default',product,defaults.slice(0,2),undefined,['country-type'])
selection('parent-most-specific',product,[...defaults.slice(0,3),{...defaults[5],is_combinable:true}],undefined,['province-product','country-product'])
selection('noncombinable-no-parent',product,[defaults[0],defaults[5]],undefined,['province-product'])
selection('missing-parent',product,[{...defaults[5],is_combinable:true}],undefined,['province-product'])
selection('parent-not-recursive',product,[rate('province',true,[],{is_combinable:true}),rate('country',false,[],{is_combinable:true,tax_region:{id:'country',province_code:null,parent_id:'ancestor'}}),rate('ancestor',false,[],{tax_region_id:'ancestor',tax_region:{id:'ancestor',province_code:null,parent_id:null}})],['country','province','ancestor'],['province','country'])
selection('stable-tie',product,[rate('first'),rate('second')],undefined,['first'])
selection('stable-tie-reversed',product,[rate('second'),rate('first')],undefined,['second'])
selection('foreign-region',product,[rate('foreign',false,[],{tax_region_id:'foreign',tax_region:{id:'foreign',province_code:null,parent_id:null}})],undefined,[])
selection('unmatched-rule',product,[rate('other',false,[rule('product','other')])],undefined,[])
selection('unknown-reference',product,[rate('other',false,[rule('future','product')])],undefined,[])
selection('default-with-unmatched-rule',product,[rate('default',false,[rule('product','other')],{is_default:true})],undefined,['default'])
selection('shipping-own-rule',shipping,[rate('default'),rate('shipping',false,[rule('shipping_option','option')])],undefined,['shipping'])
selection('shipping-ignores-product',shipping,[defaults[2]],undefined,[])
selection('product-ignores-shipping',product,[rate('shipping',false,[rule('shipping_option','option')])],undefined,[])
selection('no-type-reference', {id:'item',product_id:'product'},[defaults[1]],undefined,[])
selection('empty-candidates',product,[],undefined,[])
selection('empty-region-directory',product,defaults,[],[])
selection('empty-province-is-province',product,[rate('country'),rate('empty',true,[],{tax_region:{id:'province',province_code:'',parent_id:'country'}})],undefined,['empty'])
selection('multiple-rules-one-match',product,[rate('multi',false,[rule('product','other'),rule('product_type','type')])],undefined,['multi'])
selection('parent-rule-must-match',product,[rate('child',true,[],{is_combinable:true}),rate('parent',false,[rule('product','other')])],undefined,['child'])
selection('same-parent-region-first',product,[rate('first'),rate('second'),rate('child',true,[],{is_combinable:true})],undefined,['child','first'])
;(async()=>{
 const provider=new Provider(),providerCases=[]
 const projectionCases=[
  ['empty',[]],['null-rate',[{item:product,rates:[rate('zero',false,[],{rate:null})]}]],
  ['exact-decimal',[{item:product,rates:[rate('exact',false,[],{rate:'25.00000000000000000001'})]}]],
  ['zero-rate',[{item:product,rates:[rate('zero',false,[],{rate:'0'})]}]],
  ['items-before-shipping',[{item:shipping,rates:[rate('shipping')]},{item:product,rates:[rate('first'),rate('second',true)]}]],
  ['no-selected-rates',[{item:product,rates:[]}]],
 ]
 for(const [name,groups] of projectionCases){
  const items=groups.filter(g=>!('shipping_option_id'in g.item)).map(g=>({line_item:g.item,rates:g.rates}))
  const ships=groups.filter(g=>'shipping_option_id'in g.item).map(g=>({shipping_line:g.item,rates:g.rates}))
  const actual=await provider.getTaxLines(items,ships,{})
  // JSON rate remains a decimal string in the new API, including legacy null=>0.
  const expected=actual.map(line=>({...line,rate:String(line.rate)}))
  assert.ok(expected.every(line=>line.provider_id==='system'))
  providerCases.push({name,groups,expected})
 }
 assert.equal(providerCases[1].expected[0].rate,'0')
 assert.equal(providerCases[2].expected[0].rate,'25.00000000000000000001')
 assert.equal(providerCases[4].expected[0].line_item_id,'item')
 const sources={}
 for(const file of [serviceFile,providerFile])sources[path.relative(root,file).replaceAll(path.sep,'/')]=crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex')
 const loaderFile=path.join(packageRoot,'loaders/providers.js'),loader=fs.readFileSync(loaderFile,'utf8')
 assert.ok(loader.includes('`tp_${klass.identifier}${pluginOptions.id ? `_${pluginOptions.id}` : ""}`'),'Original provider registration key changed')
 const reviewed_sources_not_executed={[path.relative(root,loaderFile).replaceAll(path.sep,'/')]:crypto.createHash('sha256').update(fs.readFileSync(loaderFile)).digest('hex')}
 const output=JSON.stringify({format_version:1,scope:'Original pure tax candidate query, stable selection and built-in system provider; artificial data, no DB/provider network/cart transaction',sources,reviewed_sources_not_executed,cases,provider_cases:providerCases},null,2)+'\n'
 const target=path.join(root,'docs/migration/fixtures/legacy-tax-selection.json')
 if(process.argv.includes('--check'))assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),output)
 else fs.writeFileSync(target,output)
 console.log(JSON.stringify({tax_selection_cases:cases.length,system_provider_cases:providerCases.length,source_files:2,external_resources_used:false}))
})().catch(error=>{console.error(error);process.exitCode=1})
