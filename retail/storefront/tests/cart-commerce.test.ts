import test from 'node:test'
import assert from 'node:assert/strict'
import {createHash,randomUUID} from 'node:crypto'
import {createCartCommerce,cartIntentKey,browserCartCommerce} from '../src/lib/cart-commerce.ts'
import {CartError,type CartSnapshot,type CartAmounts} from '../src/lib/cart-client.ts'

function fixture(){
 const data=new Map<string,string>(), receipts=new Map<string,{version:string;quantity:string}>()
 let state:CartSnapshot|null={id:'cart_owned',region_id:'region_eu',currency_code:'eur',locale:'en',sales_channel_id:'channel',version:'1'}
 let quantity=0,posts=0,creations=0,lose=false,failRead=false,scope='a'.repeat(64),storageBroken=false,versionConflict=false
 const bindScope=async(s:string)=>createHash('sha256').update(s).digest('hex')
 const client={
  current:async()=>state,
  openScope:async()=>({csrfToken:scope}),
  create:async(input:{operationID:string})=>{creations++;state??={id:'cart_created',region_id:'region_eu',currency_code:'eur',locale:'en',sales_channel_id:'channel',version:'1'};return {cart:state,csrfToken:'b'.repeat(64),expiresAt:'2026-10-30T00:00:00Z',replayed:creations>1}},
  addItem:async(id:string,input:{operationID:string;version:string;quantity:string})=>{
   posts++
   if(versionConflict){versionConflict=false;state!.version='2';throw new CartError('conflict',409)}
   let receipt=receipts.get(input.operationID)
   const replayed=!!receipt
   if(!receipt){quantity++;state!.version=(BigInt(state!.version)+1n).toString();receipt={version:state!.version,quantity:String(quantity)};receipts.set(input.operationID,receipt)}
   if(lose){lose=false;throw new CartError('unavailable',null,true)}
   return {cart_id:id,line_id:'line_owned',...receipt,replayed}
  },
  retrieveAmounts:async()=>{if(failRead){failRead=false;throw new CartError('unavailable',503)};return {cart_id:state!.id,version:state!.version,currency_code:'eur',lines:[{variant_id:'variant_one',quantity:String(quantity)}]} as CartAmounts},
 }
 const options={client,regionID:'region_eu',currency:'eur',segment:'dk-en',
  storage:{getItem:(k:string)=>data.get(k)??null,setItem:(k:string,v:string)=>{if(storageBroken)throw Error('quota');data.set(k,v)},removeItem:(k:string)=>{data.delete(k)}},
  readCSRF:()=>'b'.repeat(64),readScopeCSRF:()=>scope,uuid:randomUUID,bindScope,exclusive:async<T>(fn:()=>Promise<T>)=>fn()}
 return {options,data,receipts,stats:()=>({posts,creations,quantity}),lose:()=>{lose=true},failRead:()=>{failRead=true},breakStorage:()=>{storageBroken=true},noCart:()=>{state=null},foreignCart:()=>{state!.region_id='other'},conflict:()=>{versionConflict=true},rotateScope:()=>{scope='c'.repeat(64)}}
}
const input={variantId:'variant_one',quantity:'1' as const,countryCode:'dk-en'}

test('commerce coalesces concurrent clicks and creates only when no current cart',async()=>{
 const f=fixture(),c=createCartCommerce(f.options)
 const [a,b]=await Promise.all([c.addToCart(input),c.addToCart(input)])
 assert.deepEqual(a,b);assert.equal(a.ok,true);assert.deepEqual(f.stats(),{posts:1,creations:0,quantity:1});assert.equal(f.data.size,0)
 const empty=fixture();empty.noCart();const result=await createCartCommerce(empty.options).addToCart(input)
 assert.equal(result.ok,true);assert.deepEqual(empty.stats(),{posts:1,creations:1,quantity:1});assert.equal(empty.data.size,0)
})
test('lost committed response survives coordinator recreation with same receipt and no duplicate item',async()=>{
 const f=fixture();f.lose()
 assert.deepEqual(await createCartCommerce(f.options).addToCart(input),{ok:false,error:'error'})
 const saved=f.data.get(cartIntentKey)!;assert.equal(JSON.parse(saved).kind,'add')
 assert.equal(saved.includes('b'.repeat(64)),false)
 assert.deepEqual(await createCartCommerce(f.options).addToCart({...input,variantId:'different'}),{ok:false,error:'error'})
 assert.equal(f.stats().posts,1)
 const result=await createCartCommerce(f.options).addToCart(input)
 assert.equal(result.ok,true);assert.equal(f.receipts.size,1);assert.equal(f.stats().quantity,1);assert.equal(f.data.size,0)
})
test('failed read after successful addition resumes refresh without issuing another POST',async()=>{
 const f=fixture();f.failRead()
 assert.equal((await createCartCommerce(f.options).addToCart(input)).ok,false)
 assert.equal(JSON.parse(f.data.get(cartIntentKey)!).kind,'refresh')
 assert.equal((await createCartCommerce(f.options).addToCart(input)).ok,true)
 assert.equal(f.stats().posts,1)
})
test('storage failure, foreign region, corrupt journal and disabled browser never create writes',async()=>{
 const f=fixture();f.breakStorage();assert.equal((await createCartCommerce(f.options).addToCart(input)).ok,false);assert.equal(f.stats().posts,0)
 const foreign=fixture();foreign.foreignCart();assert.equal((await createCartCommerce(foreign.options).addToCart(input)).ok,false);assert.equal(foreign.stats().posts,0)
 const bad=fixture();bad.data.set(cartIntentKey,'{"format":1,"private":"bad"}');assert.equal((await createCartCommerce(bad.options).addToCart(input)).ok,false);assert.equal(bad.stats().posts,0)
 assert.deepEqual(browserCartCommerce({enabled:false,regionID:'region_eu',currency:'eur',segment:'dk-en'}),{enabled:false})
})
test('definite first version conflict needs a new user click; resumed uncertainty never discards ownership failures',async()=>{
 const f=fixture();f.conflict();const c=createCartCommerce(f.options)
 assert.equal((await c.addToCart(input)).ok,false);assert.equal(f.data.size,0)
 assert.equal((await c.addToCart(input)).ok,true);assert.equal(f.stats().quantity,1)
 const uncertain=fixture();uncertain.lose();await createCartCommerce(uncertain.options).addToCart(input)
 const before=uncertain.data.get(cartIntentKey)
 uncertain.options.client.addItem=async()=>{throw new CartError('session',401)}
 assert.equal((await createCartCommerce(uncertain.options).addToCart(input)).ok,false)
 assert.equal(uncertain.data.get(cartIntentKey),before)
})
