// Evaluate the repository's original pure TypeScript functions offline. No
// Medusa boot, credentials, database, network or customer data is involved.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto'),assert=require('node:assert/strict'),{createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),req=createRequire(path.join(root,'medusa-store/apps/backend/package.json')),ts=req('typescript')
const pricing='medusa-store/apps/backend/src/lib/buyback-pricing.ts',assessment='medusa-store/apps/backend/src/lib/recycled-assessment.ts',reference='medusa-store/apps/backend/src/lib/data/swappie-buyback-reference.json'
function load(name){const code=ts.transpileModule(fs.readFileSync(path.join(root,name),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;const exports={};vm.runInNewContext(code,{exports},{filename:name});return exports}
const p=load(pricing),a=load(assessment),source=JSON.parse(fs.readFileSync(path.join(root,reference),'utf8'))
const rules=entry=>({version:1,enabled:true,bases:Object.fromEntries(['LIKE_NEW','ALMOST_NEW','GOOD','MODERATE'].map(v=>[v,entry.bases[v]])),fault_costs:entry.fault_costs,damage_fee:entry.damage_fee,floor:entry.floor,battery_threshold:80,unknown_battery_issue:true})
const healthy={battery:'90',checks:[true,true,true,true,true],faults:['none'],glass:['none'],screen:'none',frame:'none',back:'none'},base=rules(source.configurations[0]),cases=[],matrices=[],configurations=[]
for(const entry of source.configurations){
 const r=rules(entry),rules_index=configurations.length;configurations.push(r)
 for(const [key,row]of Object.entries(entry.matrix)){
  const [visual,faults]=key.split(':');if(visual==='SEALED_BOX')continue
  const selected=faults?faults.split(','):[];assert.equal(p.calculatePrice(r,visual,selected),row.amount)
  matrices.push({rules_index,visual,faults:selected,amount:row.amount})
 }
}
function add(name,patch={},r=base,legacy=null){const input={...healthy,...patch};cases.push({name,rules:r,assessment:input,legacy,expected:p.assessmentPrice(r,input,legacy),complete:a.completeAssessment(input),required_photos:['front','back',...(['damaged','visible'].includes(input.frame)?['edges']:[])]})}
add('healthy');add('many-screen-faults',{faults:['bright','pixels','lines'],glass:['cracked','chipped'],battery:'79'});add('ineligible',{checks:[false,true,true,true,true]});add('ineligible-before-disabled',{checks:[true,false,true,true,true]}, {...base,enabled:false});add('disabled',{}, {...base,enabled:false});add('legacy',{},null,123.123456789);add('missing',{},null)
for(const battery of ['','unknown','001','79','80','100'])add('battery-'+battery,{battery})
add('unknown-without-cost',{battery:'unknown'},{...base,unknown_battery_issue:false})
for(const frame of ['','damaged','visible','some','minimal','none'])for(const back of ['','damaged','visible','some','minimal','none'])for(const screen of ['','visible','some','minimal','none'])add('grades-'+frame+'-'+back+'-'+screen,{frame,back,screen})
for(let mask=0;mask<32;mask++){const checks=Array.from({length:5},(_,i)=>!!(mask&(1<<i)));add('checks-'+mask,{checks})}
add('nullable',{checks:[null,true,null,true,null]});add('empty-multi',{faults:[],glass:[]});add('mixed-none',{faults:['none','pixels']});add('broken-screen-skips-grade',{faults:['pixels'],screen:''});add('frame-skips-back',{frame:'damaged',back:''})
const normalize=['1024 GB','2048gb','256 GB','1\u00a0TB','\ufeff2TB','128GB','1\u0085TB'].map(input=>({input,expected:p.normalizeCapacity(input)}))
const output={source_hashes:[pricing,assessment,reference].map(name=>({path:name,sha256:crypto.createHash('sha256').update(fs.readFileSync(path.join(root,name))).digest('hex')})),rules:configurations,matrix_cases:matrices,assessment_cases:cases,normalize}
const target=path.join(root,'docs/migration/fixtures/buyback-pricing.json'),encoded=JSON.stringify(output)+'\n'
if(process.argv.includes('--check'))assert.equal(fs.readFileSync(target,'utf8'),encoded,'Original pricing fixture drifted')
else fs.writeFileSync(target,encoded)
console.log(JSON.stringify({matrix:matrices.length,assessments:cases.length,normalize:normalize.length,check:process.argv.includes('--check')}))
