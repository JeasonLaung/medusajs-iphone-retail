export type Customer={id:string;email:string;first_name:string;last_name:string;phone:string;verified_at:string|null}
export type AuthState={state:'error'}|{state:'verification_required';email:string;delivery:'pending'|'failed'}|{state:'success'}|null
const root='/api/store/customer'
export class CustomerError extends Error{
 status:number;verification:boolean
 constructor(status:number,verification=false){super('Customer request unavailable');this.status=status;this.verification=verification}
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
 return {
  async current():Promise<Customer|null>{
   try{return (await call('/me')).customer}catch(error){if(error instanceof CustomerError&&error.status===401)return null;throw error}
  },
  async login(_previous:AuthState,form:FormData,segment:string):Promise<AuthState>{
   const input=identity(form,segment)
   try{await write('login',{email:input.email,password:input.password});pending=null;return {state:'success'}}catch(error){
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
   const matches=document.cookie.split(';').map(p=>p.trim().split('=')).filter(([name])=>name===(location.protocol==='https:'?'__Host-':'')+'retail_customer_csrf')
   if(matches.length!==1||!/^[0-9a-f]{64}$/.test(matches[0][1]??''))throw new CustomerError(403)
   await call('/auth/logout',{},matches[0][1]);pending=null
  },
  clear(){pending=null},
 }
}
