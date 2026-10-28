import {expect, test} from '@playwright/test'
import type {Page} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {stripTypeScriptTypes} from 'node:module'
import {randomUUID} from 'node:crypto'
import type {CartClient, CartScope} from '../../storefront/src/lib/cart-client'

declare global {
 interface Window {
  ownedCartClient: CartClient
  ownedCartScope: CartScope
 }
}

// Run the unchanged storefront transport in a real browser. The Vite proxy
// forwards all API requests to the actual cmd/api and owned MySQL/Redis.
// No route interception, test HTTP routes, fake responses or auth bypass.
const source = stripTypeScriptTypes(readFileSync(new URL('../../storefront/src/lib/cart-client.ts', import.meta.url), 'utf8'))
async function install(page: Page) {
 await page.goto('/login')
 await page.getByLabel('邮箱', {exact:true}).waitFor()
 await page.evaluate(async source => {
  const url = URL.createObjectURL(new Blob([source], {type:'text/javascript'}))
  try {
   const {browserCartClient} = await import(/* @vite-ignore */ url)
   window.ownedCartClient = browserCartClient()
  } finally { URL.revokeObjectURL(url) }
 }, source)
}

test('storefront cart client through the real browser, Gin, MySQL and Redis', async ({page, context, browser}) => {
 test.setTimeout(60_000)
 const errors:string[] = []
 page.on('pageerror', error => errors.push(error.name))
 await install(page)
 const storage = await page.evaluate(() => ({local:Object.entries(localStorage), session:Object.entries(sessionStorage)}))
 expect(await context.cookies()).toEqual([])
 const scopeResponse = page.waitForResponse(response => response.url().endsWith('/api/store/cart-sessions'))
 await page.evaluate(async () => { window.ownedCartScope = await window.ownedCartClient.openScope() })
 const issuedScope = await scopeResponse
 expect(issuedScope.status()).toBe(200)
 expect(await issuedScope.request().headerValue('origin')).toBe('http://127.0.0.1:19200')
 expect(issuedScope.headers()['cache-control']).toBe('private, no-store')
 expect(issuedScope.headers()['x-request-id']).toMatch(/^req_/)
 const scopeCookies = await context.cookies()
 expect(scopeCookies.map(cookie => cookie.name).sort()).toEqual(['retail_cart_scope', 'retail_cart_scope_csrf'])
 for (const cookie of scopeCookies) {
  expect(cookie.httpOnly).toBe(cookie.name === 'retail_cart_scope')
  expect(cookie.sameSite).toBe('Lax'); expect(cookie.path).toBe('/'); expect(cookie.secure).toBe(false)
 }
 expect(await page.evaluate(() => document.cookie.split(';').map(value => value.trim().split('=')[0]))).toEqual(['retail_cart_scope_csrf'])
 const operationID = randomUUID()
 const create = () => page.evaluate(operationID => window.ownedCartClient.create({operationID, regionID:'fixture-region', locale:'en'}, window.ownedCartScope), operationID)
 const created = await create()
 expect(created.replayed).toBe(false)
 expect(created.cart).toEqual({id:expect.stringMatching(/^cart_/), region_id:'fixture-region', sales_channel_id:'fixture-channel', currency_code:'eur', locale:'en', version:'1'})
 const id = created.cart.id
 const cookies = await context.cookies()
 expect(cookies.map(cookie => cookie.name).sort()).toEqual(['retail_cart_csrf', 'retail_cart_possession', 'retail_cart_scope', 'retail_cart_scope_csrf'])
 expect(cookies.find(cookie => cookie.name === 'retail_cart_possession')?.httpOnly).toBe(true)
 const visibleNames = await page.evaluate(() => document.cookie.split(';').map(value => value.trim().split('=')[0]).sort())
 expect(visibleNames).toEqual(['retail_cart_csrf', 'retail_cart_scope_csrf'])
 expect(await page.evaluate(id => window.ownedCartClient.retrieve(id), id)).toEqual(created.cart)
 const persisted = await page.evaluate(id => window.ownedCartClient.retrieveAmounts(id), id)
 expect(persisted).toMatchObject({cart_id:id,version:'1',currency_code:'eur',lines:[],shipping_methods:[],pricing_recalculated:false,checkout:false})
 expect(persisted?.totals.total).toBe('0')
 expect(Object.values(persisted!.totals).every(value => typeof value === 'string')).toBe(true)

 // Same scope + operation across concurrent real requests is one SQL receipt.
 const replays = await page.evaluate(async operationID => Promise.all(Array.from({length:4}, () => window.ownedCartClient.create({operationID, regionID:'fixture-region', locale:'en'}, window.ownedCartScope))), operationID)
 expect(replays.every(result => result.replayed && result.cart.id === id)).toBe(true)
 const conflict = await page.evaluate(async operationID => {
  try { await window.ownedCartClient.create({operationID, regionID:'fixture-region', locale:'zh-CN'}, window.ownedCartScope); return null }
  catch (error) { const e = error as {kind:string;status:number}; return {kind:e.kind,status:e.status} }
 }, operationID)
 expect(conflict).toEqual({kind:'conflict',status:409})
 const reusedResponse = page.waitForResponse(response => response.url().endsWith('/api/store/cart-sessions'))
 await page.evaluate(async () => { window.ownedCartScope = await window.ownedCartClient.openScope() })
 expect((await reusedResponse).headers()['set-cookie']).toBeUndefined()
 const scopeAfterReuse = await context.cookies()
 for (const old of scopeCookies) expect(scopeAfterReuse.find(cookie => cookie.name === old.name)?.expires).toBe(old.expires)

 // A new tab/reload uses the browser's HttpOnly possession, never localStorage.
 await install(page)
 expect(await page.evaluate(id => window.ownedCartClient.retrieve(id), id)).toEqual(created.cart)
 expect(await page.evaluate(() => ({local:Object.entries(localStorage), session:Object.entries(sessionStorage)}))).toEqual(storage)
 // Cart possession cannot act as an Admin login.
 expect((await context.request.get('/api/admin/auth/me')).status()).toBe(401)
 const stranger = await browser.newContext()
 try {
  const strangerPage = await stranger.newPage()
  await install(strangerPage)
  expect(await strangerPage.evaluate(id => window.ownedCartClient.retrieve(id), id)).toBeNull()
  expect(await strangerPage.evaluate(id => window.ownedCartClient.retrieveAmounts(id), id)).toBeNull()
 } finally { await stranger.close() }

 await context.clearCookies({name:'retail_cart_possession'})
 expect(await page.evaluate(id => window.ownedCartClient.retrieve(id), id)).toBeNull()
 expect(await page.evaluate(id => window.ownedCartClient.retrieveAmounts(id), id)).toBeNull()
 await page.evaluate(async () => { window.ownedCartScope = await window.ownedCartClient.openScope() })
 const recovered = await create()
 expect(recovered.replayed).toBe(true); expect(recovered.cart.id).toBe(id)
 expect(await page.evaluate(id => window.ownedCartClient.retrieve(id), id)).toEqual(created.cart)
 const foreign = await context.request.post('/api/store/carts', {data:{region_id:'fixture-region'}, headers:{Origin:'https://unrelated.example','Idempotency-Key':randomUUID(),'X-CSRF-Token':created.csrfToken}})
 expect(foreign.status()).toBe(403)
 expect(errors).toEqual([])
})
