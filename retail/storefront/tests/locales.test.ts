import test from 'node:test'
import assert from 'node:assert/strict'
import {loadLocales} from '../src/lib/locales.ts'
test('public locales preserve configured code/name, project fields and never forward private credentials',async()=>{
 const locales=[{code:'en-GB',name:'English (United Kingdom)'},{code:'zh-CN',name:'Chinese Simplified (China)'}]
 assert.deepEqual(await loadLocales('https://gin.example',async(url,options)=>{
  assert.equal(String(url),'https://gin.example/api/store/locales');assert.equal(options?.credentials,'omit');assert.equal(options?.cache,'no-store');assert.deepEqual(options?.headers,{Accept:'application/json'})
  return Response.json({locales:locales.map(l=>({...l,private:'excluded'})),count:2})
 }),locales)
 assert.deepEqual(await loadLocales('https://gin.example',async()=>Response.json({locales:[],count:0})),[])
})
test('invalid or unavailable locale directory never masquerades as not configured',async()=>{
 for(const body of [{locales:null,count:0},{locales:[],count:1},{locales:[{code:'en-GB',name:''}],count:1},{locales:[{code:'en-GB\n',name:'English'}],count:1},{locales:[{code:'en-GB',name:'bad\nname'}],count:1},{locales:[{code:'en-GB',name:'English'},{code:'EN-gb',name:'Duplicate'}],count:2}])await assert.rejects(loadLocales('https://gin.example',async()=>Response.json(body)))
 for(const status of [404,503])await assert.rejects(loadLocales('https://gin.example',async()=>new Response('',{status})))
 await assert.rejects(loadLocales(undefined))
})
