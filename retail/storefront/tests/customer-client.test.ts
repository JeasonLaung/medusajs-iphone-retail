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
