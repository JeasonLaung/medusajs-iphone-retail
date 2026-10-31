export type PaymentReceipt={operation_id:string;previous_operation_id?:string;order_id:string;order_version:number;collection_id:string;amount:string;currency_code:string;state:'pending'|'uncertain'|'open'|'authorized'|'paid'|'failed'|'canceled'|'expired';applied:boolean;can_cancel:boolean;can_retry:boolean}
export type PaymentView=PaymentReceipt&{checkout_url:string|null}
export type PaymentConsent={expected_version:number;collection_id:string;amount:string;currency_code:string;previous_operation_id?:string}
export type PaymentInput=Omit<PaymentConsent,'expected_version'>&{expected_version:string;operation_id:string}
export type PaymentContext={order_id:string;order_version:number;currency_code:string;pending_difference:string;next:PaymentConsent|null;payments:PaymentView[]}
const fail=()=>Error('Order payment unavailable')
export const paymentUUID=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(v)
export const paymentOrderID=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(v)
const object=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw fail();return v as Record<string,unknown>}
const keys=(v:Record<string,unknown>,required:string[],optional:string[]=[])=>{if(required.some(k=>!(k in v))||Object.keys(v).some(k=>![...required,...optional].includes(k)))throw fail()}
const version=(v:unknown):v is number=>Number.isSafeInteger(v)&&Number(v)>0
const money=(v:unknown):v is string=>typeof v==='string'&&/^(0|[1-9][0-9]{0,17})\.[0-9]{2}$/.test(v)&&v!=='0.00'
const currency=(v:unknown):v is string=>typeof v==='string'&&/^[a-z]{3}$/.test(v)
export function paymentCheckoutURL(raw:unknown):string{
 if(typeof raw!=='string'||raw.length>2048||!raw.startsWith('https://')||raw.includes('\\'))throw fail()
 const u=new URL(raw)
 if(u.protocol!=='https:'||u.username||u.password||u.hash||u.port&&u.port!=='443'||u.hostname!=='mollie.com'&&!u.hostname.endsWith('.mollie.com')||/[\x00-\x20\x7f]/.test(raw))throw fail()
 return raw
}
export function paymentConsent(raw:unknown):PaymentConsent{
 const v=object(raw);keys(v,['expected_version','collection_id','amount','currency_code'],['previous_operation_id'])
 if(!version(v.expected_version)||!paymentOrderID(v.collection_id)||!money(v.amount)||!currency(v.currency_code)||'previous_operation_id'in v&&!paymentUUID(v.previous_operation_id))throw fail()
 return v as unknown as PaymentConsent
}
export function paymentReceipt(raw:unknown,order:string,id:string,view=false):PaymentView{
 const v=object(raw);keys(v,['operation_id','order_id','order_version','collection_id','amount','currency_code','state','applied','can_cancel','can_retry',...(view?['checkout_url']:[])],['previous_operation_id'])
 if(!paymentUUID(v.operation_id)||v.operation_id!==id||v.order_id!==order||!paymentOrderID(order)||!version(v.order_version)||!paymentOrderID(v.collection_id)||!money(v.amount)||!currency(v.currency_code)||!['pending','uncertain','open','authorized','paid','failed','canceled','expired'].includes(String(v.state))||typeof v.applied!=='boolean'||typeof v.can_cancel!=='boolean'||typeof v.can_retry!=='boolean'||'previous_operation_id'in v&&!paymentUUID(v.previous_operation_id))throw fail()
 const terminal=['failed','canceled','expired'].includes(String(v.state))
 if(v.applied&&v.state!=='paid'||v.can_retry!==terminal||v.can_cancel&&v.state!=='pending')throw fail()
 const link=view&&v.checkout_url!==null?paymentCheckoutURL(v.checkout_url):null
 if(link&&(v.applied||terminal||v.state==='paid'||v.state==='authorized'||v.can_cancel))throw fail()
 return {...v,checkout_url:link} as PaymentView
}
export function paymentContext(raw:unknown,order:string):PaymentContext{
 const v=object(raw);keys(v,['order_id','order_version','currency_code','pending_difference','next','payments'])
 if(v.order_id!==order||!paymentOrderID(order)||!version(v.order_version)||!currency(v.currency_code)||typeof v.pending_difference!=='string'||!/^(-?)(0|[1-9][0-9]{0,191})(\.[0-9]{1,128})?$/.test(v.pending_difference)||!Array.isArray(v.payments)||v.payments.length>1000)throw fail()
 const seen=new Set<string>(),payments=v.payments.map(raw=>{const row=object(raw);if(typeof row.operation_id!=='string')throw fail();const out=paymentReceipt(row,order,row.operation_id,true);if(seen.has(out.operation_id)||out.currency_code!==v.currency_code||out.order_version>Number(v.order_version))throw fail();seen.add(out.operation_id);return out})
 const next=v.next===null?null:paymentConsent(v.next)
 if(next&&(next.expected_version!==v.order_version||next.currency_code!==v.currency_code||!/^\d+\.\d{2}$/.test(next.amount)||!equalDecimal(next.amount,v.pending_difference)||payments.some(p=>!['failed','canceled','expired','paid'].includes(p.state)||p.state==='paid'&&!p.applied)))throw fail()
 if(next?.previous_operation_id&&!payments.some(p=>p.operation_id===next.previous_operation_id&&p.collection_id===next.collection_id&&p.can_retry))throw fail()
 return {...v,next,payments} as PaymentContext
}
function equalDecimal(a:string,b:string){const canonical=(s:string)=>{const [whole,decimal='']=s.split('.');return whole+'.'+decimal.replace(/0+$/,'')};return canonical(a)===canonical(b)}
export function paymentCommand(consent:PaymentConsent,id:string):PaymentInput {
 if(!paymentUUID(id))throw fail()
 const v=paymentConsent(consent)
 return {...v,expected_version:String(v.expected_version),operation_id:id}
}
