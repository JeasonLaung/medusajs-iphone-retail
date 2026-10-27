import { expect, test } from '@playwright/test'

// Fixed artificial identity is created only in the owned throwaway database.
// No route interception or mocked API responses in this file.
test('real browser cookie, Origin and CSRF flow through Vite to Gin and Redis', async ({ page, context }) => {
  const failures:string[]=[]
  page.on('pageerror',error=>failures.push(error.name))
  const signIn=async(password:string)=>{
    await page.getByLabel('邮箱',{exact:true}).fill('browser@example.invalid')
    await page.getByLabel('密码',{exact:true}).fill(password)
    await page.getByRole('button',{name:'登录',exact:true}).click()
  }
  await page.goto('/account')
  await expect(page).toHaveURL('/login')
  await signIn('wrong-artificial-password')
  await expect(page.getByText('邮箱或密码不正确。',{exact:true})).toBeVisible()
  await expect(page.getByLabel('密码',{exact:true})).toHaveValue('')
  await signIn('artificial-browser-password')
  await expect(page).toHaveURL('/account')
  await expect(page.getByText('browser@example.invalid',{exact:true})).toBeVisible()
  const cookies=await context.cookies()
  const session=cookies.find(c=>c.name==='retail_admin_session')
  const csrf=cookies.find(c=>c.name==='retail_admin_csrf')
  expect(session?.httpOnly).toBe(true)
  expect(session?.sameSite).toBe('Strict')
  expect(session?.secure).toBe(false) // This fixture is explicit loopback HTTP.
  expect(csrf?.httpOnly).toBe(false)
  expect(csrf?.value).toMatch(/^[a-f0-9]{64}$/)
  expect(cookies.find(c=>c.name==='retail_admin_prelogin')).toBeUndefined()
  const readable=await page.evaluate(()=>({cookies:document.cookie,local:{...localStorage},session:{...sessionStorage}}))
  expect(readable.cookies).not.toContain('retail_admin_session=')
  expect(JSON.stringify({local:readable.local,session:readable.session})).not.toContain('browser@example.invalid')
  expect(JSON.stringify(readable.local)).not.toContain(csrf!.value)
  await page.reload()
  await expect(page.getByText('browser@example.invalid',{exact:true})).toBeVisible()
  const me=await context.request.get('/api/admin/auth/me')
  expect(me.status()).toBe(200)
  expect(me.headers()['cache-control']).toBe('private, no-store')
  expect(me.headers()['x-request-id']).toMatch(/^req_/)
  const noCSRF=await context.request.post('/api/admin/auth/logout',{headers:{Origin:'http://127.0.0.1:19200'}})
  expect(noCSRF.status()).toBe(403)
  const wrongOrigin=await context.request.post('/api/admin/auth/logout',{headers:{Origin:'https://evil.example','X-CSRF-Token':csrf!.value}})
  expect(wrongOrigin.status()).toBe(403)
  expect((await context.request.get('/api/admin/auth/me')).status()).toBe(200)
  await page.getByRole('button',{name:'退出登录',exact:true}).click()
  await expect(page).toHaveURL('/login')
  expect((await context.cookies()).find(c=>c.name==='retail_admin_session')).toBeUndefined()
  // Captured old cookie remains invalid even when deliberately supplied.
  const replay=await context.request.get('/api/admin/auth/me',{headers:{Cookie:`retail_admin_session=${session!.value}`}})
  expect(replay.status()).toBe(401)
  await page.reload()
  await expect(page.getByRole('heading',{name:'登录管理后台'})).toBeVisible()
  expect(failures).toEqual([])
})
