import {orderConfirmationProjection,type OrderConfirmation} from './cart-client.ts'
import {orderListProjection,type CustomerOrderList} from './customer-orders.ts'
import {customerChanged} from './customer-events.ts'
import type {BuybackDraft} from './buyback-client.ts'
import {buybackID,photoProjection,photoContentProjection,type BuybackPhotoUpload} from './buyback-photos.ts'
import {noticePage,noticeID} from './customer-notices.ts'
import {customerProgressProjection,type CustomerProgress} from './customer-progress.ts'
import {afterSalesListProjection,afterSalesRequestProjection,orderDevicesProjection,type AfterSalesInput} from './customer-after-sales.ts'
export type Customer={id:string;email:string;first_name:string;last_name:string;phone:string;verified_at:string|null}
export type AddressFields={first_name?:string;last_name?:string;company?:string;address_1?:string;address_2?:string;city?:string;postal_code?:string;province?:string;country_code?:string;phone?:string;is_default_billing?:boolean;is_default_shipping?:boolean}
export type Address=Required<AddressFields>&{id:string;created_at:string;updated_at:string}
export type AuthState={state:'error'}|{state:'verification_required';email:string;delivery:'pending'|'failed'}|{state:'success'}|null
const root='/api/store/customer'
export class CustomerError extends Error{
 status:number;verification:boolean;code?:string
 constructor(status:number,verification=false,code?:string){super('Customer request unavailable');this.status=status;this.verification=verification;this.code=code}
}
function customerProjection(raw:unknown):Customer{
 if(!raw||typeof raw!=='object'||Array.isArray(raw))throw new CustomerError(503)
 const value=raw as Record<string,unknown>
 if(typeof value.id!=='string'||!/^cust_[A-Za-z0-9-]{1,80}$/.test(value.id)||typeof value.email!=='string'||value.email.length>254||!value.email.includes('@')||typeof value.verified_at!=='string'||!Number.isFinite(Date.parse(value.verified_at)))throw new CustomerError(503)
 for(const [key,max] of [['first_name',160],['last_name',160],['phone',64]] as const)if(typeof value[key]!=='string'||[...value[key] as string].length>max||/[\x00-\x1f\x7f]/.test(value[key] as string))throw new CustomerError(503)
 return {id:value.id,email:value.email,first_name:value.first_name as string,last_name:value.last_name as string,phone:value.phone as string,verified_at:value.verified_at}
}
const addressID=(id:string)=>/^addr_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(id)
function addressProjection(raw:unknown):Address{
 if(!raw||typeof raw!=='object'||Array.isArray(raw))throw new CustomerError(503)
 const value=raw as Record<string,unknown>,out:Record<string,unknown>={}
 if(typeof value.id!=='string'||!addressID(value.id))throw new CustomerError(503)
 out.id=value.id
 for(const [name,max] of Object.entries({first_name:160,last_name:160,company:255,address_1:255,address_2:255,city:160,postal_code:32,province:160,country_code:2,phone:64})){
  const text=value[name]
  if(typeof text!=='string'||[...text].length>max||/[\x00-\x1f\x7f]/.test(text))throw new CustomerError(503)
  out[name]=text
 }
 if(!/^[a-z]{2}$/.test(out.country_code as string))throw new CustomerError(503)
 for(const key of ['is_default_billing','is_default_shipping']){if(typeof value[key]!=='boolean')throw new CustomerError(503);out[key]=value[key]}
 for(const key of ['created_at','updated_at']){if(typeof value[key]!=='string'||!Number.isFinite(Date.parse(value[key] as string)))throw new CustomerError(503);out[key]=value[key]}
 return out as Address
}
function addressInput(fields:AddressFields){
 const out:Record<string,string>={}
 for(const [key,value] of Object.entries(fields)){
  if(['is_default_billing','is_default_shipping'].includes(key)){
   if(typeof value!=='boolean')throw new CustomerError(400);out[key]=String(value)
  }else{
   if(!['first_name','last_name','company','address_1','address_2','city','postal_code','province','country_code','phone'].includes(key)||typeof value!=='string')throw new CustomerError(400)
   out[key]=value
  }
 }
 if(!Object.keys(out).length)throw new CustomerError(400)
 return out
}
function photoError(e:unknown):string{
 if(e instanceof CustomerError){
  if(e.status===401)return 'BUYBACK_LOGIN_REQUIRED'
  if(e.code&&({BUYBACK_INVALID_PHOTO:400,BUYBACK_PHOTO_NOT_FOUND:404,BUYBACK_INVALID_STATE:409,BUYBACK_TOO_MANY_PHOTOS:409} as Record<string,number>)[e.code]===e.status)return e.code
 }
 return 'BUYBACK_REQUEST_FAILED'
}
export function customerClient(fetcher:typeof fetch=fetch){
 let pending:{email:string;password:string;country:string;locale:string}|null=null
 async function call(path:string,body?:Record<string,unknown>,csrf?:string,expectedStatus?:number):Promise<any>{
  const response=await fetcher(root+path,{method:body?'POST':'GET',credentials:'same-origin',cache:'no-store',redirect:'error',headers:body?{'Content-Type':'application/json',...(csrf?{'X-CSRF-Token':csrf}:{})}:undefined,body:body?JSON.stringify(body):undefined})
  const data=await response.json()
  if(!response.ok)throw new CustomerError(response.status,data?.state==='verification_required',typeof data?.error?.code==='string'?data.error.code:undefined)
  if(expectedStatus!==undefined&&response.status!==expectedStatus)throw new CustomerError(503)
  return data
 }
 async function write(path:string,body:Record<string,string>){
  const challenge=await call('/auth/challenge',{})
  if(!/^[0-9a-f]{64}$/.test(challenge.csrf_token??''))throw new CustomerError(503)
  return call('/auth/'+path,body,challenge.csrf_token)
 }
 function identity(form:FormData,segment:string){return {email:String(form.get('email')??''),password:String(form.get('password')??''),country:segment.slice(0,2),locale:segment.endsWith('-zh')?'zh':'en'}}
 function sessionCSRF(){
  const matches=document.cookie.split(';').map(p=>p.trim().split('=')).filter(([name])=>name===(location.protocol==='https:'?'__Host-':'')+'retail_customer_csrf')
  if(matches.length!==1||!/^[0-9a-f]{64}$/.test(matches[0][1]??''))throw new CustomerError(403)
  return matches[0][1]
 }
 return {
  async uploadBuybackPhoto(id:string,input:BuybackPhotoUpload){
   try{
    if(!buybackID(id))throw new CustomerError(400)
    const raw=await call('/buyback/requests/'+id+'/photos',input as unknown as Record<string,unknown>,sessionCSRF(),201)
    if(!raw||Object.keys(raw).length!==1)throw new CustomerError(503)
    const photo=photoProjection(raw.photo,id)
    if(photo.item_id!==input.item_id||photo.photo_type!==input.photo_type||photo.mime_type!==input.mime_type||photo.review_status!=='pending')throw new CustomerError(503)
    return {data:{photo},error:null}
   }catch(e){return {data:null,error:photoError(e)}}
  },
  async buybackPhoto(id:string,photoID:string){
   try{
    if(!buybackID(id)||!buybackID(photoID))throw new CustomerError(400)
    return {data:photoContentProjection(await call('/buyback/requests/'+id+'/photos/'+photoID,undefined,undefined,200)),error:null}
   }catch(e){return {data:null,error:photoError(e)}}
  },
  async createBuyback(input:BuybackDraft):Promise<{data:{request:{id:string}}|null;error:string|null}>{
   try{
    const result=await call('/buyback/requests',input as unknown as Record<string,unknown>,sessionCSRF(),201)
    if(!result||Object.keys(result).length!==1||!result.request||typeof result.request.id!=='string'||!/^bbr_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(result.request.id))throw new CustomerError(503)
    return {data:{request:{id:result.request.id}},error:null}
   }catch(e){return {data:null,error:e instanceof CustomerError&&e.status===401?'BUYBACK_LOGIN_REQUIRED':'BUYBACK_REQUEST_FAILED'}}
  },
  async notifications(offset=0){
   if(!Number.isSafeInteger(offset)||offset<0||offset>100000)throw new CustomerError(400)
   return noticePage(await call('/notifications?offset='+offset),offset)
  },
  async readNotifications(id?:string){
   if(id!==undefined&&!noticeID(id))throw new CustomerError(400)
   const out=await call('/notifications',id===undefined?{}:{id},sessionCSRF())
   if(out.success!==true||Object.keys(out).length!==1)throw new CustomerError(503)
  },
  async orders(offset=0):Promise<CustomerOrderList>{
   if(!Number.isSafeInteger(offset)||offset<0||offset>100000)throw new CustomerError(400)
   return orderListProjection(await call('/orders?offset='+offset),offset)
  },
  async order(id:string):Promise<OrderConfirmation>{
   if(!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(id))throw new CustomerError(400)
   return orderConfirmationProjection(await call('/orders/'+id),id)
  },
  async orderProgress(id:string):Promise<CustomerProgress>{
   if(!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(id))throw new CustomerError(400)
   return customerProgressProjection(await call('/orders/'+id+'/progress'),id)
  },
  async afterSales(id:string){
   if(!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(id))throw new CustomerError(400)
   return afterSalesListProjection(await call('/orders/'+id+'/after-sales'))
  },
  async orderDevices(id:string){
   if(!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(id))throw new CustomerError(400)
   return orderDevicesProjection(await call('/orders/'+id+'/devices'))
  },
  async createAfterSales(id:string,input:AfterSalesInput){
   if(!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(id))throw new CustomerError(400)
   const raw=await call('/orders/'+id+'/after-sales',input,sessionCSRF())
   if(!raw||Object.keys(raw).length!==1||!raw.request)throw new CustomerError(503)
   return afterSalesRequestProjection(raw.request)
  },
  async attachOrder(id:string):Promise<void>{
   if(!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(id))throw new CustomerError(400)
   const result=await call('/orders/'+id+'/attach',{},sessionCSRF())
   if(!result||Object.keys(result).length!==1||result.order_id!==id)throw new CustomerError(503)
  },
  async current():Promise<Customer|null>{
   try{return customerProjection((await call('/me')).customer)}catch(error){if(error instanceof CustomerError&&error.status===401)return null;throw error}
  },
  async login(_previous:AuthState,form:FormData,segment:string):Promise<AuthState>{
   const input=identity(form,segment)
   try{await write('login',{email:input.email,password:input.password});pending=null;customerChanged();return {state:'success'}}catch(error){
    if(error instanceof CustomerError&&error.verification){pending=input;return {state:'verification_required',email:input.email,delivery:'pending'}}
    pending=null;return {state:'error'}
   }
  },
  async signup(_previous:AuthState,form:FormData,segment:string):Promise<AuthState>{
   const input=identity(form,segment)
   try{
    const result=await write('register',{...input,first_name:String(form.get('first_name')??''),last_name:String(form.get('last_name')??''),phone:String(form.get('phone')??'')})
    if(result.state!=='verification_required'||result.delivery!=='pending')throw new CustomerError(503)
    pending=input;return {state:'verification_required',email:result.email,delivery:'pending'}
   }catch{pending=null;return {state:'error'}}
  },
  async resend(){if(!pending)return {success:false};try{await write('resend',pending);return {success:true}}catch{return {success:false}}},
  async verify(token:string){if(!/^[0-9a-f]{64}$/.test(token))return {success:false};try{await write('verify',{token});return {success:true}}catch{return {success:false}}},
  async logout(){
   await call('/auth/logout',{},sessionCSRF());pending=null;customerChanged()
  },
  async updateProfile(fields:{first_name?:string;last_name?:string;phone?:string}){
   const input:Record<string,string>={}
   for(const [name,value] of Object.entries(fields)){
    if(!['first_name','last_name','phone'].includes(name)||typeof value!=='string')throw new CustomerError(400)
    input[name]=value
   }
   if(!Object.keys(input).length)throw new CustomerError(400)
   const result=customerProjection((await call('/me',input,sessionCSRF())).customer)
   customerChanged();return result
  },
  async addresses():Promise<Address[]>{
   const result=await call('/addresses')
   if(!Array.isArray(result.addresses))throw new CustomerError(503)
   const addresses=result.addresses.map(addressProjection),ids=new Set(addresses.map((address:Address)=>address.id))
   if(ids.size!==addresses.length||addresses.filter((a:Address)=>a.is_default_billing).length>1||addresses.filter((a:Address)=>a.is_default_shipping).length>1)throw new CustomerError(503)
   return addresses
  },
  async saveAddress(id:string|null,fields:AddressFields):Promise<Address>{
   if(id!==null&&!addressID(id))throw new CustomerError(400)
   const input=addressInput(fields),result=addressProjection((await call('/addresses'+(id?'/'+id:''),input,sessionCSRF())).address)
   customerChanged();return result
  },
  async deleteAddress(id:string){
   if(!addressID(id))throw new CustomerError(400)
   const result=await call('/addresses/'+id+'/delete',{},sessionCSRF())
   if(result.success!==true)throw new CustomerError(503)
   customerChanged()
  },
  clear(){pending=null},
 }
}
