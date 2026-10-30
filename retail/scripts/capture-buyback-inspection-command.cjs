// Evaluate the original Zod validator; no application/database startup.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto'),{createRequire}=require('node:module'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),req=createRequire(path.join(root,'medusa-store/apps/backend/package.json')),ts=req('typescript')
const sources=['medusa-store/apps/backend/src/api/buyback-validators.ts','medusa-store/apps/backend/src/lib/recycled-assessment.ts']
function load(file){const exports={};const code=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;vm.runInNewContext(code,{exports,require:name=>name==='../lib/recycled-assessment'?load(sources[1]):name==='@medusajs/framework/zod'?req(name):(()=>{throw Error('Unreviewed dependency')})()},{filename:file});return exports}
const schema=load(sources[0]).BuybackAdminCommand,cases=[]
function add(name,input){const result=schema.safeParse(input);cases.push({name,input,accepted:result.success,...(result.success?{expected:result.data}:{})})}
const link={action:'link_device',item_id:' item ',device_id:' device '},offer={action:'final_offer',items:[{item_id:' item ',amount:294.01,notes:' inspected '}]}
for(const [index,normal]of [link,offer].entries()){
 add('normal-'+index,normal);add('unknown-'+index,{...normal,extra:1})
 for(const key of Object.keys(normal)){const missing={...normal};delete missing[key];add('missing-'+index+'-'+key,missing);add('null-'+index+'-'+key,{...normal,[key]:null});add('wrong-type-'+index+'-'+key,{...normal,[key]:42})}
}
for(const field of ['item_id','device_id'])for(const value of ['', ' ', 'x'.repeat(100),'x'.repeat(101),'😀'.repeat(50),'😀'.repeat(51),'\uFEFF ok \uFEFF'])add(field+'-'+cases.length,{...link,[field]:value})
for(const amount of [0,-0,-0.01,0.01,0.001,1e6,1e6+0.01,0.1+0.2,1.005,2.55,'2.55',null,NaN,Infinity])add('amount-'+cases.length,{...offer,items:[{...offer.items[0],amount}]})
for(const notes of ['', '12',' 123 ','x'.repeat(1000),'x'.repeat(1001),'😀'.repeat(500),'😀'.repeat(501),'\uFEFF yes \uFEFF'])add('notes-'+cases.length,{...offer,items:[{...offer.items[0],notes}]})
for(const key of ['item_id','amount','notes']){const item={...offer.items[0]};delete item[key];add('missing-item-'+key,{...offer,items:[item]});add('null-item-'+key,{...offer,items:[{...offer.items[0],[key]:null}]})}
add('unknown-item',{...offer,items:[{...offer.items[0],extra:1}]})
for(const length of [0,1,10,11])add('length-'+length,{...offer,items:Array.from({length},(_,i)=>({...offer.items[0],item_id:'item-'+i}))})
add('duplicate-item-input-valid',{...offer,items:[offer.items[0],offer.items[0]]})
add('ten-unicode-notes',{...offer,items:Array.from({length:10},(_,i)=>({...offer.items[0],item_id:'item-'+i,notes:'检'.repeat(1000)}))})
const report={source_hashes:sources.map(p=>({path:p,sha256:crypto.createHash('sha256').update(fs.readFileSync(path.join(root,p))).digest('hex')})),cases},output=JSON.stringify(report)+'\n',target=path.join(root,'docs/migration/fixtures/buyback-inspection-command.json')
if(process.argv.includes('--check'))assert.equal(fs.readFileSync(target,'utf8'),output);else fs.writeFileSync(target,output)
console.log(JSON.stringify({cases:cases.length,accepted:cases.filter(c=>c.accepted).length,check:process.argv.includes('--check')}))
