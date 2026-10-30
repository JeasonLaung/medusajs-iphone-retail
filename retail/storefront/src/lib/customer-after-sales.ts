export type AfterSalesType='return'|'exchange'|'repair'
export type AfterSalesInput={operation_id:string;type:AfterSalesType;reason:string;items:{item_id:string;quantity:number;device_ids?:string[]}[]}
export type AfterSalesRequest={id:string;type:AfterSalesType;status:'pending'|'processing'|'completed'|'rejected';reason:string;items:{item_id:string;quantity:number;title:string;device_ids?:string[]}[];created_at:string}
export type OrderDevice={id:string;assignment_id:string;item_id:string;serial_number:string|null;imei:string|null;status:'shipped'|'returned';replacement_of:string|null;tracking_number:string|null}
const obj=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw Error('Invalid after-sales response');return v as Record<string,unknown>}
const keys=(v:Record<string,unknown>,allowed:string[])=>{if(Object.keys(v).some(k=>!allowed.includes(k)))throw Error('Invalid after-sales response')}
const text=(v:unknown,max:number,multiline=false)=>{if(typeof v!=='string'||[...v].length>max||(multiline?/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/:/[\x00-\x1f\x7f]/).test(v))throw Error('Invalid after-sales response');return v}
const id=(v:unknown)=>{const s=text(v,191);if(!/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(s))throw Error('Invalid after-sales response');return s}
export function afterSalesRequestProjection(raw:unknown):AfterSalesRequest{
 const r=obj(raw);keys(r,['id','type','status','reason','items','created_at'])
 const key=id(r.id),reason=text(r.reason,2000,true),created=text(r.created_at,40)
 if(!['return','exchange','repair'].includes(String(r.type))||!['pending','processing','completed','rejected'].includes(String(r.status))||!Number.isFinite(Date.parse(created))||!Array.isArray(r.items)||r.items.length<1||r.items.length>100)throw Error('Invalid after-sales response')
 const seen=new Set<string>(),items=r.items.map(raw=>{const l=obj(raw);keys(l,['item_id','quantity','title','device_ids']);const key=id(l.item_id)
  if(seen.has(key)||!Number.isSafeInteger(l.quantity)||Number(l.quantity)<1||Number(l.quantity)>10000)throw Error('Invalid after-sales response');seen.add(key)
  const out:AfterSalesRequest['items'][number]={item_id:key,quantity:l.quantity as number,title:text(l.title,4096)}
  if('device_ids'in l){if(!Array.isArray(l.device_ids)||l.device_ids.length<1||l.device_ids.length>100)throw Error('Invalid after-sales response');out.device_ids=l.device_ids.map(id);if(new Set(out.device_ids).size!==out.device_ids.length)throw Error('Invalid after-sales response')}
  return out
 })
 return {id:key,type:r.type as AfterSalesType,status:r.status as AfterSalesRequest['status'],reason,items,created_at:created}
}
export function afterSalesListProjection(raw:unknown):AfterSalesRequest[]{
 const v=obj(raw);keys(v,['requests']);if(!Array.isArray(v.requests)||v.requests.length>1000)throw Error('Invalid after-sales response')
 const out=v.requests.map(afterSalesRequestProjection);if(new Set(out.map(r=>r.id)).size!==out.length)throw Error('Invalid after-sales response');return out
}
export function orderDevicesProjection(raw:unknown):OrderDevice[]{
 const v=obj(raw);keys(v,['devices']);if(!Array.isArray(v.devices)||v.devices.length>1000)throw Error('Invalid after-sales response')
 const seen=new Set<string>();return v.devices.map(raw=>{const r=obj(raw);keys(r,['id','assignment_id','item_id','serial_number','imei','status','replacement_of','tracking_number']);const assignment=id(r.assignment_id)
  if(seen.has(assignment)||!['shipped','returned'].includes(String(r.status)))throw Error('Invalid after-sales response');seen.add(assignment)
  return {id:id(r.id),assignment_id:assignment,item_id:id(r.item_id),serial_number:r.serial_number===null?null:text(r.serial_number,191),imei:r.imei===null?null:text(r.imei,15),status:r.status as OrderDevice['status'],replacement_of:r.replacement_of===null?null:id(r.replacement_of),tracking_number:r.tracking_number===null?null:text(r.tracking_number,200)}
 })
}
