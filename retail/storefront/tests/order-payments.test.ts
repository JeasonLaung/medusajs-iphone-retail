import test from 'node:test'
import assert from 'node:assert/strict'
import {paymentContext,paymentReceipt,paymentCommand,paymentCheckoutURL} from '../src/lib/order-payments.ts'
const id='f9adc90d-1b76-46b3-8f1c-36c83c69d627',order='order_created',consent={expected_version:3,collection_id:'paycol_extra',amount:'6.00',currency_code:'eur'},receipt={operation_id:id,order_id:order,order_version:3,collection_id:'paycol_extra',amount:'6.00',currency_code:'eur',state:'pending',applied:false,can_cancel:true,can_retry:false},context={order_id:order,order_version:3,currency_code:'eur',pending_difference:'6',next:consent,payments:[]}
test('exact supplemental consent, pending receipt and unique paid local receipt are projected',()=>{
 assert.deepEqual(paymentContext(context,order),context)
 assert.deepEqual(paymentCommand(consent,id),{...consent,expected_version:'3',operation_id:id})
 assert.equal(paymentReceipt(receipt,order,id).checkout_url,null)
 const view={...receipt,state:'open',can_cancel:false,checkout_url:'https://www.mollie.com/checkout/owned'}
 assert.equal(paymentReceipt(view,order,id,true).checkout_url,view.checkout_url)
 const paid={...receipt,state:'paid',applied:true,can_cancel:false,checkout_url:null}
 assert.equal(paymentContext({...context,pending_difference:'0',next:null,payments:[paid]},order).payments[0].applied,true)
})
test('supplemental projection refuses ambiguous settlement, private fields and contradictory flags',()=>{
 for(const patch of [{pending_difference:6},{next:{...consent,amount:'6.01'}},{next:{...consent,expected_version:2}},{next:{...consent,currency_code:'usd'}},{payments:[{...receipt,checkout_url:null},{...receipt,checkout_url:null}]},{payments:[{...receipt,checkout_url:null}]},{next:{...consent,previous_operation_id:id}},{provider_id:'PRIVATE'}])assert.throws(()=>paymentContext({...context,...patch},order))
 for(const patch of [{amount:'6'},{amount:6},{amount:'0.00'},{state:'successful'},{state:'open',can_cancel:true,checkout_url:'https://www.mollie.com/checkout/owned'},{applied:true},{can_retry:true},{cart_id:'PRIVATE'},{checkout_url:'https://evil.test/checkout'}])assert.throws(()=>paymentReceipt({...receipt,checkout_url:null,...patch},order,id,true))
 assert.throws(()=>paymentReceipt(receipt,'foreign',id))
 assert.throws(()=>paymentCommand(consent,id.toUpperCase()))
})
test('hosted checkout URL is a bounded HTTPS Mollie location',()=>{
 for(const url of ['https://www.mollie.com/checkout/owned','https://mollie.com:443/checkout/owned?lang=en'])assert.equal(paymentCheckoutURL(url),url)
 for(const url of ['http://mollie.com/checkout','https://mollie.com.evil.test/checkout','https://evil-mollie.com/checkout','https://user@mollie.com/checkout','https://mollie.com:444/checkout','https://mollie.com/checkout#private','//mollie.com/checkout','javascript:alert(1)','https://mollie.com/\ncheckout'])assert.throws(()=>paymentCheckoutURL(url))
})
