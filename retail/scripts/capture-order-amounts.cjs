// Private stdin -> private stdout oracle. The caller must suppress/preserve
// stdout as private data. No source connection, application boot or HTTP.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),readline=require('node:readline')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..')
const backendRequire=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const frameworkRequire=createRequire(backendRequire.resolve('@medusajs/framework/utils'))
const utils=frameworkRequire('@medusajs/utils')
const utilsRoot=path.dirname(frameworkRequire.resolve('@medusajs/utils'))
const {decorateCartTotals}=require(path.join(utilsRoot,'totals/cart/index.js'))
const PIN='b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985'
const wanted=new Set(['order','order_item','order_line_item','order_line_item_adjustment','order_line_item_tax_line','order_shipping','order_shipping_method','order_shipping_method_adjustment','order_shipping_method_tax_line','order_credit_line','order_summary','currency'])
const numeric=/^-?\d+(\.\d+)?([eE][+-]?\d+)?$/
function exactJSON(text){return JSON.parse(text,(k,v,c)=>{if(typeof v!=='number')return v;if(!c?.source)throw Error();return k==='precision'?v:c.source})}
function cell(cell){
 if(cell.sql_null)return null
 const text=cell.value_json
 if(numeric.test(text))return text
 return exactJSON(text)
}
const active=r=>r.deleted_at===null
const byID=(a,b)=>a.id<b.id?-1:a.id>b.id?1:0
const fields=['total','subtotal','tax_total','discount_total','discount_subtotal','discount_tax_total','original_total','original_subtotal','original_tax_total','credit_line_total','credit_line_subtotal','credit_line_tax_total','item_total','item_subtotal','item_tax_total','item_discount_total','original_item_total','original_item_subtotal','original_item_tax_total','shipping_total','shipping_subtotal','shipping_tax_total','shipping_discount_total','original_shipping_total','original_shipping_subtotal','original_shipping_tax_total','refundable_total','refundable_total_per_unit','fulfilled_total','shipped_total','return_requested_total','return_received_total','return_dismissed_total','write_off_total']
const amounts=row=>Object.fromEntries(fields.filter(k=>row[k]?.bigNumber).map(k=>[k,row[k].bigNumber.toFixed()]))
const plain=n=>utils.MathBN.convert(n).toFixed()
async function main(){
 const hash=crypto.createHash('sha256'),tables=Object.fromEntries([...wanted].map(n=>[n,[]]));let complete=false
 process.stdin.on('data',chunk=>hash.update(chunk))
 const lines=readline.createInterface({input:process.stdin,crlfDelay:Infinity})
 for await(const line of lines){
  if(line.length>16*1024*1024)throw Error()
  const record=JSON.parse(line)
  if(record.kind==='snapshot_complete')complete=true
  if(record.cells&&wanted.has(record.table))tables[record.table].push(Object.fromEntries(Object.entries(record.cells).map(([k,v])=>[k,cell(v)])))
 }
 if(!complete||hash.digest('hex')!==PIN)throw Error()
 // The independent Go verifier also checks the full framing/counts before SQL.
 const result={format_version:1,snapshot_sha256:PIN,orders:[]}
 for(const order of tables.order.filter(active).sort(byID)){
  const current=r=>r.order_id===order.id&&r.version===order.version&&active(r)
  const orderItems=tables.order_item.filter(current).sort(byID)
  const versions=tables.order_shipping.filter(current).sort(byID)
  const currencies=tables.currency.filter(r=>r.code===order.currency_code&&active(r))
  if(currencies.length!==1)throw Error()
  const items=orderItems.map(i=>{
   const l=tables.order_line_item.find(l=>l.id===i.item_id&&active(l));if(!l)throw Error()
   return {...l,id:i.id,unit_price:i.unit_price??l.unit_price,raw_unit_price:i.raw_unit_price??l.raw_unit_price,quantity:i.quantity,raw_quantity:i.raw_quantity,detail:i,
    tax_lines:tables.order_line_item_tax_line.filter(t=>t.item_id===l.id&&active(t)),
    adjustments:tables.order_line_item_adjustment.filter(a=>a.item_id===l.id&&a.version===i.version&&active(a))}
  })
  const shipping=versions.map(v=>{
   const m=tables.order_shipping_method.find(m=>m.id===v.shipping_method_id&&active(m));if(!m)throw Error()
   return {...m,id:v.id,tax_lines:tables.order_shipping_method_tax_line.filter(t=>t.shipping_method_id===m.id&&active(t)),adjustments:tables.order_shipping_method_adjustment.filter(a=>a.shipping_method_id===m.id&&a.version===v.version&&active(a))}
  })
  const summaries=tables.order_summary.filter(current);if(summaries.length>1)throw Error()
  const summary=summaries[0]?.totals??undefined
  const value=decorateCartTotals({currency_code:order.currency_code,items,shipping_methods:shipping,credit_lines:tables.order_credit_line.filter(current),...(summary?{summary:structuredClone(summary)}:{})})
  const pending=value.summary?.pending_difference
  result.orders.push({order_id:order.id,version:Number(order.version),currency_code:order.currency_code,totals:amounts(value),
   lines:value.items.map((r,i)=>({id:orderItems[i].id,item_id:orderItems[i].item_id,unit_price:plain(r.unit_price),quantity:plain(r.quantity),totals:amounts(r)})),
   shipping_methods:value.shipping_methods.map((r,i)=>({id:versions[i].id,shipping_method_id:versions[i].shipping_method_id,name:r.name,amount:plain(r.amount),is_tax_inclusive:r.is_tax_inclusive,totals:amounts(r)})),
   summary:value.summary?{transaction_total:plain(value.summary.raw_transaction_total??value.summary.transaction_total??0),pending_difference:(pending.bigNumber??pending).toFixed()}:null})
 }
 if(result.orders.length!==13)throw Error()
 process.stdout.write(JSON.stringify(result))
}
main().catch(()=>{process.stderr.write('Private order arithmetic oracle failed; values suppressed\n');process.exitCode=1})
