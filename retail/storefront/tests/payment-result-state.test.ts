import test from 'node:test'
import assert from 'node:assert/strict'
import {CartError} from '../src/lib/cart-client.ts'
import {checkPaymentResult} from '../src/lib/payment-result-state.ts'
const cart={id:'cart',region_id:'r',sales_channel_id:'c',currency_code:'eur',locale:'en',version:'5'}
const csrf='a'.repeat(64)
const receipt={cart_id:'cart',order_id:'order_created',version:'6'}

test('only a completed server receipt confirms a verified paid attempt',async()=>{
 let completions=0
 const api={current:async()=>cart,paymentStatus:async()=>({cart_id:'cart',version:'5',state:'paid_pending_order' as const}),complete:async()=>{completions++;return receipt}}
 assert.deepEqual(await checkPaymentResult(api,'cart',csrf),{status:'confirmed',orderID:receipt.order_id});assert.equal(completions,1)
 for(const state of ['pending','authorized','failed','requires_review'] as const){
  completions=0
  const result=await checkPaymentResult({...api,paymentStatus:async()=>({cart_id:'cart',version:'5',state})},'cart',csrf)
  assert.equal(result.status,state==='failed'?'failed':state==='requires_review'?'error':'pending');assert.equal(completions,0)
 }
 for(const error of [new CartError('unavailable',503,true),new CartError('conflict',409),new CartError('item',422),new CartError('protocol',200,true)]){
  assert.equal((await checkPaymentResult({...api,complete:async()=>{throw error}},'cart',csrf)).status,'error')
 }
})

test('closed-cart reload and a concurrent completion recover via the same receipt',async()=>{
 let polls=0,completions=0
 const api={current:async()=>null,paymentStatus:async()=>{polls++;throw Error('closed cart must not poll')},complete:async(id:string)=>{assert.equal(id,'cart');completions++;return receipt}}
 assert.equal((await checkPaymentResult(api,'cart',csrf)).status,'confirmed');assert.equal(polls,0)
 assert.equal((await checkPaymentResult({...api,current:async()=>cart,paymentStatus:async()=>{throw new CartError('session',404)}},'cart',csrf)).status,'confirmed');assert.equal(completions,2)
 const uncertain={...api,complete:async()=>{throw new CartError('unavailable',503,true)}}
 assert.equal((await checkPaymentResult(uncertain,'cart',csrf)).status,'error')
 assert.equal((await checkPaymentResult(api,undefined,csrf)).status,'invalid')
})

test('wrong cart, revoked possession and missing CSRF never infer confirmation',async()=>{
 let writes=0
 const api={current:async()=>cart,paymentStatus:async()=>({cart_id:'cart',version:'5',state:'paid_pending_order' as const}),complete:async()=>{writes++;return receipt}}
 assert.equal((await checkPaymentResult(api,'foreign',csrf)).status,'invalid')
 assert.equal((await checkPaymentResult(api,'cart',undefined)).status,'invalid');assert.equal(writes,0)
 assert.equal((await checkPaymentResult({...api,current:async()=>null,complete:async()=>{throw new CartError('session',401)}},'cart',csrf)).status,'invalid')
 assert.equal((await checkPaymentResult({...api,current:async()=>{throw new CartError('unavailable',503)}},'cart',csrf)).status,'error');assert.equal(writes,0)
})
