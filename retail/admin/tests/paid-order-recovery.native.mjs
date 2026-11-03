import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

const origin=process.env.CONTEXT_ASTRO_ORIGIN,api=process.env.CONTEXT_API_ORIGIN,key=process.env.CONTEXT_FIXTURE_KEY,output=process.env.CONTEXT_BROWSER_OUTPUT
assert(/^http:\/\/127\.0\.0\.1:\d+$/.test(origin)&&/^http:\/\/127\.0\.0\.1:\d+$/.test(api)&&key?.length===64&&output)
const browser=await chromium.launch({headless:true,...(process.env.CONTEXT_BROWSER_EXECUTABLE?{executablePath:process.env.CONTEXT_BROWSER_EXECUTABLE}:{})})
const cases=[],errors=[],writes=[]
async function control(path){const r=await fetch(api+'/__fixture/'+path,{method:'POST',headers:{'X-Fixture-Key':key}});assert.equal(r.status,200);return path==='new'?r.json():undefined}
const seed=await control('new')
let recoveredOrder
async function open(lang,width){
 const context=await browser.newContext({viewport:{width,height:900}})
 await context.addCookies([{name:'retail_cart_possession',value:seed.token,url:origin,httpOnly:true,sameSite:'Lax'},{name:'retail_cart_csrf',value:seed.csrf,url:origin,sameSite:'Lax'}])
 const page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname.startsWith('/api/store/'))writes.push({path:new URL(r.url()).pathname,body:r.postData()})})
 return {context,page,url:origin+'/dk-'+lang+'/payment-result?cart_id='+seed.cart_id}
}
try{
 assert.equal((await fetch(api+'/__fixture/resolve',{method:'POST'})).status,404)
 // Four layouts share one existing paid conflict; none can change its budget.
 for(const [i,[lang,width]] of [['en',390],['zh',390],['en',1440],['zh',1440]].entries()){
  const {context,page,url}=await open(lang,width)
  await page.goto(url)
  const result=page.getByTestId('payment-result'),retry=page.getByRole('button',{name:lang==='zh'?'重新检查':'Check again',exact:true})
  await expect(result).toHaveAttribute('data-status','paid_pending_order')
  await expect(page.getByRole('heading',{name:lang==='zh'?'已付款，订单待生成':'Payment received, order pending',exact:true})).toBeVisible()
  await expect(retry).toBeEnabled()
  assert.equal(await page.getByRole('link',{name:/Return to checkout|返回结账/}).count(),0)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
  await retry.focus();assert.equal(await retry.evaluate(e=>e===document.activeElement),true)
  await page.screenshot({path:join(output,`paid-pending-${lang}-${width}.png`)})
  const count=writes.filter(w=>w.path.endsWith('/complete')).length
  await page.route('**/complete',async route=>{const response=await route.fetch();assert.equal(response.status(),409);await new Promise(r=>setTimeout(r,250));await route.fulfill({response})})
  // Two synchronous clicks must still make one request while the first is live.
  await retry.evaluate(e=>{e.click();e.click()})
  await expect(page.locator('button[aria-busy="true"]')).toBeDisabled()
  await expect(retry).toBeEnabled()
  assert.equal(writes.filter(w=>w.path.endsWith('/complete')).length,count+1)
  await page.unroute('**/complete')
  let lost=false
  if(i===3){
   await control('resolve')
   await page.route('**/complete',async route=>{const response=await route.fetch();assert.equal(response.status(),200);const receipt=await response.json();recoveredOrder=receipt.order_id;assert.equal(receipt.version,'8');lost=true;await route.abort('failed')})
   await retry.click()
   await expect(result).toHaveAttribute('data-status','paid_pending_order')
   await expect(retry).toBeEnabled()
   assert(lost)
   assert.equal(page.url(),url)
   await page.unroute('**/complete')
   await page.reload()
   await expect(page).toHaveURL(origin+'/dk-'+lang+'/order/'+recoveredOrder+'/confirmed')
  }
  cases.push({scenario:'paid-conflict',lang,width,same_paid_cart:true,double_click_single_request:true,...(lost?{lost_success_response:true,reload_recovered:true}:{})})
  await context.close()
 }
 // Possession-authorized receipt recovery also works after completion/refresh.
 for(const [lang,width] of [['en',390],['zh',390],['en',1440],['zh',1440]]){
  const {context,page,url}=await open(lang,width)
  await page.goto(url)
  await expect(page).toHaveURL(origin+'/dk-'+lang+'/order/'+recoveredOrder+'/confirmed')
  cases.push({scenario:'completed-receipt-replay',lang,width,order_id:recoveredOrder})
  await context.close()
 }
 assert(writes.every(w=>/\/(payment-status|complete)$/.test(w.path)&&w.body==='{}'))
 assert(writes.filter(w=>w.path.endsWith('/complete')).length>=13)
 assert.deepEqual(errors,[])
}finally{
 await browser.close()
 await writeFile(join(output,'browser-result.json'),JSON.stringify({cases,errors,post_requests:writes,order_id:recoveredOrder},null,2)+'\n')
}
