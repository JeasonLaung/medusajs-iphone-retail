import { expect, test } from '@playwright/test'
import {ACTIONS} from '../src/api'
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

async function fixture(page: Page, options: { failMe?: boolean; invalidLogin?: boolean; failLogout?: boolean;permissions?: string[];failPermissions?:boolean } = {}) {
  let signedIn = false
  const calls = { challenge: 0, login: 0, logout: 0 }
  await page.route('**/api/admin/auth/**', async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const error = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'artificial fallback', request_id: 'req_12345678-1234-1234-1234-123456789abc' } } })
    if(path.endsWith('/permissions')) {if(options.failPermissions)return error(503,'authorization.unavailable');return signedIn?route.fulfill({json:{revision:'1',scope:'employee',actions:options.permissions??ACTIONS}}):error(401,'auth.unauthorized')}
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

test('Ant permission role save retries the exact uncertain command and reloads on conflict',async({page})=>{
 await fixture(page)
 let revision='1',fail=true,conflict=false
 const role={id:'reader',name:'Reader',enabled:true,actions:['product.read']}
 const commands:unknown[]=[]
 await page.route('**/api/admin/authorization**',route=>{
  if(route.request().method()==='GET')return route.fulfill({json:{revision,actions:ACTIONS,roles:[role],members:[{user_id:user.id,name:user.name,enabled:true,kind:'employee',roles:['reader']}]}})
  expect(route.request().headers()['x-csrf-token']).toBe(sessionCSRF)
  const c=route.request().postDataJSON();commands.push(c)
  if(conflict)return route.fulfill({status:409,json:{error:{code:'authorization.conflict'}}})
  if(fail)return route.fulfill({status:503,json:{error:{code:'authorization.unavailable'}}})
  role.name=c.role.name;revision='2';return route.fulfill({json:{revision,replayed:true}})
 })
 await page.goto('/account');await signIn(page);await page.getByRole('menuitem',{name:'权限管理',exact:true}).click()
 await page.getByRole('button',{name:'Reader',exact:true}).click()
 const drawer=page.getByRole('dialog')
 await drawer.getByLabel('姓名',{exact:true}).fill('Reviewed role')
 await drawer.getByRole('checkbox',{name:'查看订单',exact:true}).click()
 await expect(drawer.getByRole('checkbox',{name:'查看订单',exact:true})).toBeChecked()
 await drawer.getByRole('button',{name:'保存',exact:true}).click()
 await expect(drawer.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible()
 await expect(drawer.getByLabel('姓名',{exact:true})).toBeDisabled()
 fail=false;await drawer.getByRole('button',{name:'重试本次保存',exact:true}).click()
 await expect(page.getByRole('button',{name:'Reviewed role',exact:true})).toBeVisible()
 expect(commands).toHaveLength(2);expect(commands[1]).toEqual(commands[0])
 expect(commands[0]).toMatchObject({role:{actions:expect.arrayContaining(['order.read','product.read'])}})
 conflict=true;await page.getByRole('button',{name:'Reviewed role',exact:true}).click()
 await drawer.getByLabel('姓名',{exact:true}).fill('Stale role')
 await drawer.getByRole('button',{name:'保存',exact:true}).click()
 await expect(drawer.getByText('权限已变化，请关闭并重新加载后编辑。',{exact:true})).toBeVisible()
 await expect(drawer.getByRole('button',{name:'重试本次保存',exact:true})).toHaveCount(0)
 await drawer.getByRole('button',{name:'取消',exact:true}).click()
 await expect(page.getByRole('button',{name:'Reviewed role',exact:true})).toBeVisible()
})

test('permission directory respects read-only grants and rejects a malformed action dictionary',async({page})=>{
 await fixture(page,{permissions:['authorization.read']})
 let invalid=false
 await page.route('**/api/admin/authorization',route=>route.fulfill({json:{revision:'1',actions:invalid?['*']:ACTIONS,roles:[{id:'reader',name:'Reader',enabled:true,actions:['product.read']}],members:[]}}))
 await page.goto('/account');await signIn(page);await page.goto('/authorization')
 await expect(page.getByText('Reader',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'新建角色',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Reader',exact:true})).toHaveCount(0)
 invalid=true;await page.reload()
 await expect(page.getByRole('alert')).toBeVisible()
 await expect(page.getByText('Reader',{exact:true})).toHaveCount(0)
})

test('order amounts retry separately and reject a different order version',async({page})=>{
 await fixture(page)
 const order={id:'artificial-order',version:2,status:'pending',currency_code:'eur',custom_display_id:'Order fixture',display_id:'1',is_draft_order:false,email:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',line_count:0}
 let mode:'failed'|'ready'|'changed'='failed'
 await page.route('**/api/admin/orders/artificial-order**',route=>{
  if(new URL(route.request().url()).pathname.endsWith('/fulfillments'))return route.fulfill({json:{order_id:order.id,version:2,operations_enabled:false,fulfillments:[]}})
  if(new URL(route.request().url()).pathname.endsWith('/payments'))return route.fulfill({json:{order_id:order.id,version:2,operations_enabled:false,collections:[]}})
  if(!new URL(route.request().url()).pathname.endsWith('/amounts'))return route.fulfill({json:{order,lines:[]}})
  if(mode==='failed')return route.fulfill({status:503,json:{error:{code:'orders.amounts_unavailable'}}})
  return route.fulfill({json:{order_id:order.id,version:mode==='changed'?3:2,currency_code:'eur',totals:{item_subtotal:'0',shipping_subtotal:'0',discount_subtotal:'0',tax_total:'0',credit_line_total:'0',total:'0'},lines:[],shipping_methods:[],summary:null}})
 })
 await page.goto('/account');await signIn(page);await page.goto('/orders/artificial-order')
 await expect(page.getByRole('heading',{name:'Order fixture',exact:true})).toBeVisible()
 const amounts=page.getByRole('region',{name:'订单金额',exact:true})
 await expect(amounts.getByRole('alert')).toBeVisible()
 mode='ready';await amounts.getByRole('button',{name:'重新连接',exact:true}).click()
 await expect(amounts.getByText('当前合计',{exact:true})).toBeVisible()
 await expect(amounts.getByText('记录交易额',{exact:true})).toHaveCount(0)
 mode='changed';await page.reload()
 await expect(amounts.getByText('订单版本已变化，请刷新详情后重试。',{exact:true})).toBeVisible()
 await expect(amounts.getByText('当前合计',{exact:true})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:'Order fixture',exact:true})).toBeVisible()
})

test('detail dependency failure retries and a missing product stays distinct',async({page})=>{
  await fixture(page)
  let mode:'failed'|'ready'|'missing'='failed'
  const product={id:'artificial-product',revision:'a'.repeat(64),title:'Text <img src=x onerror=alert(1)>',handle:'artificial-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:0,subtitle:null,description:'<script>window.unwanted=true</script>',options:[],images:[]}
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

test('uncertain product save keeps one immutable attempt until explicit retry',async({page})=>{
  await fixture(page)
  const product={id:'edit-product',revision:'a'.repeat(64),title:'Original',handle:'edit-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:0,subtitle:'',description:null,options:[],images:[]}
  const attempts:unknown[]=[]
  await page.route('**/api/admin/products/edit-product**',route=>{
    const req=route.request()
    if(req.method()==='PATCH'){
      expect(req.headers()['x-csrf-token']).toBe(sessionCSRF)
      const body=req.postDataJSON();attempts.push(body)
      if(attempts.length===1)return route.fulfill({status:503,json:{error:{code:'catalog.unavailable'}}})
      expect(body).toEqual(attempts[0])
      expect(body.subtitle).toBe('')
      expect(body.description).toBe(null)
      product.title=body.title;product.revision='b'.repeat(64)
      return route.fulfill({json:{revision:product.revision,replayed:true}})
    }
    return route.fulfill({json:new URL(req.url()).pathname.endsWith('/variants')?{variants:[],count:0,limit:20,offset:0}:{product}})
  })
  await page.goto('/account');await signIn(page)
  await page.goto('/products/edit-product')
  await page.getByRole('button',{name:'编辑基本信息',exact:true}).click()
  const drawer=page.getByRole('dialog')
  await drawer.getByLabel('商品名称',{exact:true}).fill('Updated')
  await drawer.getByRole('button',{name:'保存',exact:true}).click()
  await expect(drawer.getByText(/暂未确认保存结果/)).toBeVisible()
  await expect(drawer.getByLabel('商品名称',{exact:true})).toBeDisabled()
  expect(attempts).toHaveLength(1)
  await drawer.getByRole('button',{name:'重试本次保存',exact:true}).click()
  await expect(drawer).toHaveCount(0)
  await expect(page.getByRole('heading',{name:'Updated',exact:true})).toBeVisible()
  expect(attempts).toHaveLength(2)
})

test('price drawer retries, preserves exact strings and distinguishes empty and malformed data',async({page})=>{
  await fixture(page)
  const product={id:'price-product',revision:'a'.repeat(64),title:'Price fixture',handle:'price-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:2,subtitle:null,description:null,options:[],images:[]}
  const variants=['priced','empty'].map(id=>({id,product_id:product.id,title:id,sku:id,thumbnail:null,variant_rank:0,manage_inventory:true,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}))
  let mode:'failed'|'ready'|'numeric'='failed'
  const rule='<img src=x onerror=window.priceInjected=true>'
  await page.route('**/api/admin/products/price-product**',route=>{
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/prices')){
      if(mode==='failed')return route.fulfill({status:503,json:{error:{code:'catalog.unavailable'}}})
      const prices=path.includes('/empty/')?[]:[{id:'price-exact',price_set_id:'set',currency_code:'eur',amount:mode==='numeric'?1:'9007199254740993.000000000000000001',min_quantity:null,max_quantity:null,rules_count:1,price_list_id:null,price_list:null,rules:[{id:'rule',attribute:'unknown',operator:'eq',value:rule,priority:0}]}]
      return route.fulfill({json:{product_id:product.id,variant_id:path.includes('/empty/')?'empty':'priced',price_set_ids:['set'],prices,count:prices.length,limit:20,offset:0}})
    }
    return route.fulfill({json:path.endsWith('/variants')?{variants,count:2,limit:20,offset:0}:{product}})
  })
  await page.goto('/account');await signIn(page);await page.goto('/products/price-product')
  const row=(name:string)=>page.getByRole('row').filter({has:page.getByRole('cell',{name,exact:true}).first()})
  await row('priced').getByRole('button',{name:'查看价格',exact:true}).click()
  const drawer=page.getByRole('dialog',{name:'价格配置',exact:true})
  await expect(drawer.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
  await expect(drawer.getByText('暂无已配置价格',{exact:true})).toHaveCount(0)
  mode='ready';await drawer.getByRole('button',{name:'重新连接',exact:true}).click()
  await expect(drawer.getByRole('cell',{name:'9007199254740993.000000000000000001',exact:true})).toBeVisible()
  await drawer.locator('button.ant-table-row-expand-icon').click()
  await expect(drawer.getByText(rule,{exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Object.hasOwn(window,'priceInjected'))).toBe(false)
  await drawer.getByRole('button',{name:'取消',exact:true}).click()
  await expect(drawer).toHaveCount(0)
  await row('empty').getByRole('button',{name:'查看价格',exact:true}).click()
  await expect(drawer.getByText('暂无已配置价格',{exact:true})).toBeVisible()
  await drawer.getByRole('button',{name:'取消',exact:true}).click()
  await expect(drawer).toHaveCount(0)
  mode='numeric';await row('priced').getByRole('button',{name:'查看价格',exact:true}).click()
  await expect(drawer.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
  await expect(drawer.getByRole('cell',{name:'1',exact:true})).toHaveCount(0)
})

test('stock save preserves exact attempt on uncertainty and stops for reconciliation',async({page})=>{
  await fixture(page)
  const product={id:'stock-edit-product',revision:'a'.repeat(64),title:'Stock fixture',handle:'stock-edit-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:1,subtitle:null,description:null,options:[],images:[]}
  const variant={id:'stock-edit-variant',product_id:product.id,title:'Stock variant',sku:null,thumbnail:null,variant_rank:0,manage_inventory:true,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}
  const attempts:unknown[]=[]
  await page.route('**/api/admin/inventory-levels/test-level/stock',route=>{
    attempts.push(route.request().postDataJSON())
    expect(route.request().method()).toBe('POST')
    expect(route.request().headers()['x-csrf-token']).toBe(sessionCSRF)
    if(attempts.length===1)return route.fulfill({status:503,json:{error:{code:'inventory.unavailable'}}})
    return route.fulfill({status:409,json:{error:{code:'inventory.reconciliation_required'}}})
  })
  await page.route('**/api/admin/products**',route=>{
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/inventory'))return route.fulfill({json:{product_id:product.id,variant_id:variant.id,manage_inventory:true,allow_backorder:false,writes_enabled:false,stock_edit_enabled:true,items:[{inventory_item_id:'test-item',required_quantity:'1',sku:null,title:'Stock item',item_available:true,migration_holds:[{location_id:'test-location',reason:'reservation_mismatch'}],unmatched_reservations:[],levels:[{id:'test-level',revision:'b'.repeat(64),location_id:'test-location',location_name:'Test location',stocked_quantity:'9007199254740993.000000000000000001',reserved_quantity:'1.000000000000000000',incoming_quantity:'0.000000000000000000',available_quantity:'9007199254740992.000000000000000001',reservation_quantity:'0.000000000000000000',reservation_count:0,reconciled:false}]}]}})
    if(path.endsWith('/variants'))return route.fulfill({json:{variants:[variant],count:1,limit:20,offset:0}})
    if(path.endsWith('/stock-edit-product'))return route.fulfill({json:{product}})
    return route.fulfill({json:{products:[product],count:1,limit:20,offset:0}})
  })
  await page.goto('/login')
  await signIn(page)
  await page.goto('/products/stock-edit-product')
  await page.getByRole('button',{name:'查看库存',exact:true}).click()
  const inventory=page.getByRole('dialog',{name:'库存明细',exact:true})
  await inventory.getByRole('button',{name:/Stock item/}).click()
  await expect(inventory.getByText('历史差异尚未完成对账，补充库存不会自动解除限制。',{exact:true})).toBeVisible()
  await inventory.getByRole('button',{name:'修改库存 test-location',exact:true}).click()
  const edit=page.getByRole('dialog',{name:'修改库存',exact:true})
  await edit.getByRole('textbox',{name:'库存数量',exact:true}).fill('9007199254740992.000000000000000001')
  await edit.getByRole('button',{name:'保存',exact:true}).click()
  await expect(edit.getByText('保存结果尚未确认。可以重试同一次保存，或关闭后重新读取库存。',{exact:true})).toBeVisible()
  await expect(edit.getByRole('textbox',{name:'库存数量',exact:true})).toBeDisabled()
  expect(attempts).toHaveLength(1)
  await edit.getByRole('button',{name:'重试本次保存',exact:true}).click()
  expect(attempts).toHaveLength(2)
  expect(attempts[0]).toEqual(attempts[1])
  expect(attempts[0]).toMatchObject({expected_revision:'b'.repeat(64),stocked_quantity:'9007199254740992.000000000000000001',incoming_quantity:'0.000000000000000000'})
  await expect(edit.getByText('此仓库的库存存在待核对差异，暂不能减少库存。',{exact:true})).toBeVisible()
  await expect(edit.getByRole('button',{name:'保存',exact:true})).toBeDisabled()
  await edit.getByRole('button',{name:'取消',exact:true}).click()
  await expect(edit).toHaveCount(0)
})

test('inventory drawer separates outage and empty state and rejects numeric quantities',async({page})=>{
  await fixture(page)
  const product={id:'stock-product',revision:'a'.repeat(64),title:'Stock fixture',handle:'stock-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:1,subtitle:null,description:null,options:[],images:[]}
  const variant={id:'stock-variant',product_id:product.id,title:'Stock variant',sku:null,thumbnail:null,variant_rank:0,manage_inventory:true,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}
  let mode:'failed'|'empty'|'numeric'='failed'
  await page.route('**/api/admin/products/stock-product**',route=>{
    const path=new URL(route.request().url()).pathname
    if(path.endsWith('/inventory')){
      if(mode==='failed')return route.fulfill({status:503,json:{error:{code:'inventory.unavailable'}}})
      const items=mode==='empty'?[]:[{inventory_item_id:'item',required_quantity:1,sku:null,title:null,item_available:true,migration_holds:[],levels:[],unmatched_reservations:[]}]
      return route.fulfill({json:{product_id:product.id,variant_id:variant.id,manage_inventory:true,allow_backorder:false,writes_enabled:false,stock_edit_enabled:true,items}})
    }
    return route.fulfill({json:path.endsWith('/variants')?{variants:[variant],count:1,limit:20,offset:0}:{product}})
  })
  await page.goto('/account');await signIn(page);await page.goto('/products/stock-product')
  await page.getByRole('button',{name:'查看库存',exact:true}).click()
  const drawer=page.getByRole('dialog',{name:'库存明细',exact:true})
  await expect(drawer.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
  await expect(drawer.getByText('未关联库存项',{exact:true})).toHaveCount(0)
  mode='empty';await drawer.getByRole('button',{name:'重新连接',exact:true}).click()
  await expect(drawer.getByText('未关联库存项',{exact:true})).toBeVisible()
  await drawer.getByRole('button',{name:'取消',exact:true}).click();await expect(drawer).toHaveCount(0)
  mode='numeric';await page.getByRole('button',{name:'查看库存',exact:true}).click()
  await expect(drawer.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
  await expect(drawer.getByText('未关联库存项',{exact:true})).toHaveCount(0)
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


test('payment evidence retry, malformed amounts and mixed versions stay separate from order details',async({page})=>{
 await fixture(page)
 const order={id:'payment-order',version:2,status:'pending',currency_code:'eur',custom_display_id:'Payment fixture',display_id:'1',is_draft_order:false,email:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',line_count:0}
 let mode:'failed'|'ready'|'changed'|'numeric'='failed'
 await page.route('**/api/admin/orders/payment-order**',route=>{
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/fulfillments'))return route.fulfill({json:{order_id:order.id,version:2,operations_enabled:false,fulfillments:[]}})
  if(path.endsWith('/amounts'))return route.fulfill({json:{order_id:order.id,version:2,currency_code:'eur',totals:{item_subtotal:'0',shipping_subtotal:'0',discount_subtotal:'0',tax_total:'0',credit_line_total:'0',total:'0'},lines:[],shipping_methods:[],summary:null}})
  if(!path.endsWith('/payments'))return route.fulfill({json:{order,lines:[]}})
  if(mode==='failed')return route.fulfill({status:503,json:{error:{code:'orders.payments_unavailable'}}})
  return route.fulfill({json:{order_id:order.id,version:mode==='changed'?3:2,operations_enabled:false,collections:mode==='numeric'?[{id:'collection',currency_code:'eur',status:'completed',amount:9007199254740993,authorized_amount:null,captured_amount:null,refunded_amount:null,payments:[],sessions:[]}]:[]}})
 })
 await page.goto('/login');await signIn(page);await page.goto('/orders/payment-order')
 const region=page.getByRole('region',{name:'支付记录',exact:true})
 await expect(region.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'Payment fixture',exact:true})).toBeVisible()
 mode='ready';await region.getByRole('button',{name:'重新连接',exact:true}).click()
 await expect(region.getByText('未关联付款集合',{exact:true})).toBeVisible()
 mode='changed';await page.reload()
 await expect(region.getByText('订单版本已变化，请刷新详情后重试。',{exact:true})).toBeVisible()
 mode='numeric';await region.getByRole('button',{name:'重新连接',exact:true}).click()
 await expect(region.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
 await expect(region.getByText('未关联付款集合',{exact:true})).toHaveCount(0)
})


test('Ant fulfillment evidence preserves missing records, safe links, exact strings and independent retry',async({page})=>{
 await fixture(page)
 const order={id:'fulfillment-order',version:2,status:'pending',currency_code:'eur',custom_display_id:'Fulfillment fixture',display_id:'1',is_draft_order:false,email:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',line_count:0}
 const record={location_id:'retired-location',provider_id:null,shipping_option_id:null,delivery_address_id:'missing-address',requires_shipping:true,created_at:'2026-10-28T00:00:00Z',packed_at:null,shipped_at:null,delivered_at:null,canceled_at:null,address:null,items:[{id:'item',title:'Packed <script>phone</script>',sku:'0000123456789012345',barcode:'00000001234567890',quantity:'9007199254740993.000000000000000001',line_item_id:null,inventory_item_id:'missing-stock'}],labels:[{id:'label',tracking_number:'000012345678901234567890',tracking_url:'javascript:alert(1)',label_url:'https://example.invalid/private-label'}]}
 let mode:'failed'|'ready'|'empty'|'changed'|'numeric'|'expired'='failed'
 const requested:string[]=[];page.on('request',r=>{if(r.url().startsWith('https://example.invalid'))requested.push(r.url())})
 await page.route('**/api/admin/orders/fulfillment-order**',route=>{
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/amounts'))return route.fulfill({json:{order_id:order.id,version:2,currency_code:'eur',totals:{item_subtotal:'0',shipping_subtotal:'0',discount_subtotal:'0',tax_total:'0',credit_line_total:'0',total:'0'},lines:[],shipping_methods:[],summary:null}})
  if(path.endsWith('/payments'))return route.fulfill({json:{order_id:order.id,version:2,operations_enabled:false,collections:[]}})
  if(!path.endsWith('/fulfillments'))return route.fulfill({json:{order,lines:[]}})
  if(mode==='failed'||mode==='expired')return route.fulfill({status:mode==='expired'?401:503,json:{error:{code:'orders.fulfillments_unavailable'}}})
  return route.fulfill({json:{order_id:order.id,version:mode==='changed'?3:2,operations_enabled:false,fulfillments:mode==='empty'?[]:[{id:'missing-package',availability:'missing',record:null},{id:'deleted-package',availability:'deleted',record:null},{id:'available-package',availability:'available',record:{...record,items:mode==='numeric'?[{...record.items[0],quantity:9007199254740993}]:record.items}}]}})
 })
 await page.goto('/login');await signIn(page);await page.goto('/orders/fulfillment-order')
 const region=page.getByRole('region',{name:'履约记录',exact:true})
 await expect(region.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'Fulfillment fixture',exact:true})).toBeVisible()
 mode='ready';await region.getByRole('button',{name:'重新连接',exact:true}).click()
 await expect(region.getByText('包裹记录缺失',{exact:true})).toBeVisible()
 await expect(region.getByText('包裹记录已删除',{exact:true})).toBeVisible()
 await expect(region.getByText('9007199254740993.000000000000000001',{exact:true})).toBeVisible()
 await expect(region.getByText('000012345678901234567890',{exact:true})).toBeVisible()
 await expect(region.getByText('Packed <script>phone</script>',{exact:true})).toBeVisible()
 await expect(region.getByText('地址记录缺失或已删除',{exact:true})).toBeVisible()
 await expect(region.getByText('javascript:alert(1)',{exact:true})).toBeVisible()
 await expect(region.getByRole('link',{name:'查看标签',exact:true})).toHaveAttribute('href','https://example.invalid/private-label')
 await expect(region.getByRole('link',{name:'查看标签',exact:true})).toHaveAttribute('rel','noopener noreferrer')
 expect(await region.locator('a[href^="javascript:"]').count()).toBe(0)
 expect(requested).toEqual([])
 mode='changed';await page.reload()
 await expect(region.getByText('订单版本已变化，请刷新详情后重试。',{exact:true})).toBeVisible()
 mode='numeric';await region.getByRole('button',{name:'重新连接',exact:true}).click()
 await expect(region.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
 await expect(region.getByText('未关联包裹',{exact:true})).toHaveCount(0)
 mode='empty';await region.getByRole('button',{name:'重新连接',exact:true}).click()
 await expect(region.getByText('未关联包裹',{exact:true})).toBeVisible()
 mode='expired';await page.reload();await expect(page).toHaveURL('/login')
})


test('current permissions hide menus, deny direct navigation and retry an unavailable authority source',async({page})=>{
 const options:{permissions:string[];failPermissions:boolean}={permissions:['product.read'],failPermissions:false}
 await fixture(page,options)
 await page.route('**/api/admin/products?*',route=>route.fulfill({json:{products:[],count:0,limit:20,offset:0}}))
 await page.goto('/account');await signIn(page)
 await expect(page.getByRole('menuitem',{name:'商品',exact:true})).toBeVisible()
 await expect(page.getByRole('menuitem',{name:'订单',exact:true})).toHaveCount(0)
 await page.goto('/orders')
 await expect(page.getByText('没有访问此功能的权限',{exact:true})).toBeVisible()
 options.failPermissions=true;await page.reload()
 await expect(page.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
 options.failPermissions=false;options.permissions=['order.read']
 await page.getByRole('button',{name:'重新连接',exact:true}).click()
 await expect(page.getByRole('menuitem',{name:'订单',exact:true})).toBeVisible()
 await expect(page.getByRole('menuitem',{name:'商品',exact:true})).toHaveCount(0)
 await page.goto('/products')
 await expect(page.getByText('没有访问此功能的权限',{exact:true})).toBeVisible()
 options.permissions=['*'];await page.reload()
 await expect(page.getByText('暂时无法连接服务，请稍后重试。',{exact:true})).toBeVisible()
 await expect(page.getByRole('menuitem',{name:'商品',exact:true})).toHaveCount(0)
})


test('price edit keeps authoritative precision and one immutable attempt, then applies current write grants',async({page})=>{
 const grants:string[]=[...ACTIONS];await fixture(page,{permissions:grants})
 const product={id:'price-edit-product',revision:'a'.repeat(64),title:'Price edit fixture',handle:'price-edit-product',status:'published',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:1,subtitle:null,description:null,options:[],images:[]}
 const variant={id:'price-edit-v',product_id:product.id,title:'Price variant',sku:'Price SKU',thumbnail:null,variant_rank:0,manage_inventory:false,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}
 const attempts:unknown[]=[]
 await page.route('**/api/admin/products/price-edit-product**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/prices/price-one')){
   if(request.method()==='POST'){expect(request.headers()['x-csrf-token']).toBe(sessionCSRF);attempts.push(request.postDataJSON());if(attempts.length===1){await route.abort('failed');return};return route.fulfill({status:409,json:{error:{code:'catalog.edit_conflict'}}})}
   return route.fulfill({json:{product_id:product.id,variant_id:variant.id,id:'price-one',price_set_id:'set',currency_code:'eur',amount:'1.000000000000000000',effective_amount:'1.00000000000000000001',min_quantity:null,max_quantity:null,price_list_id:null,shared_variants:2,enabled:true,revision:'b'.repeat(64)}})
  }
  if(path.endsWith('/prices'))return route.fulfill({json:{product_id:product.id,variant_id:variant.id,price_set_ids:['set'],prices:[{id:'price-one',price_set_id:'set',currency_code:'eur',amount:'1',min_quantity:null,max_quantity:null,rules_count:0,price_list_id:null,price_list:null,rules:[]}],count:1,limit:20,offset:0}})
  return route.fulfill({json:path.endsWith('/variants')?{variants:[variant],count:1,limit:20,offset:0}:{product}})
 })
 await page.goto('/account');await signIn(page);await page.goto('/products/price-edit-product');await page.getByRole('button',{name:'查看价格',exact:true}).click();await page.getByRole('button',{name:'修改金额',exact:true}).click()
 const edit=page.getByRole('dialog',{name:'修改金额',exact:true});await expect(edit.getByLabel('金额',{exact:true})).toHaveValue('1.00000000000000000001')
 await edit.getByLabel('金额',{exact:true}).fill('9007199254740993.000000000000000001');await edit.getByRole('button',{name:'保存',exact:true}).click();await expect(edit.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible();await expect(edit.getByLabel('金额',{exact:true})).toBeDisabled();await edit.getByRole('button',{name:'重试本次保存',exact:true}).click()
 await expect(edit.getByRole('button',{name:'重试本次保存',exact:true})).toHaveCount(0);expect(attempts).toHaveLength(2);expect(attempts[0]).toEqual(attempts[1]);expect(attempts[0]).toMatchObject({amount:'9007199254740993.000000000000000001'})
 await edit.getByRole('button',{name:'取消',exact:true}).click();grants.splice(grants.indexOf('price.write'),1);await page.reload();await page.getByRole('button',{name:'查看价格',exact:true}).click();await expect(page.getByRole('cell',{name:'1',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'修改金额',exact:true})).toHaveCount(0)
})


test('base price creation retries one immutable currency and amount, then returns to read-only',async({page})=>{
 const grants:string[]=[...ACTIONS];await fixture(page,{permissions:grants})
 const product={id:'base-create-product',revision:'a'.repeat(64),title:'Base price fixture',handle:'base-create-product',status:'published',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:1,subtitle:null,description:null,options:[],images:[]}
 const variant={id:'base-create-v',product_id:product.id,title:'Unpriced variant',sku:'UNPRICED',thumbnail:null,variant_rank:0,manage_inventory:false,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}
 const attempts:unknown[]=[]
 await page.route('**/api/admin/products/base-create-product**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/price-creation'))return route.fulfill({json:{product_id:product.id,variant_id:variant.id,price_set_id:null,currencies:['eur','usd'],existing_base_currencies:[],shared_variants:0,enabled:true,revision:'b'.repeat(64)}})
  if(path.endsWith('/prices')){
   if(request.method()==='POST'){expect(request.headers()['x-csrf-token']).toBe(sessionCSRF);attempts.push(request.postDataJSON());if(attempts.length===1){await route.abort('failed');return};return route.fulfill({json:{id:'price_created',revision:'c'.repeat(64),replayed:true}})}
   return route.fulfill({json:{product_id:product.id,variant_id:variant.id,price_set_ids:[],prices:[],count:0,limit:20,offset:0}})
  }
  return route.fulfill({json:path.endsWith('/variants')?{variants:[variant],count:1,limit:20,offset:0}:{product}})
 })
 await page.goto('/account');await signIn(page);await page.goto('/products/base-create-product');await page.getByRole('button',{name:'查看价格',exact:true}).click();await page.getByRole('button',{name:'新增基础价格',exact:true}).click()
 const edit=page.getByRole('dialog',{name:'新增基础价格',exact:true});const currency=edit.getByLabel('币种',{exact:true});await currency.click();await currency.fill('eur');await currency.press('Enter')
 await edit.getByLabel('金额',{exact:true}).fill('0');await edit.getByRole('button',{name:'保存',exact:true}).click();await expect(edit.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible();await expect(edit.getByLabel('金额',{exact:true})).toBeDisabled();await expect(currency).toBeDisabled();await edit.getByRole('button',{name:'重试本次保存',exact:true}).click();await expect(edit).toHaveCount(0)
 expect(attempts).toHaveLength(2);expect(attempts[0]).toEqual(attempts[1]);expect(attempts[0]).toMatchObject({currency_code:'eur',amount:'0.000000000000000000'})
 grants.splice(grants.indexOf('price.write'),1);await page.reload();await page.getByRole('button',{name:'查看价格',exact:true}).click();await expect(page.getByText('暂无已配置价格',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'新增基础价格',exact:true})).toHaveCount(0)
})

test('SKU creation freezes full option selection after uncertain response and rechecks current grants',async({page})=>{
 const grants:string[]=[...ACTIONS];await fixture(page,{permissions:grants})
 const product={id:'sku-create-product',revision:'a'.repeat(64),title:'SKU fixture',handle:'sku-create-product',status:'published',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:0,subtitle:null,description:null,options:[],images:[]}
 const attempts:unknown[]=[]
 await page.route('**/api/admin/products/sku-create-product**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/variant-creation'))return route.fulfill({json:{product_id:product.id,options:[{id:'storage',title:'Storage',values:[{id:'128',value:'128 GB',rank:0}]}],enabled:true,revision:'b'.repeat(64)}})
  if(path.endsWith('/variants')){
   if(request.method()==='POST'){expect(request.headers()['x-csrf-token']).toBe(sessionCSRF);attempts.push(request.postDataJSON());if(attempts.length===1){await route.abort('failed');return};return route.fulfill({json:{id:'variant_created',inventory_item_id:'iitem_created',sku:'ERP10001',revision:'c'.repeat(64),replayed:true}})}
   return route.fulfill({json:{variants:[],count:0,limit:20,offset:0}})
  }
  return route.fulfill({json:{product}})
 })
 await page.goto('/account');await signIn(page);await page.goto('/products/sku-create-product');await page.getByRole('button',{name:'新建 SKU',exact:true}).click()
 const edit=page.getByRole('dialog',{name:'新建 SKU',exact:true});await edit.getByLabel('规格名称',{exact:true}).fill('128 GB');const storage=edit.getByLabel('Storage',{exact:true});await storage.click();await storage.fill('128');await storage.press('Enter')
 await edit.getByRole('button',{name:'保存',exact:true}).click();await expect(edit.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible();await expect(storage).toBeDisabled();await expect(edit.getByLabel('SKU',{exact:true})).toBeDisabled();await edit.getByRole('button',{name:'重试本次保存',exact:true}).click();await expect(edit).toHaveCount(0)
 expect(attempts).toHaveLength(2);expect(attempts[0]).toEqual(attempts[1]);expect(attempts[0]).toMatchObject({title:'128 GB',sku:'',option_value_ids:['128']})
 grants.splice(grants.indexOf('product.write'),1);await page.reload();await expect(page.getByRole('button',{name:'新建 SKU',exact:true})).toHaveCount(0)
})

test('warehouse stock creation freezes exact location and quantities on uncertainty',async({page})=>{
 const grants:string[]=[...ACTIONS];await fixture(page,{permissions:grants})
 const product={id:'stock-create-product',revision:'a'.repeat(64),title:'Stock fixture',handle:'stock-create-product',status:'published',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:1,subtitle:null,description:null,options:[],images:[]}
 const variant={id:'stock-create-v',product_id:product.id,title:'Stock variant',sku:'Stock SKU',thumbnail:null,variant_rank:0,manage_inventory:true,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}
 const attempts:unknown[]=[]
 await page.route('**/api/admin/products/stock-create-product**',async route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  if(path.endsWith('/level-creation'))return route.fulfill({json:{product_id:product.id,variant_id:variant.id,inventory_item_id:'stock-item',locations:[{id:'stock-location',name:'Stock warehouse',has_level:false}],shared_variants:1,enabled:true,revision:'b'.repeat(64)}})
  if(path.endsWith('/levels')){expect(req.headers()['x-csrf-token']).toBe(sessionCSRF);attempts.push(req.postDataJSON());if(attempts.length===1){await route.abort('failed');return};return route.fulfill({json:{id:'ilevel_created',revision:'c'.repeat(64),replayed:true}})}
  if(path.endsWith('/inventory'))return route.fulfill({json:{product_id:product.id,variant_id:variant.id,manage_inventory:true,allow_backorder:false,writes_enabled:false,stock_edit_enabled:true,items:[{inventory_item_id:'stock-item',sku:'Stock SKU',title:'Stock inventory',required_quantity:'1.000000000000000000',item_available:true,levels:[],migration_holds:[],unmatched_reservations:[]}]}})
  return route.fulfill({json:path.endsWith('/variants')?{variants:[variant],count:1,limit:20,offset:0}:{product}})
 })
 await page.goto('/account');await signIn(page);await page.goto('/products/stock-create-product');await page.getByRole('button',{name:'查看库存',exact:true}).click();await page.getByText('Stock inventory',{exact:true}).click();await page.getByRole('button',{name:'添加仓库库存',exact:true}).click()
 const edit=page.getByRole('dialog').last(),location=edit.getByLabel('仓库',{exact:true});await location.click();await location.fill('Stock warehouse');await location.press('Enter');await edit.getByLabel('库存数量',{exact:true}).fill('0.000000000000000001');await edit.getByLabel('在途数量',{exact:true}).fill('2')
 await edit.getByRole('button',{name:'保存',exact:true}).click();await expect(edit.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible();await expect(location).toBeDisabled();await expect(edit.getByLabel('库存数量',{exact:true})).toBeDisabled();await edit.getByRole('button',{name:'重试本次保存',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(1)
 expect(attempts).toHaveLength(2);expect(attempts[0]).toEqual(attempts[1]);expect(attempts[0]).toMatchObject({location_id:'stock-location',stocked_quantity:'0.000000000000000001',incoming_quantity:'2.000000000000000000'})
 grants.splice(grants.indexOf('warehouse.read'),1);await page.reload();await page.getByRole('button',{name:'查看库存',exact:true}).click();await page.getByText('Stock inventory',{exact:true}).click();await expect(page.getByRole('button',{name:'添加仓库库存',exact:true})).toHaveCount(0)
})
