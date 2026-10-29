import test from 'node:test'
import assert from 'node:assert/strict'
import {loadPaymentReview} from '../src/lib/checkout-payment-review.ts'
import type {CartClient} from '../src/lib/cart-client.ts'
function fixture(frozen=false){
 let deliveryReads=0
 const data={contacts:{cart_id:'cart',version:'5',email:'buyer@example.invalid',shipping_address:{},billing_address:{}},view:{amounts:{cart_id:'cart',version:'5',currency_code:'eur',shipping_methods:[{}]},items:[{}]},options:{cart_id:'cart',version:'5',providers:[],attempt_started:frozen},delivery:{complete:true,version:'5',currency_code:'eur'}}
 const api={retrieveContacts:async()=>data.contacts,retrieveView:async()=>data.view,paymentOptions:async()=>data.options,retrieveDelivery:async()=>{deliveryReads++;return data.delivery}} as unknown as CartClient
 return {data,api,reads:()=>deliveryReads}
}
test('review reads exact frozen amount version and avoids mutable shipping quote after freeze',async()=>{
 const f=fixture(true);const state=await loadPaymentReview(f.api,'cart','5',true);assert.equal(state.view,f.data.view);assert.equal(f.reads(),0)
 await assert.rejects(loadPaymentReview(f.api,'cart','4',true));await assert.rejects(loadPaymentReview(f.api,'cart','6'))
})
test('new payment requires complete current delivery before showing methods',async()=>{
 const f=fixture();await loadPaymentReview(f.api,'cart','4');assert.equal(f.reads(),1)
 f.data.delivery.complete=false;await assert.rejects(loadPaymentReview(f.api,'cart','4'))
 f.data.delivery.complete=true;f.data.delivery.version='6';await assert.rejects(loadPaymentReview(f.api,'cart','4'))
})
test('review refuses mixed versions, missing contact or missing shipping',async()=>{
 for(const corrupt of [(f:ReturnType<typeof fixture>)=>{f.data.contacts.cart_id='foreign'},(f:ReturnType<typeof fixture>)=>{f.data.contacts.version='6'},(f:ReturnType<typeof fixture>)=>{f.data.options.version='6'},(f:ReturnType<typeof fixture>)=>{f.data.contacts.email=''},(f:ReturnType<typeof fixture>)=>{f.data.view.items=[]},(f:ReturnType<typeof fixture>)=>{f.data.view.amounts.shipping_methods=[]}]){const f=fixture(true);corrupt(f);await assert.rejects(loadPaymentReview(f.api,'cart','5'))}
})
