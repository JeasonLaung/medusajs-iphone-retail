// Offline evaluation of the actual original Zod submission validator.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto'),{createRequire}=require('node:module'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),req=createRequire(path.join(root,'medusa-store/apps/backend/package.json')),ts=req('typescript')
const sources=['medusa-store/apps/backend/src/api/buyback-validators.ts','medusa-store/apps/backend/src/lib/recycled-assessment.ts']
function load(file){const exports={};const code=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;vm.runInNewContext(code,{exports,require:name=>name==='../lib/recycled-assessment'?load(sources[1]):name==='@medusajs/framework/zod'?req(name):(()=>{throw Error('Unreviewed dependency')})()},{filename:file});return exports}
const schema=load(sources[0]).CreateBuyback,assessment={battery:'90',checks:[true,true,true,true,true],faults:['none'],glass:['none'],screen:'none',frame:'none',back:'none'}
const base={submission_key:'123e4567-e89b-42d3-a456-426614174000',contact_name:' Customer ',phone:' 123456 ',return_address:{name:' Customer ',address_1:' Road 1 ',address_2:' ',city:' City ',postal_code:' 001 ',country_code:'dk'},reference_acknowledged:true,privacy_acknowledged:true,items:[{model:' iphone-13 ',capacity:'128GB',assessment}]}
const cases=[]
function add(name,fn=()=>{}){const input=structuredClone(base);fn(input);const result=schema.safeParse(input);cases.push({name,input,accepted:result.success,...(result.success?{expected:result.data}:{})})}
add('normal')
for(const key of Object.keys(base))add('missing-'+key,v=>delete v[key])
for(const key of Object.keys(base))add('null-'+key,v=>v[key]=null)
for(const key of ['reference_acknowledged','privacy_acknowledged'])for(const value of [false,'true',1])add(key+'-'+value,v=>v[key]=value)
for(const key of ['name','address_1','address_2','city','postal_code','country_code']){add('address-missing-'+key,v=>delete v.return_address[key]);add('address-null-'+key,v=>v.return_address[key]=null)}
for(const [key,min,max]of [['contact_name',2,100],['phone',5,50]])for(const n of [min-1,min,max,max+1])add(key+'-'+n,v=>v[key]='x'.repeat(n))
for(const [key,min,max]of [['name',2,100],['address_1',3,200],['address_2',0,200],['city',1,100],['postal_code',1,30]])for(const n of [Math.max(0,min-1),min,max,max+1])add('address-'+key+'-'+n,v=>v.return_address[key]='x'.repeat(n))
for(const value of ['DK','d','dkk',' dk ','éé','us'])add('country-'+value,v=>v.return_address.country_code=value)
for(const n of [0,1,10,11])add('items-'+n,v=>v.items=Array.from({length:n},()=>structuredClone(base.items[0])))
for(const value of ['128 GB','128gb','1024GB','1TB','0GB','999999999999999999999999GB','1.5TB'])add('capacity-'+value,v=>v.items[0].capacity=value)
for(const n of [0,1,150,151])add('model-'+n,v=>v.items[0].model='x'.repeat(n))
for(const s of ['\ufeff  Customer \u00a0','\u0085Customer\u0085','😀'.repeat(50),'😀'.repeat(51)])add('unicode-'+cases.length,v=>{v.contact_name=s;v.return_address.name=s})
for(const id of ['123E4567-E89B-42D3-A456-426614174000','00000000-0000-0000-0000-000000000000','ffffffff-ffff-ffff-ffff-ffffffffffff','123e4567-e89b-12d3-a456-426614174000','123e4567-e89b-82d3-a456-426614174000','123e4567-e89b-92d3-a456-426614174000','123e4567-e89b-42d3-7456-426614174000','123e4567e89b42d3a456426614174000','bad'])add('uuid-'+id,v=>v.submission_key=id)
for(const key of Object.keys(assessment)){add('assessment-missing-'+key,v=>delete v.items[0].assessment[key]);add('assessment-null-'+key,v=>v.items[0].assessment[key]=null)}
add('ineligible',v=>v.items[0].assessment.checks[0]=false);add('broken-other-check',v=>v.items[0].assessment.checks[3]=false)
add('null-check',v=>v.items[0].assessment.checks[4]=null);add('short-check',v=>v.items[0].assessment.checks.pop())
add('mixed-none',v=>v.items[0].assessment.faults=['none','pixels']);add('duplicate-fault',v=>v.items[0].assessment.faults=['pixels','pixels'])
add('skipped-grades',v=>{v.items[0].assessment.faults=['pixels'];v.items[0].assessment.screen='arbitrary';v.items[0].assessment.frame='damaged';v.items[0].assessment.back='anything'})
add('long-skipped-grade',v=>{v.items[0].assessment.faults=['pixels'];v.items[0].assessment.screen='x'.repeat(11)})
for(const value of ['','0','001','unknown','101','100','９０'])add('battery-'+value,v=>v.items[0].assessment.battery=value)
for(const level of ['root','address','item','assessment'])add('unknown-'+level,v=>{const target=level==='root'?v:level==='address'?v.return_address:level==='item'?v.items[0]:v.items[0].assessment;target.private_field='denied'})
const report={source_hashes:sources.map(p=>({path:p,sha256:crypto.createHash('sha256').update(fs.readFileSync(path.join(root,p))).digest('hex')})),cases},output=JSON.stringify(report)+'\n',target=path.join(root,'docs/migration/fixtures/buyback-submission.json')
if(process.argv.includes('--check'))assert.equal(fs.readFileSync(target,'utf8'),output);else fs.writeFileSync(target,output)
console.log(JSON.stringify({cases:cases.length,accepted:cases.filter(c=>c.accepted).length,check:process.argv.includes('--check')}))
