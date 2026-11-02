import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient} from '../src/lib/cart-client.ts'
import {createPaymentCommands,paymentIntentKey,clearCompletedPayment} from '../src/lib/checkout-payment.ts'
const reply=(v:unknown)=>Response.json(v,{headers:{'Cache-Control':'private, no-store'}})
const operationID='12345678-1234-4234-8234-123456789012',csrf='a'.repeat(64),wallet='pp_paypal_paypal' as const
test('wallet journal preserves original provider on loss and blocks provider switching',async()=>{
 const values=new Map<string,string>(),calls:unknown[]=[]
 const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(_path,init)=>{calls.push(JSON.parse(String(init?.body)));if(calls.length===1)throw Error('lost');return reply({cart_id:'cart',version:'5',operation_id:operationID,state:'sdk_ready',checkout_url:null,provider_id:wallet,provider_order_id:'5O190127TN364715T'})}})
 const storage={getItem:(k:string)=>values.get(k)??null,setItem:(k:string,v:string)=>{values.set(k,v)},removeItem:(k:string)=>{values.delete(k)}}
 const c=createPaymentCommands({client,storage,readCSRF:()=>csrf,uuid:()=>operationID,exclusive:async a=>a()})
 await assert.rejects(c.submit('cart','4',wallet));await assert.rejects(c.submit('cart','5'))
 assert.equal((await c.submit('cart','5',wallet)).state,'sdk_ready');assert.deepEqual(calls,[{version:'4',provider_id:wallet},{version:'4',provider_id:wallet}])
 assert.equal(c.pending()?.providerID,wallet);clearCompletedPayment(storage,'cart');assert.equal(storage.getItem(paymentIntentKey),null)
})
test('settlement transmits no browser order, amount or capture proof',async()=>{
 const c=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{assert.equal(path,'/api/store/carts/cart/paypal-settlement');assert.equal(init?.body,'{}');assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf);return reply({cart_id:'cart',version:'5',state:'paid_pending_order'})}})
 assert.equal((await c.settlePayPal('cart',{csrfToken:csrf})).state,'paid_pending_order')
})
test('wallet SDK options reject private configuration and conflicting providers',async()=>{
 const paypal={client_id:'artificial_client',currency:'EUR',intent:'capture',environment:'sandbox'}
 const options={cart_id:'cart',version:'4',attempt_started:false,providers:[{id:wallet,paypal}]}
 const read=(body:unknown)=>new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(body)}).paymentOptions('cart')
 assert.equal((await read(options))?.providers[0].paypal?.currency,'EUR')
 for(const body of [{...options,providers:[{id:wallet,paypal:{...paypal,client_secret:'private'}}]},{...options,providers:[options.providers[0],options.providers[0]]},{...options,active_provider:options.providers[0]},{...options,providers:[{id:wallet,paypal:{...paypal,currency:'eur'}}]}])await assert.rejects(read(body))
})
