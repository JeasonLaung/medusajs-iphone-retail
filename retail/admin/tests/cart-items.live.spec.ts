import {expect,test} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {stripTypeScriptTypes} from 'node:module'
import {randomUUID} from 'node:crypto'
import type {CartClient} from '../../storefront/src/lib/cart-client'
type ItemCommerce={enabled:false}|{enabled:true;addToCart:(input:{variantId:string;quantity:'1';countryCode:string})=>Promise<{ok:true;quantities:Record<string,string>}|{ok:false;error:'inventory'|'error'}>}

declare global {interface Window {itemClient:CartClient;itemCSRF:()=>string|undefined;itemCommerce:ItemCommerce}}
const commerceSource=stripTypeScriptTypes(readFileSync(new URL('../../storefront/src/lib/cart-commerce.ts',import.meta.url),'utf8'))
const quantitiesSource=stripTypeScriptTypes(readFileSync(new URL('../../storefront/src/lib/cart-quantities.ts',import.meta.url),'utf8'))
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
 async function installCommerce(loseResponse=false){
  await page.evaluate(async({clientSource,commerceSource,quantitiesSource,loseResponse})=>{
   const urls:string[]=[]
   const blob=(text:string)=>{const url=URL.createObjectURL(new Blob([text],{type:'text/javascript'}));urls.push(url);return url}
   let drop=loseResponse
   const fetcher=window.fetch.bind(window)
   window.fetch=async(...args)=>{
    const response=await fetcher(...args)
    if(drop&&String(args[0]).endsWith('/line-items')){drop=false;await response.arrayBuffer();throw Error('injected lost committed response')}
    return response
   }
   try{
    const clientURL=blob(clientSource),quantitiesURL=blob(quantitiesSource)
    const url=blob(commerceSource.replace("'./cart-client.ts'",JSON.stringify(clientURL)).replace("'./cart-quantities.ts'",JSON.stringify(quantitiesURL)))
    const {browserCartCommerce}=await import(/* @vite-ignore */ url)
    window.itemCommerce=browserCartCommerce({enabled:true,regionID:'fixture-region',currency:'eur',segment:'dk-en'})
   }finally{urls.forEach(url=>URL.revokeObjectURL(url))}
  },{clientSource:source,commerceSource,quantitiesSource,loseResponse})
 }
 await install()
 expect(await page.evaluate(()=>window.itemClient.current())).toBeNull()
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
 // Resolve only from the HttpOnly cookie after module reload, not a saved ID.
 const restored=await page.evaluate(()=>window.itemClient.current())
 expect(restored).toMatchObject({id,version:'3',region_id:'fixture-region'})
 const saved=await page.evaluate(async()=>{
  const cart=await window.itemClient.current()
  return cart ? window.itemClient.retrieveAmounts(cart.id) : null
 })
 expect(saved).toMatchObject({cart_id:id,version:'3',pricing_recalculated:false,checkout:false,lines:[{id:merged.line_id,quantity:'2',unit_price:'80.000000000000000000001'}]})
 expect(saved?.totals.total).toBe('160.000000000000000000002')
 expect(saved?.lines).toHaveLength(1)
 const currentCookies=await context.cookies()
 expect(currentCookies).toEqual(cookies)
 expect(await page.evaluate(()=>({local:Object.entries(localStorage),session:Object.entries(sessionStorage)}))).toEqual(storage)
 // Inject a transport failure only AFTER a real Gin addition responded.
 // Reload then reconstruct the coordinator from its noncredential journal.
 await installCommerce(true)
 expect(await page.evaluate(async()=>{if(!window.itemCommerce.enabled)throw Error('Commerce must be enabled');return window.itemCommerce.addToCart({variantId:'browser-add-v',quantity:'1',countryCode:'dk-en'})})).toEqual({ok:false,error:'error'})
 const pending=await page.evaluate(()=>JSON.parse(localStorage.getItem('retail.cart.intent.v1')!))
 expect(pending).toMatchObject({kind:'add',cartID:id,version:'3',variantID:'browser-add-v'})
 expect(Object.keys(pending).sort()).toEqual(['cartID','format','kind','operationID','quantity','regionID','segment','variantID','version'])
 await install()
 await installCommerce()
 const recovered=await page.evaluate(async()=>{if(!window.itemCommerce.enabled)throw Error('Commerce must be enabled');return window.itemCommerce.addToCart({variantId:'browser-add-v',quantity:'1',countryCode:'dk-en'})})
 expect(recovered).toEqual({ok:true,quantities:{'browser-add-v':'3'}})
 expect(await page.evaluate(()=>window.itemClient.current())).toMatchObject({id,version:'4'})
 expect(await page.evaluate(()=>localStorage.getItem('retail.cart.intent.v1'))).toBeNull()
 expect(await page.evaluate(()=>({local:Object.entries(localStorage),session:Object.entries(sessionStorage)}))).toEqual(storage)
 const savedView=await page.evaluate(id=>window.itemClient.retrieveView(id),id)
 expect(savedView?.amounts).toMatchObject({cart_id:id,version:'4',checkout:false,pricing_recalculated:false})
 expect(savedView?.items).toHaveLength(1)
 expect(savedView?.items[0]).toMatchObject({id:merged.line_id})
 expect(savedView?.items[0].title).not.toBe('')
 expect(savedView?.items[0].product_title).not.toBeNull()
 expect(savedView?.items[0].option_values).toEqual({Storage:'128 GB'})
 const setKey=randomUUID()
 const changed=await page.evaluate(async({id,lineID,operationID})=>window.itemClient.setItem(id,lineID,{operationID,version:'4',quantity:'1'},{csrfToken:window.itemCSRF()!}),{id,lineID:merged.line_id,operationID:setKey})
 expect(changed).toMatchObject({cart_id:id,line_id:merged.line_id,version:'5',quantity:'1',replayed:false})
 const repriced=await page.evaluate(id=>window.itemClient.retrieveAmounts(id),id)
 expect(repriced?.totals.total).toBe('100.000000000000000000001')
 const removeKey=randomUUID()
 for(const replayed of [false,true]){
  const removed=await page.evaluate(async({id,lineID,operationID})=>window.itemClient.setItem(id,lineID,{operationID,version:'5',quantity:'0'},{csrfToken:window.itemCSRF()!}),{id,lineID:merged.line_id,operationID:removeKey})
  expect(removed).toMatchObject({cart_id:id,line_id:merged.line_id,version:'6',quantity:'0',replayed})
 }
 const emptyCart=await page.evaluate(id=>window.itemClient.retrieveAmounts(id),id)
 expect(emptyCart).toMatchObject({version:'6',lines:[]})
 expect(emptyCart?.totals.total).toBe('0')
 expect(await page.evaluate(id=>window.itemClient.retrieveView(id),id)).toMatchObject({amounts:{version:'6',lines:[]},items:[]})
 const stranger=await browser.newContext()
 try {
  const denied=await stranger.request.post(`http://127.0.0.1:19200/api/store/carts/${id}/line-items`,{headers:{Origin:'http://127.0.0.1:19200','Idempotency-Key':randomUUID()},data:{version:'3',variant_id:'browser-add-v',quantity:'1'}})
  expect(denied.status()).toBe(401)
 } finally {await stranger.close()}
 await context.clearCookies({name:'retail_cart_possession'})
 expect(await page.evaluate(()=>window.itemClient.current())).toBeNull()
 expect(errors).toEqual([])
})
