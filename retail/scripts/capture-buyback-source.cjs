// Pinned private snapshot stdin -> private original-function oracle stdout.
// No service startup, database, credentials or provider calls.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto'),readline=require('node:readline'),{createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),req=createRequire(path.join(root,'medusa-store/apps/backend/package.json')),ts=req('typescript')
const source='medusa-store/apps/backend/src/lib/buyback-pricing.ts'
const code=ts.transpileModule(fs.readFileSync(path.join(root,source),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,exportsObject={}
vm.runInNewContext(code,{exports:exportsObject},{filename:source})
const PIN='b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985'
async function main(){
 const hash=crypto.createHash('sha256'),rows=[];let complete=false
 process.stdin.on('data',chunk=>hash.update(chunk))
 for await(const line of readline.createInterface({input:process.stdin,crlfDelay:Infinity})){
  if(line.length>16*1024*1024)throw Error('Source record exceeds policy')
  const r=JSON.parse(line);if(r.kind==='snapshot_complete')complete=true
  if(r.table==='buyback_price'&&r.cells)rows.push(Object.fromEntries(Object.entries(r.cells).map(([k,c])=>[k,c.sql_null?null:JSON.parse(c.value_json)])))
 }
 if(!complete||hash.digest('hex')!==PIN||rows.length!==61)throw Error('Pinned complete buyback source required')
 const healthy={battery:'90',checks:[true,true,true,true,true],faults:['none'],glass:['none'],screen:'none',frame:'none',back:'none'}
 const assessments=[healthy,{...healthy,battery:'79',faults:['pixels','lines'],glass:['cracked']},{...healthy,battery:'unknown',frame:'damaged',back:''},{...healthy,checks:[false,true,true,true,true]}]
 const active=rows.filter(r=>r.deleted_at===null),products=[...new Set(rows.map(r=>r.product_id))]
 const prices=products.map(product_id=>({product_id,prices:active.filter(r=>r.product_id===product_id).sort((a,b)=>b.created_at.localeCompare(a.created_at)||b.id.localeCompare(a.id)).slice(0,100).map(r=>({product_id:r.product_id,capacity:r.capacity,amount:Number(r.amount),currency_code:r.currency_code}))}))
 const estimates=[]
 for(const r of active)for(const assessment of assessments){
  const capacity=exportsObject.normalizeCapacity(r.capacity),match=active.find(p=>p.product_id===r.product_id&&p.capacity===capacity)
  estimates.push({input:{product_id:r.product_id,capacity:r.capacity,assessment},expected:{...exportsObject.assessmentPrice(match?.pricing_rules??null,assessment,match?Number(match.amount):null),currency_code:'eur'}})
 }
 process.stdout.write(JSON.stringify({snapshot_sha256:PIN,source_rows:rows.length,active_rows:active.length,prices,estimates}))
}
main().catch(()=>{process.stderr.write('Private buyback oracle failed\n');process.exitCode=1})
