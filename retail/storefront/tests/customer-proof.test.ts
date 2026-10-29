import test from 'node:test'
import assert from 'node:assert/strict'
import {customerProof} from '../src/lib/customer-proof.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'
const key='71'.repeat(32),shop='http://127.0.0.1:9300'
test('customer proof is stable and binds every request field',()=>{
 const headers=new Headers({Host:'shop.example',Origin:'https://shop.example',Cookie:'customer=fixture','X-CSRF-Token':'b'.repeat(64)}),body=new TextEncoder().encode('{}')
 const proof=customerProof(key,'192.0.2.5','POST','/api/store/customer/auth/login',headers,body,1700000000000)
 assert.equal(proof.get('X-Retail-Bridge-Time'),'1700000000')
 assert.equal(proof.get('X-Retail-Client-IP'),'192.0.2.5')
 const value=proof.get('X-Retail-Bridge-Proof')
 assert.equal(value,'805ac93b9fa7d70a0c3103f6829fa1e0a22270cae12b545da7a98f5bd6ef2513')
 for(const [ip,method,path,data] of [['192.0.2.6','POST','/api/store/customer/auth/login',body],['192.0.2.5','GET','/api/store/customer/auth/login',body],['192.0.2.5','POST','/api/store/customer/me',body],['192.0.2.5','POST','/api/store/customer/auth/login',new Uint8Array()]] as const)assert.notEqual(customerProof(key,ip,method,path,headers,data,1700000000000).get('X-Retail-Bridge-Proof'),value)
 for(const ip of ['', '1.2.3.4,5.6.7.8','fe80::1%zone'])assert.throws(()=>customerProof(key,ip,'GET','/',headers,body))
 for(const bad of ['','0'.repeat(64),'AA'.repeat(32)])assert.throws(()=>customerProof(bad,'192.0.2.5','GET','/',headers,body))
})
test('bridge signs adapter client address and ignores attacker proof headers',async()=>{
 const req=new Request(shop+'/api/store/customer/me',{headers:{'X-Retail-Client-IP':'evil','X-Retail-Bridge-Proof':'evil','X-Forwarded-For':'evil'}})
 const result=await customerBridge(req,{ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:shop,bridgeKey:key,clientAddress:'192.0.2.5'},async(_url,init)=>{
  const headers=new Headers(init?.headers)
  assert.equal(headers.get('X-Retail-Client-IP'),'192.0.2.5');assert.equal(headers.get('X-Forwarded-For'),null)
  const expected=customerProof(key,'192.0.2.5','GET','/api/store/customer/me',headers,undefined,Number(headers.get('X-Retail-Bridge-Time'))*1000)
  assert.equal(headers.get('X-Retail-Bridge-Proof'),expected.get('X-Retail-Bridge-Proof'))
  return Response.json({customer:{id:'cust_1'}},{headers:{'Cache-Control':'private, no-store'}})
 })
 assert.equal(result.status,200)
 let called=false
 assert.equal((await customerBridge(req,{ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:shop,bridgeKey:key},async()=>{called=true;throw new Error()})).status,503)
 assert.equal(called,false)
})
