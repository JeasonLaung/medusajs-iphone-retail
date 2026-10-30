import {APIError,request,csrfCookie} from './api'
import type {ParcelInput} from './api'
export type WarehouseParcel={id:string;order_id:string;reference:string;location_id:string;created_at:string;packed_at:string|null;shipped_at:string|null;delivered_at:string|null}
export type WarehouseParcels={location_id:string;fulfillments:WarehouseParcel[];offset:number;limit:50;has_more:boolean}
export type WarehouseParcelDetail=WarehouseParcel&{items:{id:string;item_id:string|null;title:string;sku:string;quantity:string}[];devices:{id:string;item_id:string;serial_number:string;status:string}[];labels:{id:string;tracking_number:string;tracking_url:string;label_url:string}[]}
const parcel=(p:WarehouseParcel,location:string)=>p&&p.location_id===location&&typeof p.id==='string'&&typeof p.order_id==='string'&&typeof p.reference==='string'&&['created_at','packed_at','shipped_at','delivered_at'].every(k=>{const v=p[k as keyof WarehouseParcel];return v===null&&k!=='created_at'||typeof v==='string'&&Number.isFinite(Date.parse(v))})
const path=(location:string)=>`warehouse/locations/${encodeURIComponent(location)}`
const shipmentPath=(location:string,order:string,id:string)=>`${path(location)}/orders/${encodeURIComponent(order)}/fulfillments/${encodeURIComponent(id)}/shipment`
export type WarehouseShipment={location_id:string;order_id:string;fulfillment_id:string;enabled:boolean;order_active:boolean;supported:boolean;status:string;revision:string}
export async function warehouseShipment(location:string,order:string,id:string,signal?:AbortSignal){
 const out=await request<WarehouseShipment>(shipmentPath(location,order,id),{signal},'/api/admin/')
 if(!out||out.location_id!==location||out.order_id!==order||out.fulfillment_id!==id||![out.enabled,out.order_active,out.supported].every(v=>typeof v==='boolean')||!['packed','unpacked','shipped','delivered'].includes(out.status)||typeof out.revision!=='string'||!/^[a-f0-9]{64}$/.test(out.revision))throw new APIError(0,'request.invalid_response')
 return out
}
export async function shipWarehouseParcel(location:string,order:string,id:string,body:ParcelInput){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const out=await request<{fulfillment_id:string;action:string;assignment_ids:string[];replayed:boolean}>(shipmentPath(location,order,id),{method:'POST',body,csrf},'/api/admin/')
 if(!out||out.fulfillment_id!==id||out.action!=='ship'||typeof out.replayed!=='boolean'||!Array.isArray(out.assignment_ids)||!out.assignment_ids.length||out.assignment_ids.some(v=>typeof v!=='string'))throw new APIError(0,'request.invalid_response')
}
export async function warehouseParcels(location:string,q:string,offset:number,signal?:AbortSignal){
 const out=await request<WarehouseParcels>(`${path(location)}/fulfillments?${new URLSearchParams({q,offset:String(offset)})}`,{signal},'/api/admin/')
 if(!out||out.location_id!==location||out.offset!==offset||out.limit!==50||typeof out.has_more!=='boolean'||!Array.isArray(out.fulfillments)||out.fulfillments.length>50||!out.fulfillments.every(p=>parcel(p,location)))throw new APIError(0,'request.invalid_response')
 return out
}
export async function warehouseParcel(location:string,order:string,id:string,signal?:AbortSignal){
 const out=await request<WarehouseParcelDetail>(`${path(location)}/orders/${encodeURIComponent(order)}/fulfillments/${encodeURIComponent(id)}`,{signal},'/api/admin/')
 if(!parcel(out,location)||out.id!==id||out.order_id!==order||!Array.isArray(out.items)||out.items.length>1000||!out.items.every(i=>i&&typeof i.id==='string'&&(i.item_id===null||typeof i.item_id==='string')&&typeof i.title==='string'&&typeof i.sku==='string'&&typeof i.quantity==='string'&&/^\d+(\.\d+)?$/.test(i.quantity))||!Array.isArray(out.devices)||out.devices.length>1000||!out.devices.every(d=>d&&typeof d.id==='string'&&typeof d.item_id==='string'&&typeof d.serial_number==='string'&&typeof d.status==='string')||!Array.isArray(out.labels)||out.labels.length>100||!out.labels.every(l=>l&&['id','tracking_number','tracking_url','label_url'].every(k=>typeof l[k as keyof typeof l]==='string')))throw new APIError(0,'request.invalid_response')
 return out
}
