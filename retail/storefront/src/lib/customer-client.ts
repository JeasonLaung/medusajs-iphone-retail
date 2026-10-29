import {customerChanged} from './customer-events.ts'
export type Customer={id:string;email:string;first_name:string;last_name:string;phone:string;verified_at:string|null}
export type AuthState={state:'error'}|{state:'verification_required';email:string;delivery:'pending'|'failed'}|{state:'success'}|null
const root='/api/store/customer'
export class CustomerError extends Error{
 status:number;verification:boolean
 constructor(status:number,verification=false){super('Customer request unavailable');this.status=status;this.verification=verification}
}
function customerProjection(raw:unknown):Customer{
 if(!raw||typeof raw!=='object'||Array.isArray(raw))throw new CustomerError(503)
 const value=raw as Record<string,unknown>
 if(typeof value.id!=='string'||!/^cust_[A-Za-z0-9-]{1,80}$/.test(value.id)||typeof value.email!=='string'||value.email.length>254||!value.email.includes('@')||typeof value.verified_at!=='string'||!Number.isFinite(Date.parse(value.verified_at)))throw new CustomerError(503)
 for(const [key,max] of [['first_name',160],['last_name',160],['phone',64]] as const)if(typeof value[key]!=='string'||[...value[key] as string].length>max||/[\x00-\x1f\x7f]/.test(value[key] as string))throw new CustomerError(503)
 return {id:value.id,email:value.email,first_name:value.first_name as string,last_name:value.last_name as string,phone:value.phone as string,verified_at:value.verified_at}
}
export function customerClient(fetcher:typeof fetch=fetch){
 let pending:{email:string;password:string;country:string;locale:string}|null=null
 async function call(path:string,body?:Record<string,string>,csrf?:string):Promise<any>{
  const response=await fetcher(root+path,{method:body?'POST':'GET',credentials:'same-origin',cache:'no-store',redirect:'error',headers:body?{'Content-Type':'application/json',...(csrf?{'X-CSRF-Token':csrf}:{})}:undefined,body:body?JSON.stringify(body):undefined})
  const data=await response.json()
  if(!response.ok)throw new CustomerError(response.status,data?.state==='verification_required')
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
  clear(){pending=null},
 }
}
