import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError,type CartView,type CartContactDetail,type CartContactInput} from '../src/lib/cart-client.ts'
import {createContactCommands,contactFields,contactIntentKey} from '../src/lib/checkout-contacts.ts'
import {cartIntentKey} from '../src/lib/cart-commerce.ts'
const address={first_name:'New',last_name:'Customer',company:'',address_1:'Private street 1',address_2:'',city:'Copenhagen',country_code:'dk',province:'',postal_code:'0012',phone:'+45 11'}
const fields={email:'customer@example.invalid',shipping_address:address,billing_address:{...address}}
const key='12345678-1234-4234-8234-123456789012',csrf='a'.repeat(64)
const reply=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
test('contact client projects value fields only and verifies exact next version receipts',async()=>{
 const input={...fields,operationID:key,version:'9007199254740993123',shipping_address:{...address,id:'old-private-address'},customer_id:'old'}
 const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
  assert.equal(path,'/api/store/carts/cart/contacts');assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store')
  const headers=new Headers(init?.headers);assert.equal(headers.get('Idempotency-Key'),key);assert.equal(headers.get('X-CSRF-Token'),csrf)
  const sent=JSON.parse(String(init?.body));assert.deepEqual(Object.keys(sent),['version','email','shipping_address','billing_address']);assert.equal(sent.shipping_address.id,undefined);assert.equal(sent.shipping_address.postal_code,'0012')
  return reply({cart_id:'cart',version:'9007199254740993124',replayed:false})
 }})
 assert.equal((await client.saveContacts('cart',input,{csrfToken:csrf})).version,'9007199254740993124')
 const bad=new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({cart_id:'cart',version:'2',replayed:false})})
 await assert.rejects(bad.saveContacts('cart',input,{csrfToken:csrf}),e=>e instanceof CartError&&e.uncertain)
})
test('contact reads preserve absence versus outage and project only copied private values',async()=>{
 const detail={cart_id:'cart',version:'3',...fields}
 assert.deepEqual(await new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(detail)}).retrieveContacts('cart'),detail)
 assert.equal(await new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},401)}).retrieveContacts('cart'),null)
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},503)}).retrieveContacts('cart'),e=>e instanceof CartError&&e.status===503)
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({...detail,shipping_address:{...address,postal_code:12}})}).retrieveContacts('cart'))
})
function fixture(){
 const data=new Map<string,string>(),calls:CartContactInput[]=[];let failWrite=false,failRead=false
 let contacts:CartContactDetail={cart_id:'cart',version:'2',...fields}
 const view=()=>({amounts:{cart_id:'cart',version:contacts.version,currency_code:'eur'},items:[]}) as unknown as CartView
 const commands=createContactCommands({storage:{getItem:k=>data.get(k)??null,setItem:(k,v)=>{data.set(k,v)},removeItem:k=>{data.delete(k)}},readCSRF:()=>csrf,uuid:()=>key,hash:async value=>{assert.ok(!value.includes('customer_id'));return value.includes('Other')?'b'.repeat(64):'c'.repeat(64)},exclusive:async action=>action(),client:{
  saveContacts:async(_id,input)=>{calls.push(input);contacts={...contacts,version:'3'};if(failWrite)throw new CartError('unavailable',null,true);return {cart_id:'cart',version:'3',replayed:calls.length>1}},
  retrieveContacts:async()=>{if(failRead)throw new CartError('unavailable');return contacts},retrieveView:async()=>view()
 }})
 return {commands,data,calls,failWrite:(v:boolean)=>{failWrite=v},failRead:(v:boolean)=>{failRead=v}}
}
test('lost contact response retains same operation/version and no private form values in journal',async()=>{
 const f=fixture();f.failWrite(true)
 await assert.rejects(f.commands.submit('cart','2',fields))
 const journal=f.data.get(contactIntentKey)!;assert.ok(journal);for(const privateValue of [fields.email,address.address_1,address.postal_code,csrf])assert.equal(journal.includes(privateValue),false)
 await assert.rejects(f.commands.submit('cart','3',{...fields,shipping_address:{...address,first_name:'Other'}}))
 assert.equal(f.calls.length,1)
 f.failWrite(false);const result=await f.commands.submit('cart','3',fields)
 assert.equal(result.contacts.version,'3');assert.deepEqual(f.calls[1],f.calls[0]);assert.equal(f.data.size,0)
})
test('acknowledged contacts recover by reads only and pending item commands block contact writes',async()=>{
 const f=fixture();f.failRead(true);await assert.rejects(f.commands.submit('cart','2',fields));assert.equal(f.commands.pending()?.kind,'refresh')
 f.failRead(false);assert.equal((await f.commands.recover()).contacts.version,'3');assert.equal(f.calls.length,1)
 f.data.set(cartIntentKey,'pending');await assert.rejects(f.commands.submit('cart','3',fields));assert.equal(f.calls.length,1)
})
test('form uses same billing copies, requires email confirmation and preserves postal zero',()=>{
 const form=new FormData();for(const [k,v] of Object.entries(address))form.set('shipping_address.'+k,v)
 form.set('email',fields.email);form.set('confirm_email',fields.email.toUpperCase());form.set('same_as_billing','on')
 const result=contactFields(form);assert.deepEqual(result.billing_address,address);assert.notEqual(result.billing_address,result.shipping_address);assert.equal(result.shipping_address.postal_code,'0012')
 form.set('confirm_email','other@example.invalid');assert.throws(()=>contactFields(form))
 form.set('confirm_email',fields.email);form.delete('same_as_billing');assert.throws(()=>contactFields(form))
})
