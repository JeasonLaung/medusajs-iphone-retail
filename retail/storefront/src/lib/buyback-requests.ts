import type {BuybackAssessment} from './buyback-client.ts'
import {buybackID,photoProjection,type BuybackPhoto} from './buyback-photos.ts'
export type BuybackCommand={action:'submit_photos';items:{item_id:string;serial_number:string;imei:string}[]}|{action:'mark_shipped';tracking_number:string}|{action:'accept_offer';final_amount:number}|{action:'decline_offer'|'cancel'}
export type BuybackItem={id:string;product_id:string;model:string;model_name:string;capacity:string;assessment:BuybackAssessment;reference_amount:number|null;final_amount:number|null;inspection_notes:string;serial_number:string;imei:string;device_id:string|null}
export type BuybackSummary={recycled:number;processing:number;received:{currency_code:string;amount:number}[]}
export type BuybackRequest={id:string;status:string;contact_name:string;email:string;phone:string;return_address:{name:string;address_1:string;address_2:string;city:string;postal_code:string;country_code:string};items:BuybackItem[];photos:BuybackPhoto[];reference_amount:number|null;final_amount:number|null;currency_code:string;shipping_instructions:string;tracking_number:string;return_tracking:string;payment_reference:string;review_note:string;created_at:string;updated_at:string;closed_at:string|null;deleted_at:null;retention_days:number;history:{at:string;action:string;actor?:'customer'|'platform';status:string;note?:string;photo_id?:string}[]}
export type BuybackList={requests:BuybackRequest[];summary:BuybackSummary;count:number;offset:number;limit:20}
const statuses=['awaiting_photos','photo_review','ready_to_ship','in_transit','received','final_offer','accepted','paid','return_requested','returning','returned','canceled']
const fail=()=>new Error('Buyback request unavailable')
const object=(v:any)=>{if(!v||typeof v!=='object'||Array.isArray(v))throw fail();return v}
const text=(v:any,max=16384)=>{if(typeof v!=='string'||v.length>max)throw fail();return v}
const amount=(v:any)=>{if(v!==null&&(typeof v!=='number'||!Number.isFinite(v)||v<0))throw fail();return v as number|null}
const date=(v:any)=>{if(typeof v!=='string'||!Number.isFinite(Date.parse(v)))throw fail();return v}
const id=(v:any)=>{if(typeof v!=='string'||!buybackID(v))throw fail();return v}
const status=(v:any)=>{if(!statuses.includes(v))throw fail();return v as string}
function assessment(raw:any):BuybackAssessment{
 const r=object(raw)
 if(!Array.isArray(r.checks)||r.checks.length!==5||!r.checks.every((v:any)=>typeof v==='boolean'||v===null))throw fail()
 for(const k of ['faults','glass'])if(!Array.isArray(r[k])||r[k].length>3||!r[k].every((v:any)=>typeof v==='string'&&v.length<32))throw fail()
 return {battery:text(r.battery,7),checks:[...r.checks],faults:[...r.faults],glass:[...r.glass],screen:text(r.screen,10),frame:text(r.frame,10),back:text(r.back,10)}
}
export function buybackRequestProjection(raw:unknown):BuybackRequest{
 const r=object(raw),requestID=id(r.id),a=object(r.return_address)
 const fields=['id','status','contact_name','email','phone','return_address','items','photos','reference_amount','final_amount','currency_code','shipping_instructions','tracking_number','return_tracking','payment_reference','review_note','created_at','updated_at','closed_at','deleted_at','retention_days','history']
 if(Object.keys(r).length!==fields.length||!fields.every(k=>Object.hasOwn(r,k))||r.deleted_at!==null||r.retention_days!==180||!Array.isArray(r.items)||r.items.length<1||r.items.length>10||!Array.isArray(r.photos)||r.photos.length>300||!Array.isArray(r.history)||r.history.length>10000||!/^[a-z]{3}$/.test(r.currency_code))throw fail()
 const items:BuybackItem[]=r.items.map((value:any)=>{const v=object(value);return {id:id(v.id),product_id:id(v.product_id),model:text(v.model,150),model_name:text(v.model_name,255),capacity:text(v.capacity,100),assessment:assessment(v.assessment),reference_amount:amount(v.reference_amount),final_amount:amount(v.final_amount),inspection_notes:text(v.inspection_notes),serial_number:text(v.serial_number,100),imei:text(v.imei,100),device_id:v.device_id===null?null:id(v.device_id)}})
 if(new Set(items.map(v=>v.id)).size!==items.length)throw fail()
 const photos:BuybackPhoto[]=r.photos.map((p:any)=>photoProjection(p,requestID))
 if(new Set(photos.map(v=>v.id)).size!==photos.length||photos.some(p=>!items.some(v=>v.id===p.item_id)))throw fail()
 const history=r.history.map((raw:any)=>{const v=object(raw);if(v.actor!==undefined&&!['customer','platform'].includes(v.actor))throw fail();return {at:date(v.at),action:text(v.action,100),status:status(v.status),...(v.actor!==undefined?{actor:v.actor}:{}),...(v.note!==undefined?{note:text(v.note)}:{}),...(v.photo_id!==undefined?{photo_id:id(v.photo_id)}:{})}})
 return {id:requestID,status:status(r.status),contact_name:text(r.contact_name,100),email:text(r.email,254),phone:text(r.phone,50),return_address:{name:text(a.name,100),address_1:text(a.address_1,200),address_2:text(a.address_2,200),city:text(a.city,100),postal_code:text(a.postal_code,30),country_code:text(a.country_code,2)},items,photos,reference_amount:amount(r.reference_amount),final_amount:amount(r.final_amount),currency_code:r.currency_code,shipping_instructions:text(r.shipping_instructions),tracking_number:text(r.tracking_number),return_tracking:text(r.return_tracking),payment_reference:text(r.payment_reference),review_note:text(r.review_note),created_at:date(r.created_at),updated_at:date(r.updated_at),closed_at:r.closed_at===null?null:date(r.closed_at),deleted_at:null,retention_days:180,history}
}
export function buybackListProjection(raw:unknown,offset:number):BuybackList{
 const r=object(raw),s=object(r.summary)
 if(Object.keys(r).length!==5||!['requests','summary','count','offset','limit'].every(k=>Object.hasOwn(r,k))||!Array.isArray(r.requests)||r.offset!==offset||r.limit!==20||!Number.isSafeInteger(r.count)||r.count<0||r.requests.length!==Math.min(20,Math.max(0,r.count-offset))||!Number.isSafeInteger(s.recycled)||s.recycled<0||!Number.isSafeInteger(s.processing)||s.processing<0||!Array.isArray(s.received)||s.received.length>100)throw fail()
 const requests=r.requests.map(buybackRequestProjection)
 if(new Set(requests.map((v:BuybackRequest)=>v.id)).size!==requests.length)throw fail()
 const received=s.received.map((v:any)=>{object(v);if(typeof v.currency_code!=='string'||!/^[A-Z]{3}$/.test(v.currency_code)||amount(v.amount)===null)throw fail();return {currency_code:v.currency_code,amount:v.amount as number}})
 if(new Set(received.map((v:any)=>v.currency_code)).size!==received.length)throw fail()
 return {requests,count:r.count,offset,limit:20,summary:{recycled:s.recycled,processing:s.processing,received}}
}
