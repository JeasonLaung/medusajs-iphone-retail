// Offline evaluation of the actual original Zod submission validator.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto'),{createRequire}=require('node:module'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),req=createRequire(path.join(root,'medusa-store/apps/backend/package.json')),ts=req('typescript')
const sources=['medusa-store/apps/backend/src/api/buyback-validators.ts','medusa-store/apps/backend/src/lib/recycled-assessment.ts']
function load(file){const exports={};const code=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;vm.runInNewContext(code,{exports,require:name=>name==='../lib/recycled-assessment'?load(sources[1]):name==='@medusajs/framework/zod'?req(name):(()=>{throw Error('Unreviewed dependency')})()},{filename:file});return exports}

const schema=load(sources[0]).BuybackCustomerCommand,cases=[]
function add(name,input){const result=schema.safeParse(input);cases.push({name,input,accepted:result.success,...(result.success?{expected:result.data}:{})})}
const identity={item_id:' item ',serial_number:' sn-ab ',imei:'000000000000001'}
add('normal-identities',{action:'submit_photos',items:[identity]})
for(const serial of ['ß','\ufeffsn\ufeff','😀'.repeat(50),'😀'.repeat(51),'x'.repeat(100),'x'.repeat(101),'','  '])add('serial-'+serial.length+'-'+cases.length,{action:'submit_photos',items:[{...identity,serial_number:serial}]})
for(const item_id of ['', ' ', 'i'.repeat(100),'i'.repeat(101), '😀'.repeat(50),'😀'.repeat(51)])add('id-'+cases.length,{action:'submit_photos',items:[{...identity,item_id}]})
for(const imei of ['000000000000000','12345678901234','1234567890123456','１２３４５６７８９０１２３４５',' 123456789012345','123456789012345 '])add('imei-'+cases.length,{action:'submit_photos',items:[{...identity,imei}]})
for(const n of [0,1,10,11])add('item-count-'+n,{action:'submit_photos',items:Array.from({length:n},()=>({...identity}))})
for(const key of ['item_id','serial_number','imei'])for(const kind of ['missing','null','unknown']){const item={...identity};if(kind==='missing')delete item[key];else if(kind==='null')item[key]=null;else item.private=true;add(key+'-'+kind,{action:'submit_photos',items:[item]})}
for(const action of ['submit_photos','mark_shipped','accept_offer','decline_offer','cancel','unknown']){
 const input={action,...(action==='submit_photos'?{items:[identity]}:action==='mark_shipped'?{tracking_number:' Track 1 '}:action==='accept_offer'?{final_amount:12.34}:{})}
 add('action-'+action,input);add('extra-'+action,{...input,private:'extra'});add('null-action-'+action,{...input,action:null})
}
for(const tracking_number of ['', '12',' 123 ', 'x'.repeat(150),'x'.repeat(151),'😀'.repeat(75),'😀'.repeat(76)])add('tracking-'+cases.length,{action:'mark_shipped',tracking_number})
for(const final_amount of [0,0.01,0.29,0.3,0.1+0.2,1.005,999999.99,1000000,1000000.01,-0.01,'1.00',null])add('amount-'+cases.length,{action:'accept_offer',final_amount})
const report={source_hashes:sources.map(p=>({path:p,sha256:crypto.createHash('sha256').update(fs.readFileSync(path.join(root,p))).digest('hex')})),cases},output=JSON.stringify(report)+'\n',target=path.join(root,'docs/migration/fixtures/buyback-customer-command.json')
if(process.argv.includes('--check'))assert.equal(fs.readFileSync(target,'utf8'),output);else fs.writeFileSync(target,output)
console.log(JSON.stringify({cases:cases.length,accepted:cases.filter(c=>c.accepted).length,check:process.argv.includes('--check')}))
