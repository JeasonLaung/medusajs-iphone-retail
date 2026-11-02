import assert from 'node:assert/strict'
import test from 'node:test'
import {publicCatalogImage} from '../src/lib/catalog-image.ts'
const id='img_0031fc47-9a68-4e0b-ad64-877f380ba7fa'
test('public catalog asset bridge isolates the fixed path and browser credentials',async()=>{
 let called=0
 const response=await publicCatalogImage('http://127.0.0.1:9100',id,'',async(url,init)=>{
  called++;assert.equal(String(url),'http://127.0.0.1:9100/api/store/catalog-images/'+id);assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error');assert.deepEqual(init?.headers,{Accept:'image/jpeg,image/png,image/webp'});return new Response(new Uint8Array([1,2,3]),{headers:{'Content-Type':'image/png'}})
 });assert.equal(called,1);assert.equal(response.status,200);assert.deepEqual([...new Uint8Array(await response.arrayBuffer())],[1,2,3]);assert.equal(response.headers.get('X-Content-Type-Options'),'nosniff')
 for(const [base,key,query]of [['http://127.0.0.1:9100', '../private',''],['http://user:password@127.0.0.1:9100',id,''],['http://127.0.0.1:9100',id,'?private=1']]){let called=false;const r=await publicCatalogImage(base,key,query,async()=>{called=true;throw Error()});assert.notEqual(r.status,200);assert.equal(called,false)}
})
test('catalog asset bridge rejects redirects, active MIME and oversize streams',async()=>{
 for(const source of [new Response(null,{status:302,headers:{Location:'https://example.test/private'}}),new Response('<svg/>',{headers:{'Content-Type':'image/svg+xml'}}),new Response(new Uint8Array(3*1024*1024+1),{headers:{'Content-Type':'image/png'}})]){const r=await publicCatalogImage('http://127.0.0.1:9100',id,'',async()=>source);assert.equal(r.status,503)}
 assert.equal((await publicCatalogImage('http://127.0.0.1:9100',id,'',async()=>new Response(null,{status:404}))).status,404)
})
