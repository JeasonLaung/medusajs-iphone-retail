import {publicJSON} from './gin-public.ts'
import type {Locale} from './preferences.ts'

// Empty means not configured. An unavailable/invalid directory stays an error;
// the existing header alone adds its en/zh presentation fallbacks.
export async function loadLocales(origin:string|undefined,fetcher:typeof fetch=fetch):Promise<Locale[]>{
 const input=await publicJSON(origin,'/api/store/locales',128*1024,fetcher)
 const fail=()=>new Error('Store locales unavailable')
 if(!input||typeof input!=='object'||Array.isArray(input))throw fail()
 const body=input as Record<string,unknown>
 if(!Array.isArray(body.locales)||body.locales.length>100||body.count!==body.locales.length)throw fail()
 const codes=new Set<string>()
 return body.locales.map(value=>{
  if(!value||typeof value!=='object'||Array.isArray(value))throw fail()
  const {code,name}=value as Record<string,unknown>
  if(typeof code!=='string'||code.length>35||! /^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$/.test(code)||/[\r\n]/.test(code)||typeof name!=='string'||!name.trim()||Array.from(name).length>256||/[\x00-\x1f\x7f]/.test(name)||codes.has(code.toLowerCase()))throw fail()
  codes.add(code.toLowerCase());return {code,name}
 })
}
