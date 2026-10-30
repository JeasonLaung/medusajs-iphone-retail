import test from 'node:test'
import assert from 'node:assert/strict'
import {customerClient} from '../src/lib/customer-client.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'
import {photoContentProjection,photoProjection} from '../src/lib/buyback-photos.ts'
const root='/api/store/customer/buyback/requests',id='bbr_fixture',item='bbi_fixture',csrf='b'.repeat(64),origin='http://127.0.0.1:9300'
const photo={id:'bbp_fixture',request_id:id,item_id:item,photo_type:'front',uploaded_at:'2026-10-30T00:00:00Z',review_status:'pending',review_note:'',mime_type:'image/jpeg'}
const input={item_id:item,photo_type:'front',mime_type:'image/jpeg',content:'/9j/'}
test('photo projection validates bytes, MIME, identity and the full 3 MiB response',()=>{
 assert.deepEqual(photoProjection(photo,id),photo)
 assert.deepEqual(photoContentProjection({content:'/9j/',mime_type:'image/jpeg'}),{content:'/9j/',mime_type:'image/jpeg'})
 const bytes=Buffer.alloc(3*1024*1024);bytes.set([255,216,255])
 assert.equal(photoContentProjection({content:bytes.toString('base64'),mime_type:'image/jpeg'}).content.length,4194304)
 for(const raw of [{content:'/9j/',mime_type:'image/png'},{content:'AAAA',mime_type:'image/jpeg'},{content:'/9j/',mime_type:'image/jpeg',storage_key:'PRIVATE'},{content:Buffer.concat([bytes,Buffer.alloc(1)]).toString('base64'),mime_type:'image/jpeg'}])assert.throws(()=>photoContentProjection(raw))
 for(const raw of [{...photo,request_id:'foreign'},{...photo,storage_key:'PRIVATE'},{...photo,review_status:'unknown'},{...photo,mime_type:'image/gif'}])assert.throws(()=>photoProjection(raw,id))
})
test('customer photo client uses CSRF, strict statuses and allowlisted localized error codes',async()=>{
 const previousDocument=Object.getOwnPropertyDescriptor(globalThis,'document'),previousLocation=Object.getOwnPropertyDescriptor(globalThis,'location')
 Object.defineProperty(globalThis,'document',{value:{cookie:`retail_customer_csrf=${csrf}`},configurable:true});Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 try{
  let calls=0
  const api=customerClient(async(url,init)=>{
   calls++;assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error')
   if(init?.method==='POST'){
    assert.equal(String(url),root+'/'+id+'/photos');assert.equal(new Headers(init.headers).get('X-CSRF-Token'),csrf);assert.deepEqual(JSON.parse(String(init.body)),input)
    return Response.json({photo},{status:201})
   }
   assert.equal(String(url),root+'/'+id+'/photos/'+photo.id);return Response.json({content:'/9j/',mime_type:'image/jpeg'})
  })
  assert.deepEqual(await api.uploadBuybackPhoto(id,input),{data:{photo},error:null})
  assert.deepEqual(await api.buybackPhoto(id,photo.id),{data:{content:'/9j/',mime_type:'image/jpeg'},error:null});assert.equal(calls,2)
  assert.equal((await api.buybackPhoto('../foreign',photo.id)).error,'BUYBACK_REQUEST_FAILED');assert.equal(calls,2)
  for(const [status,code] of [[400,'BUYBACK_INVALID_PHOTO'],[404,'BUYBACK_PHOTO_NOT_FOUND'],[409,'BUYBACK_INVALID_STATE'],[409,'BUYBACK_TOO_MANY_PHOTOS'],[401,'BUYBACK_LOGIN_REQUIRED']]){
   assert.equal((await customerClient(async()=>Response.json({error:{code}},{status:status as number})).uploadBuybackPhoto(id,input)).error,code)
  }
  for(const status of [200,202])assert.equal((await customerClient(async()=>Response.json({photo},{status})).uploadBuybackPhoto(id,input)).error,'BUYBACK_REQUEST_FAILED')
  assert.equal((await customerClient(async()=>Response.json({error:{code:'PRIVATE'}},{status:409})).uploadBuybackPhoto(id,input)).error,'BUYBACK_REQUEST_FAILED')
 }finally{if(previousDocument)Object.defineProperty(globalThis,'document',previousDocument);else Reflect.deleteProperty(globalThis,'document');if(previousLocation)Object.defineProperty(globalThis,'location',previousLocation);else Reflect.deleteProperty(globalThis,'location')}
})
test('photo bridge widens only fixed upload and photo response bodies and preserves original signed bytes',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:origin,bridgeKey:'a'.repeat(64),clientAddress:'127.0.0.1'}
 const request=(path:string,body?:string)=>new Request(origin+path,{method:body?'POST':'GET',headers:{Origin:origin,'Content-Type':'application/json',Cookie:`retail_customer_session=${csrf}`,'X-CSRF-Token':csrf},body})
 const reply=(data:unknown,status=200)=>Response.json(data,{status,headers:{'Cache-Control':'private, no-store'}})
 const bytes=Buffer.alloc(3*1024*1024);bytes.set([255,216,255]);const content=bytes.toString('base64'),body=JSON.stringify({...input,content})
 let calls=0
 const upload=await customerBridge(request(root+'/'+id+'/photos',body),config,async(url,init)=>{
  calls++;assert.equal(String(url),config.ginOrigin+root+'/'+id+'/photos');assert.equal(new TextDecoder().decode(init?.body as Uint8Array),body);assert.match(new Headers(init?.headers).get('X-Retail-Bridge-Proof')??'',/^[a-f0-9]{64}$/);return reply({photo},201)
 })
 assert.equal(upload.status,201)
 const read=await customerBridge(request(root+'/'+id+'/photos/'+photo.id),config,async()=>reply({content,mime_type:'image/jpeg'}));assert.equal(read.status,200);assert.equal((await read.json()).content,content)
 assert.equal((await customerBridge(request(root,body),config,async()=>{calls++;return reply({})})).status,413);assert.equal(calls,1)
 assert.equal((await customerBridge(request(root+'/'+id+'/photos','x'.repeat((4<<20)+8193)),config)).status,413)
 assert.equal((await customerBridge(request(root+'/'+id+'/photos?offset=0',body),config)).status,404)
 for(const [status,code] of [[400,'BUYBACK_INVALID_PHOTO'],[404,'BUYBACK_PHOTO_NOT_FOUND'],[409,'BUYBACK_TOO_MANY_PHOTOS']]){
  const result=await customerBridge(request(root+'/'+id+'/photos',JSON.stringify(input)),config,async()=>reply({error:{code,message:'PRIVATE_DATABASE'}},status as number))
  assert.equal((await result.json()).error.code,code)
 }
 for(const status of [200,202])assert.equal((await customerBridge(request(root+'/'+id+'/photos',JSON.stringify(input)),config,async()=>reply({photo},status))).status,503)
})
