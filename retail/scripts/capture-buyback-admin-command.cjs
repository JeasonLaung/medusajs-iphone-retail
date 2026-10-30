// Offline evaluation of the actual original Zod submission validator.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto'),{createRequire}=require('node:module'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),req=createRequire(path.join(root,'medusa-store/apps/backend/package.json')),ts=req('typescript')
const sources=['medusa-store/apps/backend/src/api/buyback-validators.ts','medusa-store/apps/backend/src/lib/recycled-assessment.ts']
function load(file){const exports={};const code=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;vm.runInNewContext(code,{exports,require:name=>name==='../lib/recycled-assessment'?load(sources[1]):name==='@medusajs/framework/zod'?req(name):(()=>{throw Error('Unreviewed dependency')})()},{filename:file});return exports}

const schema=load(sources[0]).BuybackAdminCommand,cases=[]
function add(name,input){const result=schema.safeParse(input);cases.push({name,input,accepted:result.success,...(result.success?{expected:result.data}:{})})}
const normal=[{action:'review_photo',photo_id:' photo ',review_status:'approved'},{action:'review_photo',photo_id:'p',review_status:'retake',note:' reason '},{action:'approve_shipping',shipping_instructions:' ship instructions '},{action:'mark_received'},{action:'cancel',note:' canceled '},{action:'mark_paid',payment_reference:' reference '},{action:'request_return',note:' return requested '},{action:'mark_returning',return_tracking:' track '},{action:'mark_returned'}]
for(const [i,input]of normal.entries()){
 add('normal-'+i,input);add('extra-'+i,{...input,private:true});add('null-'+i,{...input,action:null})
 for(const key of Object.keys(input)){const v={...input};delete v[key];add('missing-'+i+'-'+key,v);add('null-'+i+'-'+key,{...input,[key]:null})}
}
for(const note of ['', '12',' 123 ','x'.repeat(500),'x'.repeat(501),'😀'.repeat(250),'😀'.repeat(251)])add('review-note-'+cases.length,{action:'review_photo',photo_id:'p',review_status:'retake',note})
for(const photo_id of ['', ' ', 'x'.repeat(100),'x'.repeat(101),'😀'.repeat(50),'😀'.repeat(51)])add('photo-'+cases.length,{action:'review_photo',photo_id,review_status:'approved'})
for(const shipping_instructions of ['', '1234', ' 12345 ','x'.repeat(2000),'x'.repeat(2001),'😀'.repeat(1000),'😀'.repeat(1001)])add('shipping-'+cases.length,{action:'approve_shipping',shipping_instructions})
for(const note of ['', '12',' 123 ','x'.repeat(1000),'x'.repeat(1001)])add('cancel-note-'+cases.length,{action:'cancel',note})
for(const review_status of ['pending','expired','APPROVED'])add('status-'+review_status,{action:'review_photo',photo_id:'p',review_status})
for(const [action,key,max]of [['mark_paid','payment_reference',200],['request_return','note',1000],['mark_returning','return_tracking',150]])for(const value of ['', '12',' 123 ','x'.repeat(max),'x'.repeat(max+1),'😀'.repeat(max/2),'😀'.repeat(max/2+1)])add(action+'-'+cases.length,{action,[key]:value})
for(const key of ['payment_reference','return_tracking'])add('review-cross-arm-'+key,{action:'review_photo',photo_id:'p',review_status:'approved',[key]:'foreign'})
const report={source_hashes:sources.map(p=>({path:p,sha256:crypto.createHash('sha256').update(fs.readFileSync(path.join(root,p))).digest('hex')})),cases},output=JSON.stringify(report)+'\n',target=path.join(root,'docs/migration/fixtures/buyback-admin-command.json')
if(process.argv.includes('--check'))assert.equal(fs.readFileSync(target,'utf8'),output);else fs.writeFileSync(target,output)
console.log(JSON.stringify({cases:cases.length,accepted:cases.filter(c=>c.accepted).length,check:process.argv.includes('--check')}))
