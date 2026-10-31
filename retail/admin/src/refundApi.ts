import {APIError,csrfCookie,request} from './api'
export type RefundInput={operation_id:string;payment_id:string;amount:string;previous_operation_id?:string}
export type RefundOperation={operation_id:string;order_id:string;request_id:string;payment_id:string;amount:string;currency_code:string;state:string;applied:boolean;can_cancel:boolean;can_retry:boolean;previous_operation_id?:string}
export type RefundContext={order_id:string;request_id:string;enabled:boolean;request_type:string;exchange_credit?:string;operations:RefundOperation[]}
const path=(o:string,r:string)=>`orders/${encodeURIComponent(o)}/after-sales/${encodeURIComponent(r)}/refunds`
function operation(d:RefundOperation,o:string,r:string):RefundOperation{
 const text=(v:unknown)=>typeof v==='string'&&v.length>0&&v.length<=191
 if(!d||d.order_id!==o||d.request_id!==r||![d.operation_id,d.payment_id,d.currency_code].every(text)||!/^(?:0|[1-9]\d{0,17})(?:\.\d{1,2})?$/.test(d.amount)||!['pending','uncertain','queued','processing','refunded','failed','canceled'].includes(d.state)||typeof d.applied!=='boolean'||typeof d.can_cancel!=='boolean'||typeof d.can_retry!=='boolean'||d.previous_operation_id!==undefined&&!text(d.previous_operation_id))throw new APIError(0,'request.invalid_response')
 return {operation_id:d.operation_id,order_id:o,request_id:r,payment_id:d.payment_id,amount:d.amount,currency_code:d.currency_code,state:d.state,applied:d.applied,can_cancel:d.can_cancel,can_retry:d.can_retry,...(d.previous_operation_id?{previous_operation_id:d.previous_operation_id}:{})}
}
export async function readRefunds(o:string,r:string,signal?:AbortSignal):Promise<RefundContext>{
 const d=await request<RefundContext>(path(o,r),{signal},'/api/admin/')
 if(!d||d.order_id!==o||d.request_id!==r||typeof d.enabled!=='boolean'||!Array.isArray(d.operations)||d.operations.length>100)throw new APIError(0,'request.invalid_response')
 if(!['return','exchange','repair'].includes(d.request_type)||d.request_type==='exchange'&&(typeof d.exchange_credit!=='string'||!/^(?:0|[1-9]\d{0,17})\.\d{2}$/.test(d.exchange_credit))||d.request_type!=='exchange'&&d.exchange_credit!==undefined)throw new APIError(0,'request.invalid_response')
 const rows=d.operations.map(v=>operation(v,o,r));if(new Set(rows.map(v=>v.operation_id)).size!==rows.length)throw new APIError(0,'request.invalid_response')
 return {order_id:o,request_id:r,enabled:d.enabled,request_type:d.request_type,...(d.exchange_credit!==undefined?{exchange_credit:d.exchange_credit}:{}),operations:rows}
}
export async function prepareRefund(o:string,r:string,body:RefundInput):Promise<RefundOperation>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=operation(await request<RefundOperation>(path(o,r),{method:'POST',body,csrf},'/api/admin/'),o,r)
 if(d.operation_id!==body.operation_id||d.payment_id!==body.payment_id||d.amount!==body.amount||(d.previous_operation_id??'')!==(body.previous_operation_id??''))throw new APIError(0,'request.invalid_response')
 return d
}
export async function cancelRefund(o:string,r:string,original:string,operationID:string):Promise<RefundOperation>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=operation(await request<RefundOperation>(path(o,r)+'/'+encodeURIComponent(original)+'/cancel',{method:'POST',body:{operation_id:operationID},csrf},'/api/admin/'),o,r)
 if(d.operation_id!==original||d.state!=='canceled')throw new APIError(0,'request.invalid_response')
 return d
}
