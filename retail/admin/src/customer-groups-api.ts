import {APIError,request,csrfCookie} from './api'
export type CustomerGroup={id:string;name:string;members:number}
export type GroupDirectory={revision:string;enabled:boolean;groups:CustomerGroup[]}
export type GroupAccount={id:string;email:string}
export type GroupCommand={operation_id:string;expected_revision:string;kind:'create'|'rename'|'delete'|'add_members'|'remove_members';group_id:string;name:string;customer_ids:string[]}
export async function readCustomerGroups(signal?:AbortSignal){
 const d=await request<GroupDirectory>('customer-groups',{signal},'/api/admin/')
 if(!d||!/^\d+$/.test(d.revision)||typeof d.enabled!=='boolean'||!Array.isArray(d.groups)||d.groups.length>1000||!d.groups.every(g=>g&&typeof g.id==='string'&&typeof g.name==='string'&&Number.isSafeInteger(g.members)&&g.members>=0))throw new APIError(0,'request.invalid_response');return d
}
export async function readGroupAccounts(q:string,group:string,offset:number,signal?:AbortSignal){
 const params=new URLSearchParams({q,group_id:group,offset:String(offset)}),d=await request<{accounts:GroupAccount[];count:number}>('customer-group-accounts?'+params,{signal},'/api/admin/')
 if(!d||!Array.isArray(d.accounts)||d.accounts.length>20||!d.accounts.every(a=>a&&typeof a.id==='string'&&typeof a.email==='string')||!Number.isSafeInteger(d.count)||d.count<0)throw new APIError(0,'request.invalid_response');return d
}
export async function changeCustomerGroup(body:GroupCommand){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{group_id:string;revision:string;replayed:boolean}>('customer-groups',{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.group_id!=='string'||!/^\d+$/.test(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response');return d
}
