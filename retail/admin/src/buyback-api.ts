import {APIError,request,csrfCookie} from './api'
import {buybackRequestProjection,type BuybackRequest} from '../../storefront/src/lib/buyback-requests'
import {photoContentProjection} from '../../storefront/src/lib/buyback-photos'
export type {BuybackRequest}
export type ReviewCommand={action:'review_photo';photo_id:string;review_status:'approved'|'retake';note:string}|{action:'approve_shipping';shipping_instructions:string}|{action:'mark_received'|'mark_returned'}|{action:'cancel'|'request_return';note:string}|{action:'mark_paid';payment_reference:string}|{action:'mark_returning';return_tracking:string}
export type InspectionCommand={action:'link_device';item_id:string;device_id:string}|{action:'final_offer';items:{item_id:string;amount:number;notes:string}[]}
export async function buybacks(query:URLSearchParams,signal:AbortSignal){
 const d=await request<{requests:unknown[];count:number;limit:number;offset:number}>('buyback?'+query,{signal},'/api/admin/')
 if(!d||!Array.isArray(d.requests)||d.limit!==20||d.offset!==Number(query.get('offset')??0)||!Number.isSafeInteger(d.count)||d.count<0||d.requests.length!==Math.min(20,Math.max(0,d.count-d.offset)))throw new APIError(0,'request.invalid_response')
 return {...d,requests:d.requests.map(buybackRequestProjection)}
}
export async function buyback(id:string,command?:ReviewCommand|InspectionCommand,signal?:AbortSignal){
 const csrf=command?csrfCookie():undefined;if(command&&!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{request:unknown;enabled:boolean;historical:boolean;claim_enabled:boolean}>('buyback/'+encodeURIComponent(id),{method:command?'POST':'GET',body:command,csrf,signal},'/api/admin/')
 if(!d||typeof d.enabled!=='boolean'||typeof d.historical!=='boolean'||typeof d.claim_enabled!=='boolean')throw new APIError(0,'request.invalid_response')
 const r=buybackRequestProjection(d.request);if(r.id!==id)throw new APIError(0,'request.invalid_response');return {request:r,enabled:d.enabled,historical:d.historical,claim_enabled:d.claim_enabled}
}
export async function buybackPhoto(id:string,photo:string,signal:AbortSignal){return photoContentProjection(await request('buyback/'+encodeURIComponent(id)+'/photos/'+encodeURIComponent(photo),{signal},'/api/admin/'))}
export async function claimBuyback(id:string,body:{customer_id:string;evidence_note:string}){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 return request('buyback/'+encodeURIComponent(id)+'/claim',{method:'POST',body,csrf},'/api/admin/')
}
export async function inspectionDevices(id:string,item:string,signal:AbortSignal){
 const d=await request<{devices:unknown}>('buyback/'+encodeURIComponent(id)+'/items/'+encodeURIComponent(item)+'/devices',{signal},'/api/admin/')
 if(!d||!Array.isArray(d.devices)||d.devices.length>100||d.devices.some(v=>typeof v!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(v))||new Set(d.devices).size!==d.devices.length)throw new APIError(0,'request.invalid_response')
 return d.devices as string[]
}
