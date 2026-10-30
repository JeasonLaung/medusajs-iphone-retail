export type CustomerNotice={id:string;kind:string;resource_id:string;reference:string;action:string;read_at:string|null;created_at:string}
export type NoticePage={notifications:CustomerNotice[];count:number;unread:number;offset:number;limit:number}
export const noticeID=(id:string)=>/^notice_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id)
const kinds=new Set(['shipment.created','delivery.created','order.canceled','after_sales.processing','after_sales.completed','after_sales.rejected','buyback.updated'])
export function noticePage(raw:unknown,offset:number):NoticePage{
 const fail=()=>{throw Error('Notification response unavailable')}
 if(!raw||typeof raw!=='object'||Array.isArray(raw))return fail()
 const p=raw as Record<string,unknown>
 if(!Array.isArray(p.notifications)||p.notifications.length>20||p.offset!==offset||p.limit!==20||!Number.isSafeInteger(p.count)||Number(p.count)<0||!Number.isSafeInteger(p.unread)||Number(p.unread)<0||Number(p.unread)>Number(p.count))return fail()
 const notifications=p.notifications.map((row:unknown):CustomerNotice=>{
  if(!row||typeof row!=='object'||Array.isArray(row))return fail()
  const n=row as Record<string,unknown>
  if(typeof n.id!=='string'||!noticeID(n.id)||typeof n.kind!=='string'||!kinds.has(n.kind)||typeof n.resource_id!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(n.resource_id)||typeof n.reference!=='string'||[...n.reference].length>191||/[\x00-\x1f\x7f]/.test(n.reference)||typeof n.action!=='string'||!/^[a-z_]{0,32}$/.test(n.action)||typeof n.created_at!=='string'||!Number.isFinite(Date.parse(n.created_at))||n.read_at!==null&&(typeof n.read_at!=='string'||!Number.isFinite(Date.parse(n.read_at))))return fail()
  return {id:n.id,kind:n.kind,resource_id:n.resource_id,reference:n.reference,action:n.action,created_at:n.created_at,read_at:n.read_at as string|null}
 })
 if(new Set(notifications.map(n=>n.id)).size!==notifications.length||notifications.length!==Math.min(20,Math.max(0,Number(p.count)-offset)))return fail()
 return {notifications,count:Number(p.count),unread:Number(p.unread),offset,limit:20}
}
