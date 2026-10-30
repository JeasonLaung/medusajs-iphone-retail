import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {mkdir,writeFile} from 'node:fs/promises'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacySell} from './legacy-sell.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const answers={model:'iphone-13',memory:'128GB',battery:'90',checks:[true,true,true,true,true],faults:['none'],glass:['none'],screen:'none',frame:'none',back:'none'}
const key='sell-iphone-questionnaire-v2',csrf='b'.repeat(64),id='bbr_00000000-0000-4000-8000-000000000001',errors=[],cases=[]
const output=new URL('../../../logs/gin-migration/astro-sell-questionnaire/',import.meta.url)
let server,browser
try{
 server=await dev({root:new URL('./sell-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],publicDir:fileURLToPath(new URL('../public/',import.meta.url)),vite:{plugins:[legacySell()],resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const origin=`http://127.0.0.1:${server.address.port}`
 async function open({seed,locale='en',stage='model',logged=true,width=390}={}){
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  await context.addCookies([{name:'retail_customer_csrf',value:csrf,url:origin}])
  if(seed)await context.addInitScript(({key,seed})=>{if(!sessionStorage.getItem(key))sessionStorage.setItem(key,JSON.stringify(seed))},{key,seed})
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
  await page.route('https://**/*',r=>r.abort())
  await page.route('**/api/store/customer/me',async r=>{await new Promise(resolve=>setTimeout(resolve,250));await r.fulfill({status:logged?200:401,json:logged?{customer:{id:'cust_fixture',email:'customer@example.invalid',first_name:'Test',last_name:'Customer',phone:'+45123456',verified_at:'2026-10-30T00:00:00Z'}}:{}})})
  await page.route('**/api/store/buyback/estimate',r=>r.fulfill({json:{amount:294,status:'estimated',visual:'LIKE_NEW',faults:[],provisional:false,currency_code:'eur'}}))
  await page.goto(origin+`/dk-${locale}/sell/iphone/`+stage)
  return {page,context}
 }
 // Complete original questionnaire through every healthy branch, with real client navigation.
 {
  const {page,context}=await open()
  await page.getByRole('combobox',{name:'Select device model'}).click();await page.getByText('iPhone 13',{exact:true}).click()
  const next=()=>page.getByRole('button',{name:'Continue',exact:true}).click()
  await next();await expect(page).toHaveURL(/\/memory$/)
  await page.getByRole('radio',{name:'128GB',exact:true}).check();await next()
  await page.locator('#battery-capacity').fill('90');await next()
  for(let i=0;i<5;i++)await page.getByRole('button',{name:'Yes',exact:true}).click()
  await next();await page.getByRole('checkbox',{name:'Everything works',exact:true}).check();await next()
  await page.getByRole('checkbox',{name:'No cracks, chips or heavy scratches',exact:true}).check();await next()
  for(const stage of ['screen','frame','back']){
   await expect(page).toHaveURL(new RegExp('/'+stage+'$'))
   await page.getByRole('radio').last().check();await next()
  }
  await expect(page.getByRole('heading',{name:'Summary',exact:true})).toBeVisible()
  await expect(page.getByTestId('buyback-submission')).toBeVisible()
  await page.reload();await expect(page.locator('#name')).toHaveValue('Test Customer')
  assert.deepEqual(await page.evaluate(key=>JSON.parse(sessionStorage.getItem(key)),key),answers)
  let bodies=[],attempt=0
  await page.route('**/api/store/customer/buyback/requests',async r=>{
   assert.equal(r.request().headers()['x-csrf-token'],csrf);bodies.push(r.request().postDataJSON())
   await r.fulfill({status:attempt++?201:503,json:attempt===1?{error:{code:'unavailable'}}:{request:{id}}})
  })
  for(const [field,value] of Object.entries({address:'Street 1',city:'Copenhagen',postal:'1000',country:'DK'}))await page.locator('#'+field).fill(value)
  for(const checkbox of await page.getByTestId('buyback-submission').getByRole('checkbox').all())await checkbox.check()
  const submit=page.getByTestId('buyback-submission').locator('button[type=submit]')
  await submit.click();await expect(page.getByTestId('buyback-submission').locator('.ant-alert-error')).toBeVisible()
  await submit.click();await expect(page).toHaveURL(new RegExp('/account/buyback/'+id+'$'))
  assert.deepEqual(bodies[0],bodies[1]);assert.equal(bodies[0].return_address.country_code,'dk');assert.equal(bodies[0].items.length,1);const {model,memory,...assessment}=answers;assert.deepEqual(bodies[0].items[0].assessment,assessment)
  assert.equal(await page.evaluate(()=>sessionStorage.getItem('buyback-submission-key-v1')),null)
  cases.push('healthy nine stages, refresh, delayed identity, CSRF, failed submission retry uses same identity, success destination')
  await context.close()
 }
 // Stored damage skips original screen and back stages; reload keeps skipped fields valid.
 {
  const {page,context}=await open({seed:answers,stage:'iphone-13/128gb/summary',logged:false})
  const form=page.getByTestId('buyback-submission');await expect(form).toBeVisible()
  let writes=0
  await page.route('**/api/store/customer/buyback/requests',r=>{writes++;return r.fulfill({status:401,json:{}})})
  for(const [field,value] of Object.entries({name:'Test Customer',phone:'+45123456',address:'Street 1',city:'Copenhagen',postal:'1000',country:'DK'}))await page.locator('#'+field).fill(value)
  for(const checkbox of await form.getByRole('checkbox').all())await checkbox.check()
  await form.locator('button[type=submit]').click();await expect(page.getByRole('dialog')).toBeVisible()
  assert.equal(writes,0)
  await page.getByRole('button',{name:'Close',exact:true}).click()
  await expect(page.locator('#address')).toHaveValue('Street 1')
  cases.push('anonymous submit opens account modal without writing or losing contact form');await context.close()
 }
 {
  const {page,context}=await open({seed:{...answers,faults:['bright'],screen:'',frame:'damaged',back:''},stage:'iphone-13/128gb/frame',width:1440})
  await page.getByRole('button',{name:'Continue',exact:true}).click();await expect(page).toHaveURL(/\/summary$/)
  await expect(page.getByTestId('buyback-submission')).toBeVisible();await page.reload();await expect(page.getByTestId('buyback-submission')).toBeVisible()
  await page.getByRole('button',{name:'Show all answers',exact:true}).click()
  assert.equal(await page.getByRole('button',{name:'Edit: Screen condition',exact:true}).count(),0)
  assert.equal(await page.getByRole('button',{name:'Edit: Back condition',exact:true}).count(),0)
  await mkdir(output,{recursive:true});await page.screenshot({path:fileURLToPath(new URL('damaged-summary.png',output)),fullPage:true})
  cases.push('damaged screen/back skips and persisted summary')
  await context.close()
 }
 {
  const {page,context}=await open({stage:'iphone-13/128gb/summary'})
  await expect(page).toHaveURL(/\/battery$/)
  cases.push('direct incomplete summary guards to first missing answer');await context.close()
 }
 {
  const {page,context}=await open({seed:answers,locale:'zh',stage:'iphone-13/128gb/summary'})
  await expect(page.getByRole('heading',{name:'信息汇总',exact:true})).toBeVisible()
  await expect(page.getByTestId('buyback-submission')).toBeVisible()
  await expect(page.locator('#name')).toHaveValue('Test Customer')
  await expect(page.getByText(/294/).first()).toBeVisible()
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
  await page.screenshot({path:fileURLToPath(new URL('zh-summary-mobile.png',output)),fullPage:true})
  cases.push('Chinese summary with original copy and no mobile overflow');await context.close()
 }
 {
  const {page,context}=await open({seed:{...answers,checks:[false,true,true,true,true]},stage:'iphone-13/128gb/functionality'})
  await expect(page.getByText('We can’t purchase this device.',{exact:true})).toBeVisible()
  assert.equal(await page.getByRole('button',{name:'Continue',exact:true}).count(),0)
  assert.equal(await page.getByTestId('buyback-submission').count(),0)
  await page.getByRole('button',{name:'Start over',exact:true}).click();await expect(page).toHaveURL(/\/model$/)
  cases.push('ineligible powered-off device blocks progression and can restart');await context.close()
 }
 {
  const {page,context}=await open({seed:answers,stage:'iphone-13/128gb/summary'})
  const form=page.getByTestId('buyback-submission')
  await expect(form).toBeVisible()
  // Preserve a separately configured phone while restarting the questionnaire for the next one.
  await form.locator('button').filter({hasText:'Add another phone'}).click()
  await expect(page).toHaveURL(/\/model$/)
  assert.equal(await page.evaluate(()=>JSON.parse(sessionStorage.getItem('buyback-phones-v1')).length),1)
  cases.push('add another phone preserves first device while clearing current questionnaire');await context.close()
 }
 assert.deepEqual(errors,[])
 const report={cases,hydration_errors:0,scope:'Real Astro/React questionnaire and customer client; API responses intercepted. Does not prove actual Gin submit or request-detail lifecycle.'}
 await writeFile(new URL('report.json',output),JSON.stringify(report,null,2)+'\n')
 console.log('ASTRO_SELL_QUESTIONNAIRE_BROWSER '+JSON.stringify(report))
}finally{if(browser)await browser.close();if(server)await server.stop()}
