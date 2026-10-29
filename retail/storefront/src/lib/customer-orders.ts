export type CustomerOrder={id:string;display_id:string;custom_display_id:string|null;status:string;created_at:string;currency_code:string;total:string;original_total:string;items:{id:string;title:string;thumbnail:string|null;quantity:string;sku:string|null}[]}
export type CustomerOrderList={orders:CustomerOrder[];count:number;limit:number;offset:number}
const fail=()=>new Error('Customer orders unavailable')
const obj=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw fail();return v as Record<string,unknown>}
const keys=(v:Record<string,unknown>,names:string[])=>{if(Object.keys(v).length!==names.length||names.some(k=>!(k in v)))throw fail()}
const text=(v:unknown,max=4096):string=>{if(typeof v!=='string'||new TextEncoder().encode(v).length>max||/[\x00-\x1f\x7f]/.test(v))throw fail();return v}
const match=(v:unknown,re:RegExp)=>{const s=text(v);if(!re.test(s))throw fail();return s}
const id=(v:unknown)=>match(v,/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/)
const amount=(v:unknown)=>match(v,/^(?:0|[1-9][0-9]{0,191})(?:\.[0-9]{1,128})?$/)
export function orderListProjection(raw:unknown,offset:number):CustomerOrderList{
 const v=obj(raw);keys(v,['orders','count','limit','offset'])
 if(v.offset!==offset||v.limit!==20||!Number.isSafeInteger(v.count)||Number(v.count)<0||!Array.isArray(v.orders)||v.orders.length>20)throw fail()
 const seen=new Set<string>()
 const orders=v.orders.map(raw=>{
  const row=obj(raw);keys(row,['id','display_id','custom_display_id','status','created_at','currency_code','total','original_total','items'])
  const orderID=id(row.id);if(seen.has(orderID))throw fail();seen.add(orderID)
  const created=text(row.created_at);if(!/^\d{4}-\d\d-\d\dT/.test(created)||!Number.isFinite(Date.parse(created)))throw fail()
  const status=match(row.status,/^(pending|completed|archived|canceled)$/)
  if(!Array.isArray(row.items)||row.items.length>1000||!row.items.length)throw fail()
  const items=new Set<string>()
  return {id:orderID,display_id:match(row.display_id,/^[1-9][0-9]{0,18}$/),custom_display_id:row.custom_display_id===null?null:match(row.custom_display_id,/^[0-9]{19}$/),status,created_at:created,currency_code:match(row.currency_code,/^[a-z]{3}$/),total:amount(row.total),original_total:amount(row.original_total),items:row.items.map(raw=>{
   const item=obj(raw);keys(item,['id','title','thumbnail','quantity','sku']);const itemID=id(item.id);if(items.has(itemID))throw fail();items.add(itemID)
   return {id:itemID,title:text(item.title),thumbnail:item.thumbnail===null?null:text(item.thumbnail),quantity:amount(item.quantity),sku:item.sku===null?null:text(item.sku)}
  })}
 })
 if(orders.length>Math.max(0,Number(v.count)-offset))throw fail()
 return {orders,count:Number(v.count),limit:20,offset}
}

export function sumOrderQuantities(values:string[]):string{
 let result=0n
 for(const value of values){if(!/^(?:0|[1-9][0-9]{0,191})(?:\.[0-9]{1,128})?$/.test(value))throw fail();const [whole,part='']=value.split('.');result+=BigInt(whole)*10n**128n+BigInt(part.padEnd(128,'0'))}
 const whole=result/10n**128n,fraction=(result%10n**128n).toString().padStart(128,'0').replace(/0+$/,'')
 return whole.toString()+(fraction?'.'+fraction:'')
}
