export const progressKinds=['placed','paid','refund','allocated','released','reallocated','packed','shipped','delivered','fulfillment_canceled','canceled'] as const
export type ProgressKind=typeof progressKinds[number]
export type CustomerProgress={order_id:string;currency_code:string;events:{id:string;kind:ProgressKind;occurred_at:string;amount?:string}[];shipments:{id:string;shipped_at:string;delivered_at:string|null;labels:{tracking_number:string;tracking_url:string}[]}[]}
const object=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw Error('Invalid order progress');return v as Record<string,unknown>}
const keys=(v:Record<string,unknown>,allowed:string[])=>{if(Object.keys(v).some(k=>!allowed.includes(k)))throw Error('Invalid order progress')}
const text=(v:unknown,max=255)=>{if(typeof v!=='string'||[...v].length>max||/[\x00-\x1f\x7f]/.test(v))throw Error('Invalid order progress');return v}
const id=(v:unknown)=>{const s=text(v);if(!/^[A-Za-z0-9][A-Za-z0-9_:\-]{0,254}$/.test(s))throw Error('Invalid order progress');return s}
const date=(v:unknown)=>{const s=text(v,40);if(!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(s)||!Number.isFinite(Date.parse(s)))throw Error('Invalid order progress');return s}
export function customerProgressProjection(raw:unknown,orderID:string):CustomerProgress{
 const v=object(raw);keys(v,['order_id','currency_code','events','shipments'])
 if(v.order_id!==orderID||typeof v.currency_code!=='string'||!(/^[a-z]{3}$/).test(v.currency_code)||!Array.isArray(v.events)||v.events.length>2000||!Array.isArray(v.shipments)||v.shipments.length>100)throw Error('Invalid order progress')
 const seen=new Set<string>();let previous=-Infinity
 const events=v.events.map(raw=>{const e=object(raw);keys(e,['id','kind','occurred_at','amount']);const key=id(e.id),at=date(e.occurred_at)
  if(seen.has(key)||!progressKinds.includes(e.kind as ProgressKind)||Date.parse(at)<previous)throw Error('Invalid order progress');seen.add(key);previous=Date.parse(at)
  const out:CustomerProgress['events'][number]={id:key,kind:e.kind as ProgressKind,occurred_at:at}
  if('amount'in e){const amount=text(e.amount,321);if(!/^(0|[1-9][0-9]{0,190})(\.[0-9]{1,128})?$/.test(amount)||!['paid','refund'].includes(out.kind))throw Error('Invalid order progress');out.amount=amount}return out
 })
 seen.clear()
 const shipments=v.shipments.map(raw=>{const s=object(raw);keys(s,['id','shipped_at','delivered_at','labels']);const key=id(s.id)
  if(seen.has(key)||!Array.isArray(s.labels)||s.labels.length>100)throw Error('Invalid order progress');seen.add(key)
  return {id:key,shipped_at:date(s.shipped_at),delivered_at:s.delivered_at===null?null:date(s.delivered_at),labels:s.labels.map(raw=>{const l=object(raw);keys(l,['tracking_number','tracking_url']);const number=text(l.tracking_number,200),url=text(l.tracking_url,2048)
   if(url){const parsed=new URL(url);if(!['https:','http:'].includes(parsed.protocol)||parsed.username||parsed.password)throw Error('Invalid order progress')}return {tracking_number:number,tracking_url:url}
  })}
 })
 return {order_id:orderID,currency_code:v.currency_code,events,shipments}
}
