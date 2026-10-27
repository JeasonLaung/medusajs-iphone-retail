import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

const user = { id: 'artificial-admin', email: 'fixture@example.invalid', name: 'Fixture Admin', role: 'admin', enabled: true }
const challenge = 'a'.repeat(64)
const sessionCSRF = 'b'.repeat(64)
const runtimeErrors=new WeakMap<Page,string[]>()
test.beforeEach(({page})=>{
  const errors:string[]=[];runtimeErrors.set(page,errors)
  page.on('pageerror',error=>errors.push(error.name))
  page.on('console',message=>{if(message.text().includes('[antd: compatible]'))errors.push('AntReactCompatibility')})
})
test.afterEach(({page})=>{expect(runtimeErrors.get(page)).toEqual([])})

async function fixture(page: Page, options: { failMe?: boolean; invalidLogin?: boolean; failLogout?: boolean } = {}) {
  let signedIn = false
  const calls = { challenge: 0, login: 0, logout: 0 }
  await page.route('**/api/admin/auth/**', async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const error = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'artificial fallback', request_id: 'req_12345678-1234-1234-1234-123456789abc' } } })
    if (path.endsWith('/me')) {
      if (options.failMe) return error(503, 'auth.unavailable')
      return signedIn ? route.fulfill({ json: { user } }) : error(401, 'auth.unauthorized')
    }
    if (path.endsWith('/challenge')) {
      calls.challenge++
      expect(request.method()).toBe('POST')
      return route.fulfill({ json: { csrf_token: challenge } })
    }
    if (path.endsWith('/login')) {
      calls.login++
      expect(request.headers()['x-csrf-token']).toBe(challenge)
      expect(request.postDataJSON()).toEqual({ email: user.email, password: 'artificial-password' })
      if (options.invalidLogin) return error(401, 'auth.invalid_credentials')
      signedIn = true
      return route.fulfill({ json: { user, csrf_token: sessionCSRF }, headers: { 'Set-Cookie': `retail_admin_csrf=${sessionCSRF}; Path=/; SameSite=Strict` } })
    }
    if (path.endsWith('/logout')) {
      calls.logout++
      expect(request.headers()['x-csrf-token']).toBe(sessionCSRF)
      if (options.failLogout) return error(503, 'auth.unavailable')
      signedIn = false
      return route.fulfill({ json: { success: true } })
    }
    return error(404, 'not_found')
  })
  return calls
}

async function signIn(page: Page) {
  await page.getByLabel('邮箱', { exact: true }).fill(user.email)
  await page.getByLabel('密码', { exact: true }).fill('artificial-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
}

test('detail dependency failure retries and a missing product stays distinct',async({page})=>{
  await fixture(page)
  let mode:'failed'|'ready'|'missing'='failed'
  const product={id:'artificial-product',title:'Text <img src=x onerror=alert(1)>',handle:'artificial-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:0,subtitle:null,description:'<script>window.unwanted=true</script>',options:[],images:[]}
  await page.route('**/api/admin/products/artificial-product**',route=>{
    if(mode==='failed')return route.fulfill({status:503,json:{error:{code:'catalog.unavailable'}}})
    if(mode==='missing')return route.fulfill({status:404,json:{error:{code:'catalog.not_found'}}})
    return route.fulfill({json:new URL(route.request().url()).pathname.endsWith('/variants')?{variants:[],count:0,limit:20,offset:0}:{product}})
  })
  await page.goto('/account');await signIn(page)
  await page.goto('/products/artificial-product')
  await expect(page.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
  mode='ready'
  await page.getByRole('button',{name:'重新连接',exact:true}).click()
  await expect(page.getByRole('heading',{name:product.title,exact:true})).toBeVisible()
  await expect(page.getByText(product.description,{exact:true})).toBeVisible()
  await expect(page.getByText('暂无变体',{exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Object.hasOwn(window,'unwanted'))).toBe(false)
  mode='missing';await page.reload()
  await expect(page.getByText('商品不存在或已删除',{exact:true})).toBeVisible()
})

test('product dependency failure is distinct from empty state and supports retry and expiry', async ({page})=>{
  await fixture(page)
  let mode:'failed'|'ready'|'expired'='failed'
  await page.route('**/api/admin/products?*',route=>{
    if(mode==='failed')return route.fulfill({status:503,json:{error:{code:'catalog.unavailable'}}})
    if(mode==='expired')return route.fulfill({status:401,json:{error:{code:'auth.unauthorized'}}})
    return route.fulfill({json:{products:[],count:0,limit:20,offset:0}})
  })
  await page.goto('/account')
  await signIn(page)
  await page.getByRole('menuitem',{name:'商品',exact:true}).click()
  await expect(page.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
  await expect(page.getByText('没有符合条件的商品',{exact:true})).toHaveCount(0)
  mode='ready'
  await page.getByRole('button',{name:'重新连接',exact:true}).click()
  await expect(page.getByText('没有符合条件的商品',{exact:true})).toBeVisible()
  mode='expired'
  await page.getByRole('button',{name:'重置',exact:true}).click()
  // Change a filter to issue a new request if reset leaves the same URL.
  await page.getByRole('textbox',{name:/^搜索/}).fill('expired')
  await page.getByRole('button',{name:'搜索',exact:true}).click()
  await expect(page).toHaveURL('/login')
  await expect(page.getByText('登录已过期，请重新登录。',{exact:true})).toBeVisible()
})

test('Ant form signs in, reloads account, and signs out without storing capabilities', async ({ page }) => {
  const calls = await fixture(page)
  await page.goto('/account')
  await expect(page).toHaveURL('/login')
  await signIn(page)
  await expect(page).toHaveURL('/account')
  await expect(page.getByText(user.email, { exact: true })).toBeVisible()
  expect(calls.challenge).toBe(1); expect(calls.login).toBe(1)
  await page.reload()
  await expect(page.getByText(user.email, { exact: true })).toBeVisible()
  const stored = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))
  expect(stored).not.toContain(sessionCSRF)
  expect(stored).not.toContain('artificial-password')
  expect(stored).not.toContain(user.email)
  await page.getByRole('button', { name: '退出登录', exact: true }).click()
  await expect(page).toHaveURL('/login')
  expect(calls.logout).toBe(1)
})

test('invalid credentials reset password and each retry gets a new challenge', async ({ page }) => {
  const calls = await fixture(page, { invalidLogin: true })
  await page.goto('/login')
  await signIn(page)
  await expect(page.getByText('邮箱或密码不正确。', { exact: true })).toBeVisible()
  await expect(page.getByLabel('密码', { exact: true })).toHaveValue('')
  await page.getByLabel('密码', { exact: true }).fill('artificial-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect.poll(() => calls.login).toBe(2)
  expect(calls.challenge).toBe(2)
})

test('dependency outage does not become anonymous or an empty account', async ({ page }) => {
  await fixture(page, { failMe: true })
  await page.goto('/account')
  await expect(page.getByText('暂时无法连接服务，请稍后重试。', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '重新连接' })).toBeVisible()
  await expect(page.getByLabel('密码', { exact: true })).toHaveCount(0)
})

test('failed logout keeps account and allows explicit retry', async ({ page }) => {
  const options={ failLogout: true }
  const calls = await fixture(page, options)
  await page.goto('/login'); await signIn(page)
  await page.getByRole('button', { name: '退出登录', exact: true }).click()
  await expect(page.getByText('暂时无法连接服务，请稍后重试。', { exact: true })).toBeVisible()
  await expect(page.getByText(user.email, { exact: true })).toBeVisible()
  await expect(page).toHaveURL('/account')
  expect(calls.logout).toBe(1)
  options.failLogout=false
  await page.getByRole('button', { name: '退出登录', exact: true }).click()
  await expect(page).toHaveURL('/login')
  expect(calls.logout).toBe(2)
})

for (const width of [390, 768, 1440]) {
  test(`Chinese/English form and account fit ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await fixture(page)
    await page.goto('/login')
    await expect(page.getByRole('heading', { name: '登录管理后台' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `../../logs/gin-migration/admin-ui/login-zh-${width}.png` })
    await signIn(page)
    await expect(page.getByRole('heading', { name: '我的账号' })).toBeVisible()
    await test.step('open language selector with keyboard and choose English', async () => {
      const selector=page.getByRole('combobox', { name: '语言' })
      await selector.focus()
      await selector.press('Enter')
      await expect(page.getByRole('option',{name:'English',exact:true})).toBeVisible()
      await page.getByRole('option',{name:'English',exact:true}).click()
    })
    await expect(page.getByRole('heading', { name: 'My account' })).toBeVisible()
    await page.keyboard.press('Escape')
    if (width === 390) {
      await page.getByRole('button', { name: 'Open navigation' }).click()
      await expect(page.getByRole('dialog')).toBeVisible()
      await page.getByRole('dialog').getByRole('menuitem', { name: 'My account' }).click()
      await expect(page.getByRole('dialog')).toHaveCount(0)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `../../logs/gin-migration/admin-ui/account-en-${width}.png` })
    await page.getByRole('button', { name: 'Sign out', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Sign in to administration' })).toBeVisible()
    await page.screenshot({ path: `../../logs/gin-migration/admin-ui/login-en-${width}.png` })
  })
}
