import test from 'node:test'
import assert from 'node:assert/strict'
import {routePreferenceCookie,savePreference} from '../src/lib/preference-save.ts'
import type {PreferenceTarget} from '../src/lib/preferences.ts'
const target:PreferenceTarget={country:'de',locale:'zh-CN',regionID:'eu',currency:'eur',segment:'de-zh',currentPath:'/about?next=a%2Bb#contact',location:'/de-zh/about?next=a%2Bb#contact'}
test('preference save confirms cart before cookie and exact navigation; failures never navigate',async()=>{
 const calls:string[]=[]
 const options={apply:async()=>{calls.push('cart')},writeLocale:(v:string)=>{calls.push('cookie:'+v)},navigate:(v:string)=>{calls.push(v)}}
 await savePreference(target,options)
 assert.deepEqual(calls,['cart','cookie:zh-CN',target.location])
 calls.length=0
 await assert.rejects(savePreference(target,{...options,apply:async()=>{calls.push('cart');throw Error('uncertain')}}));assert.deepEqual(calls,['cart'])
 calls.length=0
 await assert.rejects(savePreference(target,{...options,writeLocale:()=>{calls.push('cookie');throw Error('blocked')}}));assert.deepEqual(calls,['cart','cookie'])
})
test('URL presentation language wins without destroying same-language preference variants',()=>{
 for(const [segment,current,want] of [['dk-en',undefined,undefined],['dk-zh',undefined,'zh-CN'],['dk-en','zh-CN','en'],['dk-zh','en-GB','zh-CN'],['dk-en','en-GB',undefined],['dk-en','da',undefined],['dk-zh','zh-Hans',undefined],['dk-en','en\n','en']] as const)assert.equal(routePreferenceCookie(segment,current),want)
 assert.equal(routePreferenceCookie('foreign','zh-CN'),undefined)
})
test('preference finalization rejects external or malformed navigation before any cart change',async()=>{
 for(const location of ['//foreign.example','https://foreign.example','/de-zh\\evil','/de-zh/about\n','/de-zhwrong/about'])await assert.rejects(savePreference({...target,location},{apply:async()=>{assert.fail('must not mutate')},writeLocale:()=>{},navigate:()=>{}}))
})
