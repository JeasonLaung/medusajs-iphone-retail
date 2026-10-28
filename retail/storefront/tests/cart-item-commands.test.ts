import test from 'node:test'
import assert from 'node:assert/strict'
import {randomUUID,createHash} from 'node:crypto'
import {CartError,type CartSnapshot,type CartView} from '../src/lib/cart-client.ts'
import {createCartItemCommands,browserCartItemCommands} from '../src/lib/cart-item-commands.ts'
import {cartIntentKey,createCartCommerce} from '../src/lib/cart-commerce.ts'

function fixture(){
 const data=new Map<string,string>(),receipts=new Map<string,{version:string;quantity:string}>(),requests:{operationID:string;version:string;quantity:string}[]=[]
 let state:CartSnapshot={id:'cart_one',region_id:'region',currency_code:'eur',sales_channel_id:'channel',locale:'en',version:'3'},qty='2',lose=false,failRead=false,denied=false,storageBroken=false,conflict=false
 let tail=Promise.resolve()
 const options={regionID:'region',currency:'eur',segment:'dk-en',storage:{getItem:(k:string)=>data.get(k)??null,setItem:(k:string,v:string)=>{if(storageBroken)throw Error('quota');data.set(k,v)},removeItem:(k:string)=>{data.delete(k)}},readCSRF:()=>'b'.repeat(64),uuid:randomUUID,
  exclusive:<T>(fn:()=>Promise<T>)=>{const result=tail.then(fn);tail=result.then(()=>{},()=>{});return result},
  client:{current:async()=>denied?null:state,setItem:async(cartID:string,lineID:string,input:{operationID:string;version:string;quantity:string})=>{
   requests.push({...input});if(denied)throw new CartError('session',401)
   if(conflict){conflict=false;throw new CartError('conflict',409)}
   let receipt=receipts.get(input.operationID);const replayed=!!receipt
   if(!receipt){if(input.version!==state.version)throw new CartError('conflict',409);qty=input.quantity;state={...state,version:(BigInt(state.version)+1n).toString()};receipt={version:state.version,quantity:qty};receipts.set(input.operationID,receipt)}
   if(lose){lose=false;throw new CartError('unavailable',null,true)}
   return {cart_id:cartID,line_id:lineID,...receipt,replayed}
  },retrieveView:async()=>{
   if(failRead){failRead=false;throw new CartError('unavailable',503)}
   return {amounts:{cart_id:state.id,currency_code:state.currency_code,version:state.version,lines:qty==='0'?[]:[{id:'line_one',quantity:qty}]},items:qty==='0'?[]:[{id:'line_one'}]} as CartView
  }}}
 return {options,data,requests,receipts,qty:()=>qty,lose:()=>{lose=true},failRead:()=>{failRead=true},deny:()=>{denied=true},breakStorage:()=>{storageBroken=true},conflict:()=>{conflict=true},advance:()=>{state={...state,version:'4'}},foreign:()=>{state={...state,region_id:'other'}}}
}
const input={cartID:'cart_one',version:'3',lineID:'line_one',quantity:'1'}
test('quantity edits coalesce double clicks and concurrent controllers never promote stale UI versions',async()=>{
 const f=fixture(),c=createCartItemCommands(f.options)
 const [a,b]=await Promise.all([c.set(input),c.set(input)])
 assert.deepEqual(a,b);assert.equal(f.requests.length,1);assert.equal(f.qty(),'1');assert.deepEqual(c.pending(),{kind:'none'})
 await assert.rejects(createCartItemCommands(f.options).set({...input,quantity:'3'}))
 assert.equal(f.requests[1].version,'3');assert.equal(f.qty(),'1');assert.equal(f.data.size,0)
 const concurrent=fixture()
 const results=await Promise.allSettled([
  createCartItemCommands(concurrent.options).set(input),
  createCartItemCommands(concurrent.options).set({...input,quantity:'3'})
 ])
 assert.equal(results.filter(result=>result.status==='fulfilled').length,1)
 assert.equal(concurrent.receipts.size,1);assert.equal(concurrent.requests.length,2)
 assert.equal(concurrent.requests[1].version,'3');assert.equal(concurrent.qty(),'1');assert.equal(concurrent.data.size,0)
})
test('lost committed removal recovers after recreation with exactly the same operation key and original version',async()=>{
 const f=fixture();f.lose();const command={...input,quantity:'0'}
 await assert.rejects(createCartItemCommands(f.options).set(command))
 const saved=f.data.get(cartIntentKey)!,pending=JSON.parse(saved)
 assert.equal(pending.kind,'set');assert.equal(pending.quantity,'0');assert.ok(!saved.includes('b'.repeat(64)))
 assert.equal(f.qty(),'0');assert.equal(f.receipts.size,1)
 const recovered=await createCartItemCommands(f.options).recover()
 assert.equal(recovered.items.length,0);assert.deepEqual(f.requests[0],f.requests[1]);assert.equal(f.receipts.size,1);assert.equal(f.data.size,0)
})
test('successful receipt followed by failed refresh recovers with GET only, never a replacement write',async()=>{
 const f=fixture();f.failRead();await assert.rejects(createCartItemCommands(f.options).set(input))
 assert.equal(JSON.parse(f.data.get(cartIntentKey)!).kind,'refresh')
 await createCartItemCommands(f.options).recover();assert.equal(f.requests.length,1);assert.equal(f.data.size,0)
})
test('blocked journals, storage failure, foreign contexts and changed pending requests preserve original intent',async()=>{
 for(const raw of ['{bad','{"format":1,"kind":"add"}',JSON.stringify({format:2,kind:'set',regionID:'other',currency:'eur',segment:'dk-en',cartID:'cart_one',lineID:'line_one',quantity:'1',version:'3',operationID:randomUUID()})]){
  const f=fixture();f.data.set(cartIntentKey,raw);const c=createCartItemCommands(f.options);assert.equal(c.pending().kind,'blocked');await assert.rejects(c.set(input));assert.equal(f.data.get(cartIntentKey),raw);assert.equal(f.requests.length,0)
 }
 const f=fixture();f.lose();await assert.rejects(createCartItemCommands(f.options).set(input));const before=f.data.get(cartIntentKey)
 await assert.rejects(createCartItemCommands(f.options).set({...input,quantity:'0'}));f.deny();await assert.rejects(createCartItemCommands(f.options).recover());assert.equal(f.data.get(cartIntentKey),before);assert.equal(f.requests.length,1)
 const broken=fixture();broken.breakStorage();await assert.rejects(createCartItemCommands(broken.options).set(input));assert.equal(broken.requests.length,0)
 const foreign=fixture();foreign.foreign();await assert.rejects(createCartItemCommands(foreign.options).set(input));assert.equal(foreign.requests.length,0)
})
test('definite rejection clears only fresh intent; pending item changes block product additions in the shared journal',async()=>{
 const f=fixture();f.conflict();await assert.rejects(createCartItemCommands(f.options).set(input));assert.equal(f.data.size,0)
 f.lose();await assert.rejects(createCartItemCommands(f.options).set(input));const saved=f.data.get(cartIntentKey)
 let adds=0
 const commerce=createCartCommerce({...f.options,client:{current:f.options.client.current,openScope:async()=>{throw Error('must not create')},create:async()=>{throw Error('must not create')},addItem:async()=>{adds++;throw Error('must not add')},retrieveAmounts:async()=>{throw Error('must not read')}},readScopeCSRF:()=>undefined,bindScope:async(s:string)=>createHash('sha256').update(s).digest('hex')})
 assert.equal((await commerce.addToCart({countryCode:'dk-en',variantId:'variant',quantity:'1'})).ok,false);assert.equal(adds,0);assert.equal(f.data.get(cartIntentKey),saved)
})
test('invalid input, missing recovery and disabled browser never issue a write',async()=>{
 const f=fixture(),c=createCartItemCommands(f.options)
 for(const patch of [{quantity:'00'},{quantity:'-1'},{quantity:'1000000001'},{version:'18446744073709551615'},{lineID:'../bad'}])await assert.rejects(c.set({...input,...patch}))
 await assert.rejects(c.recover());assert.equal(f.requests.length,0)
 assert.deepEqual(browserCartItemCommands({enabled:false,regionID:'region',currency:'eur',segment:'dk-en'}),{enabled:false})
})
