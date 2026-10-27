// Pinned private NDJSON stdin -> private expected quotes stdout. No application
// boot, database, network, provider, session or source-file writes.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),readline=require('node:readline')
const {createRequire}=require('node:module')
const backend=createRequire(path.resolve(__dirname,'../../medusa-store/apps/backend/package.json'))
const medusa=createRequire(backend.resolve('@medusajs/medusa'))
const Service=medusa('@medusajs/pricing').default.service
const pricingPackage=JSON.parse(fs.readFileSync(path.resolve(path.dirname(medusa.resolve('@medusajs/pricing')),'../package.json'),'utf8'))
if(pricingPackage.version!=='2.21.2')throw Error('Reviewed legacy pricing version required')
const framework=createRequire(backend.resolve('@medusajs/framework/utils'))
const utils=framework('@medusajs/utils')
const PIN='b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985'
const wanted=new Set(['price','price_rule','price_list','price_preference','region','store','sales_channel','product_variant_price_set','price_set'])
const active=row=>row.deleted_at===null
const numeric=/^-?\d+(\.\d+)?([eE][+-]?\d+)?$/
function exactJSON(text){return JSON.parse(text,(key,value,ctx)=>typeof value==='number'?(key==='precision'?value:ctx.source):value)}
function cell(cell){if(cell.sql_null)return null;return numeric.test(cell.value_json)?cell.value_json:exactJSON(cell.value_json)}
const exact=value=>utils.MathBN.convert(value).toFixed()
async function main(){
 const hash=crypto.createHash('sha256'),tables=Object.fromEntries([...wanted].map(key=>[key,[]]));let complete=false
 process.stdin.on('data',chunk=>hash.update(chunk))
 for await(const line of readline.createInterface({input:process.stdin,crlfDelay:Infinity})){
  if(line.length>16*1024*1024)throw Error()
  const record=JSON.parse(line);if(record.kind==='snapshot_complete')complete=true
  if(record.cells&&wanted.has(record.table))tables[record.table].push(Object.fromEntries(Object.entries(record.cells).map(([key,value])=>[key,cell(value)])))
 }
 if(!complete||hash.digest('hex')!==PIN||tables.store.filter(active).length!==1||tables.price_list.filter(active).length!==0)throw Error()
 const channel=tables.store.find(active).default_sales_channel_id
 if(!tables.sales_channel.some(row=>row.id===channel&&active(row)&&row.is_disabled===false))throw Error()
 const sets=new Set(tables.price_set.filter(active).map(row=>row.id))
 const links=tables.product_variant_price_set.filter(row=>active(row)&&sets.has(row.price_set_id))
 const rules=tables.price_rule.filter(active),prefs=tables.price_preference.filter(active)
 const result={format_version:1,snapshot_sha256:PIN,legacy_version:pricingPackage.version,contexts:[]}
 for(const region of tables.region.filter(active))for(const quantity of [1,2]){
  const context={region_id:region.id,currency_code:region.currency_code,sales_channel_id:channel,quantity}
  // The pinned source has no price lists; matching is an independent projection
  // of repository constraints. The installed service owns reference/tax output.
  const prices=tables.price.filter(row=>{
   if(!active(row)||!sets.has(row.price_set_id)||row.currency_code!==context.currency_code||row.price_list_id!==null)return false
   if(row.min_quantity!==null&&utils.MathBN.gt(row.min_quantity,quantity)||row.max_quantity!==null&&utils.MathBN.lt(row.max_quantity,quantity))return false
   const own=rules.filter(rule=>rule.price_id===row.id)
   return row.rules_count!==null&&Number(row.rules_count)===own.length&&own.every(rule=>rule.operator==='eq'&&typeof context[rule.attribute]==='string'&&rule.value===context[rule.attribute])
  }).sort((a,b)=>Number(b.rules_count)-Number(a.rules_count)||utils.MathBN.convert(a.amount).comparedTo(b.amount)||(a.id<b.id?-1:a.id>b.id?1:0))
  const service=Object.create(Service.prototype)
  service.baseRepository_={getFreshManager:()=>({})}
  service.pricingRepository_={calculatePrices:async()=>prices}
  service.listPriceRules=async({price_id:ids})=>rules.filter(rule=>ids.includes(rule.price_id))
  service.listPricePreferences_=async()=>prefs.filter(pref=>pref.attribute==='region_id'&&pref.value===region.id||pref.attribute==='currency_code'&&pref.value===region.currency_code)
  const values=await service.calculatePrices({id:[...sets]},{context},{manager:{}})
  const bySet=new Map(values.map(value=>[value.id,{
   calculated_amount:exact(value.raw_calculated_amount??value.calculated_amount),
   original_amount:value.original_amount===null?null:exact(value.raw_original_amount??value.original_amount),
   currency_code:value.currency_code,
   is_calculated_price_tax_inclusive:value.is_calculated_price_tax_inclusive,
   is_original_price_tax_inclusive:value.is_original_price_tax_inclusive,
   is_calculated_price_price_list:value.is_calculated_price_price_list,
   is_original_price_price_list:value.is_original_price_price_list,
   calculated_price:{price_list_type:value.calculated_price.price_list_type},
  }]))
  const variants={}
  for(const link of links){if(Object.hasOwn(variants,link.variant_id))throw Error();variants[link.variant_id]=bySet.get(link.price_set_id)??null}
  result.contexts.push({region_id:region.id,quantity:String(quantity),variants})
 }
 if(result.contexts.length!==2)throw Error()
 process.stdout.write(JSON.stringify(result))
}
main().catch(()=>{process.stderr.write('Private store price oracle failed\n');process.exitCode=1})
