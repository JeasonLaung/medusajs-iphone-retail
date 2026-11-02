import test from 'node:test'
import assert from 'node:assert/strict'
import {cartCustomerBinding} from '../src/lib/cart-customer-binding.ts'

test('binding uses fixed routes, session CSRF and exact immutable command only',async()=>{
 const command={operation_id:'11111111-1111-4111-8111-111111111111',version:'1'},calls:string[]=[]
 const binding=cartCustomerBinding(async(url,init)=>{calls.push(String(url));assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error');if(String(url).endsWith('/me'))return Response.json({customer:{id:'new_account'}});assert.deepEqual(JSON.parse(String(init?.body)),command);assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),'a'.repeat(64));return Response.json({cart_id:'cart_new',version:'2',replayed:false})},()=> 'retail_customer_csrf='+'a'.repeat(64),false)
 assert.equal((await binding.transfer('cart_new','new_account',command)).version,'2');assert.deepEqual(calls,['/api/store/customer/me','/api/store/customer/cart-transfer'])
})

test('changed account, failed lookup, missing or duplicate CSRF and malformed replies prevent addition',async()=>{
 const command={operation_id:'11111111-1111-4111-8111-111111111111',version:'1'}
 for(const mode of ['account','unavailable','csrf missing','csrf duplicate','wrong cart','wrong version','unexpected field']){
  let posts=0
  const binding=cartCustomerBinding(async(url)=>{if(String(url).endsWith('/me'))return mode==='unavailable'?Response.json({}, {status:503}):Response.json({customer:{id:mode==='account'?'other_account':'new_account'}});posts++;return Response.json({cart_id:mode==='wrong cart'?'foreign':'cart_new',version:mode==='wrong version'?'3':'2',replayed:false,...mode==='unexpected field'?{customer_id:'foreign'}:{}})},()=>mode==='csrf missing'?'':mode==='csrf duplicate'?'retail_customer_csrf='+'a'.repeat(64)+'; retail_customer_csrf='+'a'.repeat(64):'retail_customer_csrf='+'a'.repeat(64),false)
  await assert.rejects(()=>binding.transfer('cart_new','new_account',command));if(['account','unavailable','csrf missing','csrf duplicate'].includes(mode))assert.equal(posts,0)
 }
 const anonymous=cartCustomerBinding(async()=>Response.json({},{status:401}),()=>'',false);assert.equal(await anonymous.current(),null);await assert.rejects(()=>anonymous.transfer('cart_new','new_account',command))
})
