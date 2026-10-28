import {expect,test} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {stripTypeScriptTypes} from 'node:module'
import {randomUUID} from 'node:crypto'
import type {CartClient} from '../../storefront/src/lib/cart-client'

declare global {interface Window {itemClient:CartClient;itemCSRF:()=>string|undefined}}
const source=stripTypeScriptTypes(readFileSync(new URL('../../storefront/src/lib/cart-client.ts',import.meta.url),'utf8'))

test('actual storefront client adds and merges draft items through browser cookies and Gin',async({page,context,browser})=>{
 test.setTimeout(60_000)
 const errors:string[]=[]
 page.on('pageerror',e=>errors.push(e.name))
 async function install(){
  await page.goto('/login')
  await page.getByLabel('邮箱',{exact:true}).waitFor()
  await page.evaluate(async source=>{
   const url=URL.createObjectURL(new Blob([source],{type:'text/javascript'}))
   try {
    const module=await import(/* @vite-ignore */ url)
    window.itemClient=module.browserCartClient()
    window.itemCSRF=()=>module.cartCSRFFromCookies(document.cookie,location.protocol==='https:')
   } finally {URL.revokeObjectURL(url)}
  },source)
 }
 await install()
 const storage=await page.evaluate(()=>({local:Object.entries(localStorage),session:Object.entries(sessionStorage)}))
 const created=await page.evaluate(async operationID=>{
  const scope=await window.itemClient.openScope()
  return window.itemClient.create({operationID,regionID:'fixture-region',locale:'en'},scope)
 },randomUUID())
 const id=created.cart.id
 const cookies=await context.cookies()
 const operationID=randomUUID()
 const responsePromise=page.waitForResponse(r=>r.url().endsWith('/line-items'))
 const replays=await page.evaluate(async({id,operationID})=>Promise.all(Array.from({length:4},()=>window.itemClient.addItem(id,{operationID,version:'1',variantID:'browser-add-v',quantity:'1'},{csrfToken:window.itemCSRF()!}))),{id,operationID})
 const response=await responsePromise
 expect(response.status()).toBe(200)
 expect(await response.request().headerValue('origin')).toBe('http://127.0.0.1:19200')
 expect(response.headers()['set-cookie']).toBeUndefined()
 expect(response.headers()['cache-control']).toBe('private, no-store')
 expect(replays.filter(r=>!r.replayed)).toHaveLength(1)
 expect(replays.every(r=>r.cart_id===id&&r.version==='2'&&r.quantity==='1'&&r.line_id===replays[0].line_id)).toBe(true)
 const first=await page.evaluate(id=>window.itemClient.retrieveAmounts(id),id)
 expect(first?.totals.total).toBe('100.000000000000000000001')
 expect(first?.lines[0].quantity).toBe('1')
 const merged=await page.evaluate(async({id,operationID})=>window.itemClient.addItem(id,{operationID,version:'2',variantID:'browser-add-v',quantity:'1'},{csrfToken:window.itemCSRF()!}),{id,operationID:randomUUID()})
 expect(merged).toMatchObject({cart_id:id,line_id:replays[0].line_id,version:'3',quantity:'2',replayed:false})
 const replay=await page.evaluate(async({id,operationID})=>window.itemClient.addItem(id,{operationID,version:'1',variantID:'browser-add-v',quantity:'1'},{csrfToken:window.itemCSRF()!}),{id,operationID})
 expect(replay).toEqual({...replays[0],replayed:true})
 for(const {version,quantity,kind,status} of [{version:'2',quantity:'1',kind:'conflict',status:409},{version:'3',quantity:'3',kind:'item',status:422}]){
  const failed=await page.evaluate(async({id,operationID,version,quantity})=>{
   try {await window.itemClient.addItem(id,{operationID,version,variantID:'browser-add-v',quantity},{csrfToken:window.itemCSRF()!});return null}
   catch(e){const error=e as {kind:string;status:number;uncertain:boolean};return {kind:error.kind,status:error.status,uncertain:error.uncertain}}
  },{id,operationID:randomUUID(),version,quantity})
  expect(failed).toEqual({kind,status,uncertain:false})
 }
 await install()
 const saved=await page.evaluate(id=>window.itemClient.retrieveAmounts(id),id)
 expect(saved).toMatchObject({cart_id:id,version:'3',pricing_recalculated:false,checkout:false,lines:[{id:merged.line_id,quantity:'2',unit_price:'80.000000000000000000001'}]})
 expect(saved?.totals.total).toBe('160.000000000000000000002')
 expect(saved?.lines).toHaveLength(1)
 const currentCookies=await context.cookies()
 expect(currentCookies).toEqual(cookies)
 expect(await page.evaluate(()=>({local:Object.entries(localStorage),session:Object.entries(sessionStorage)}))).toEqual(storage)
 const stranger=await browser.newContext()
 try {
  const denied=await stranger.request.post(`http://127.0.0.1:19200/api/store/carts/${id}/line-items`,{headers:{Origin:'http://127.0.0.1:19200','Idempotency-Key':randomUUID()},data:{version:'3',variant_id:'browser-add-v',quantity:'1'}})
  expect(denied.status()).toBe(401)
 } finally {await stranger.close()}
 expect(errors).toEqual([])
})
