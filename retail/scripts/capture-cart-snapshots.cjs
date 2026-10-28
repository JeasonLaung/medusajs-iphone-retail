// Execute only the installed pure preparation function with artificial inputs.
// No application, workflow, account, database, provider or environment access.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const flows=createRequire(backend.resolve('@medusajs/medusa/core-flows'))
const source=path.join(path.dirname(flows.resolve('@medusajs/core-flows')),'cart/utils/prepare-line-item-data.js')
const {prepareLineItemData}=require(source)
const variant={id:'v',title:'Current variant',sku:'000123',barcode:'000456',product:{id:'product',title:'Current title',subtitle:'Current subtitle',description:'Current description',handle:'current-handle',thumbnail:'current-image',is_giftcard:true,discountable:true}}
const base={quantity:1,title:'Saved title',requires_shipping:true,is_discountable:false,variant_option_values:{Saved:'Option'}}
const cases=[
 ['null-fallback',{item:{...base,product_id:'old-product',product_title:null,product_description:null,product_subtitle:'',thumbnail:'',variant_title:null,variant_sku:null,variant_barcode:'',metadata:null},variant},o=>{assert.equal(o.product_id,'product');assert.equal(o.product_title,'Current title');assert.equal(o.product_description,'Current description');assert.equal(o.product_subtitle,'');assert.equal(o.thumbnail,'');assert.equal(o.variant_title,'Current variant');assert.equal(o.variant_sku,'000123');assert.equal(o.variant_barcode,'');assert.deepEqual(o.metadata,{});assert.equal(o.is_giftcard,true)}],
 ['saved-fields',{item:{...base,product_title:'Historical title',product_description:'Historical description',product_handle:'historical-handle',metadata:{saved:true}},variant},o=>{assert.equal(o.product_title,'Historical title');assert.equal(o.product_description,'Historical description');assert.equal(o.product_handle,'historical-handle');assert.deepEqual(o.metadata,{saved:true});assert.equal(o.requires_shipping,true);assert.equal(o.is_discountable,false);assert.deepEqual(o.variant_option_values,{Saved:'Option'})}],
 ['manual',{item:{...base,is_giftcard:true,product_id:'historical-product',metadata:{manual:true}}},o=>{assert.equal(o.is_giftcard,false);assert.equal(o.product_id,'historical-product');assert.deepEqual(o.metadata,{manual:true})}],
 ['zero-compare',{item:{...base,compare_at_unit_price:0},variant:{...variant,calculated_price:{calculated_amount:60,original_amount:100,calculated_price:{price_list_type:'sale'}}}},o=>assert.equal(o.compare_at_unit_price,0)],
 ['sale-fallback',{item:base,variant:{...variant,calculated_price:{calculated_amount:60,original_amount:100,calculated_price:{price_list_type:'sale'}}}},o=>assert.equal(o.compare_at_unit_price,100)],
]
const results=cases.map(([name,input,check])=>{const output=prepareLineItemData(input);check(output);return {name,input,output}})
const text=JSON.stringify({format_version:1,scope:'Artificial pure original item preparation; not SQL or full refresh acceptance',source_file:path.relative(root,source).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(fs.readFileSync(source,'utf8').replaceAll('\r\n','\n')).digest('hex'),cases:results},null,2)+'\n'
const destination=path.join(root,'docs/migration/fixtures/cart-item-snapshots.json')
if(process.argv.slice(2).length>1 || process.argv[2] && process.argv[2]!=='--check')throw Error('Unknown option')
if(process.argv[2]==='--check')assert.equal(fs.readFileSync(destination,'utf8').replaceAll('\r\n','\n'),text,'Original snapshot preparation evidence drift')
else fs.writeFileSync(destination,text)
console.log(JSON.stringify({original_preparation_cases:cases.length,application_started:false}))
