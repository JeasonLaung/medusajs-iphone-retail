import test from 'node:test'
import assert from 'node:assert/strict'
import {customerClient,CustomerError} from '../src/lib/customer-client.ts'
const csrf='b'.repeat(64)
function form(){const data=new FormData();for(const [key,value] of Object.entries({email:'new@example.invalid',password:'temporary-password',first_name:'New',last_name:'Customer'}))data.set(key,value);return data}
test('customer registration uses a fresh challenge and keeps resend credentials only in closure memory',async()=>{
 const calls:{path:string;body:any}[]=[]
 const api=customerClient(async(input,init)=>{
  const path=String(input),body=JSON.parse(String(init?.body));calls.push({path,body})
  assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error')
  if(path.endsWith('challenge'))return Response.json({csrf_token:csrf})
  assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf)
  return Response.json({state:'verification_required',delivery:'pending',email:'new@example.invalid'},{status:202})
 })
 assert.deepEqual(await api.resend(),{success:false})
 const state=await api.signup(null,form(),'dk-zh')
 assert.deepEqual(state,{state:'verification_required',delivery:'pending',email:'new@example.invalid'})
 assert.ok(!JSON.stringify(state).includes('temporary-password'))
 assert.deepEqual(await api.resend(),{success:true});assert.equal(calls.length,4)
 assert.equal(calls[1].body.country,'dk');assert.equal(calls[1].body.locale,'zh')
 api.clear();assert.deepEqual(await api.resend(),{success:false});assert.equal(calls.length,4)
})
test('customer observation distinguishes unauthorized from dependency failure and never retries login',async()=>{
 let calls=0
 const api=customerClient(async(input)=>{
  calls++;if(String(input).endsWith('challenge'))return Response.json({csrf_token:csrf})
  return Response.json({state:'verification_required'},{status:403})
 })
 assert.deepEqual(await api.login(null,form(),'dk-en'),{state:'verification_required',email:'new@example.invalid',delivery:'pending'});assert.equal(calls,2)
 const denied=customerClient(async()=>Response.json({},{status:401}));assert.equal(await denied.current(),null)
 const failed=customerClient(async()=>Response.json({},{status:503}));await assert.rejects(failed.current(),CustomerError)
 let uncertainCalls=0
 const uncertain=customerClient(async(input)=>{uncertainCalls++;if(String(input).endsWith('challenge'))return Response.json({csrf_token:csrf});throw new Error('lost response')})
 assert.deepEqual(await uncertain.login(null,form(),'dk-en'),{state:'error'});assert.equal(uncertainCalls,2)
})
test('verification checks capability shape before transport and does not create an authenticated observation',async()=>{
 let calls=0
 const api=customerClient(async(input)=>{calls++;return Response.json(String(input).endsWith('challenge')?{csrf_token:csrf}:{success:true})})
 assert.deepEqual(await api.verify('bad'),{success:false});assert.equal(calls,0)
 assert.deepEqual(await api.verify('a'.repeat(64)),{success:true});assert.equal(calls,2)
})
test('customer observation rejects malformed or unverified success and projects only public fields',async()=>{
 const customer={id:'cust_fixture',email:'new@example.invalid',first_name:'New',last_name:'Customer',phone:'',verified_at:'2026-10-29T00:00:00Z'}
 const api=customerClient(async()=>Response.json({customer:{...customer,admin:true,password_hash:'PRIVATE'}}))
 assert.deepEqual(await api.current(),customer)
 for(const body of [{},{customer:null},{customer:{...customer,verified_at:null}},{customer:{...customer,id:'old-customer-id'}},{customer:{...customer,phone:{unexpected:true}}}]){
  await assert.rejects(customerClient(async()=>Response.json(body)).current(),CustomerError)
 }
})
test('profile update sends only explicit supported fields with current session CSRF and never prelogin',async()=>{
 const previousDocument=Object.getOwnPropertyDescriptor(globalThis,'document'),previousLocation=Object.getOwnPropertyDescriptor(globalThis,'location')
 Object.defineProperty(globalThis,'document',{value:{cookie:`retail_customer_csrf=${csrf}`},configurable:true})
 Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 const customer={id:'cust_fixture',email:'new@example.invalid',first_name:'Changed',last_name:'Customer',phone:'',verified_at:'2026-10-29T00:00:00Z'}
 let calls=0
 try{
  const api=customerClient(async(input,init)=>{
   calls++;assert.equal(String(input),'/api/store/customer/me');assert.equal(init?.method,'POST')
   assert.deepEqual(JSON.parse(String(init?.body)),{first_name:'Changed',phone:''});assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf)
   return Response.json({customer})
  })
  assert.deepEqual(await api.updateProfile({first_name:'Changed',phone:''}),customer)
  await assert.rejects(api.updateProfile({}),CustomerError)
  await assert.rejects(api.updateProfile({email:'forbidden'} as any),CustomerError)
  assert.equal(calls,1)
 }finally{
  if(previousDocument)Object.defineProperty(globalThis,'document',previousDocument);else Reflect.deleteProperty(globalThis,'document')
  if(previousLocation)Object.defineProperty(globalThis,'location',previousLocation);else Reflect.deleteProperty(globalThis,'location')
 }
})

const address={id:'addr_00000000-0000-4000-8000-000000000001',first_name:'New',last_name:'Customer',company:'',address_1:'Street 1',address_2:'',city:'Copenhagen',postal_code:'0012',province:'',country_code:'dk',phone:'',is_default_billing:false,is_default_shipping:false,created_at:'2026-10-29T00:00:00Z',updated_at:'2026-10-29T00:00:00Z'}
test('address writes bind current CSRF and fixed paths, preserving explicit false and empty fields',async()=>{
 const beforeDocument=Object.getOwnPropertyDescriptor(globalThis,'document'),beforeLocation=Object.getOwnPropertyDescriptor(globalThis,'location')
 Object.defineProperty(globalThis,'document',{value:{cookie:`retail_customer_csrf=${csrf}`},configurable:true})
 Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 const calls:{path:string;body:any}[]=[]
 try{
  const api=customerClient(async(input,init)=>{
   calls.push({path:String(input),body:JSON.parse(String(init?.body))});assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf)
   assert.equal(init?.method,'POST');assert.equal(init?.cache,'no-store')
   return Response.json(String(input).endsWith('/delete')?{success:true}:{address})
  })
  assert.deepEqual(await api.saveAddress(null,{first_name:'New',phone:'',is_default_billing:false}),address)
  assert.deepEqual(calls[0],{path:'/api/store/customer/addresses',body:{first_name:'New',phone:'',is_default_billing:'false'}})
  await api.saveAddress(address.id,{is_default_billing:true})
  await api.deleteAddress(address.id)
  assert.equal(calls[1].path,'/api/store/customer/addresses/'+address.id)
  assert.equal(calls[2].path,'/api/store/customer/addresses/'+address.id+'/delete')
  for(const id of ['other-customer-address','../auth/logout',address.id+'?secret=x'])await assert.rejects(api.deleteAddress(id),CustomerError)
  await assert.rejects(api.saveAddress(null,{customer_id:'forbidden'} as any),CustomerError)
  await assert.rejects(api.saveAddress(null,{is_default_billing:'true'} as any),CustomerError)
  assert.equal(calls.length,3)
  const failed=customerClient(async()=>{calls.push({path:'failure',body:{}});throw new Error('Lost response')})
  await assert.rejects(failed.deleteAddress(address.id));assert.equal(calls.length,4)
 }finally{
  if(beforeDocument)Object.defineProperty(globalThis,'document',beforeDocument);else Reflect.deleteProperty(globalThis,'document')
  if(beforeLocation)Object.defineProperty(globalThis,'location',beforeLocation);else Reflect.deleteProperty(globalThis,'location')
 }
})
test('address observation projects public fields and rejects invalid lists or conflicting defaults',async()=>{
 const api=customerClient(async()=>Response.json({addresses:[{...address,customer_id:'PRIVATE',metadata:{private:true}}]}))
 assert.deepEqual(await api.addresses(),[address])
 for(const addresses of [null,{},[null],[{...address,phone:null}],[{...address,id:'historical'}],[address,address],[{...address,is_default_billing:true},{...address,id:'addr_00000000-0000-4000-8000-000000000002',is_default_billing:true}]]){
  await assert.rejects(customerClient(async()=>Response.json({addresses})).addresses(),CustomerError)
 }
 assert.deepEqual(await customerClient(async()=>Response.json({addresses:[]})).addresses(),[])
 await assert.rejects(customerClient(async()=>Response.json({},{status:401})).addresses(),CustomerError)
})
