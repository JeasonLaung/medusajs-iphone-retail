// Main preview: inspect existing catalog editors without saving any business data.
import assert from 'node:assert/strict'
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '@playwright/test'
const expect=baseExpect.configure({timeout:20000}),origin='http://127.0.0.1:9200'
const logs=resolve('../../logs/retail-native/catalog-read');mkdirSync(logs,{recursive:true})
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
const credential=name=>credentials.split(/\r?\n/).find(line=>new RegExp('^'+name+'\\s*[:=]','i').test(line))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const errors=[],writes=[],checks=[]
try{
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 // Block business writes as well as recording them; account sign-in is separate.
 await page.route('**/api/admin/**',async route=>{if(route.request().method()!=='GET'&&!route.request().url().includes('/auth/')){writes.push(route.request().url());return route.abort()}return route.continue()})
 await page.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(credential('Email'));await page.getByLabel('Password',{exact:true}).fill(credential('Password'))
 await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 const get=async path=>{const response=await context.request.get(origin+'/api/admin/'+path);assert.equal(response.status(),200);return response.json()}
 // Obtain the existing product identity through the public handle rather than a fixture ID.
 const publicResponse=await context.request.get(origin+'/api/store/products?handle=swappie-iphone-17e');assert.equal(publicResponse.status(),200)
 const publicData=await publicResponse.json();const found=publicData.products.find(p=>p.handle==='swappie-iphone-17e');assert.ok(found)
 const variants=await get('products/'+found.id+'/variants?limit=100&offset=0');const variant=variants.variants[0];assert.ok(variant)
 const base='products/'+found.id,sub=base+'/variants/'+variant.id
 for(const path of [base+'/images',base+'/options',sub+'/edit',sub+'/images',sub+'/inventory-items','option-definitions?offset=0'])assert.equal((await get(path)).enabled,true,path)
 for(const [language,width] of [['en',1440],['zh',390]]){
  await page.evaluate(language=>localStorage.setItem('eurorefurb.admin.language',language),language);await page.setViewportSize({width,height:1000})
  for(const [en,zh] of [['Manage images','管理图片'],['Manage options','管理选项'],['Edit SKU','编辑 SKU'],['SKU images','SKU 图片']]){
   await page.goto(origin+'/'+base+'?limit=100&offset=0')
   const button=(en==='Edit SKU'||en==='SKU images')?page.getByRole('row').filter({hasText:variant.sku}).getByRole('button',{name:language==='en'?en:zh,exact:true}):page.getByRole('button',{name:language==='en'?en:zh,exact:true})
   await button.click();const drawer=page.getByRole('dialog');await expect(drawer).toBeVisible();await expect(drawer.locator('.ant-spin-spinning')).toHaveCount(0);writeFileSync(resolve(logs,'last-dialog.txt'),await drawer.innerText());await expect(drawer.getByRole('button',{name:language==='en'?'Save':/^保\s*存$/,exact:true})).toBeEnabled()
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
   await page.screenshot({path:resolve(logs,language+'-'+en.replaceAll(' ','-')+'.png'),fullPage:true})
   await drawer.getByRole('button',{name:language==='en'?'Cancel':/^取\s*消$/,exact:true}).click();await expect(drawer).toHaveCount(0)
  }
  checks.push({language,width,editors_enabled:true,no_horizontal_overflow:true})
 }
 assert.deepEqual(writes,[]);assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'report.json'),JSON.stringify({passed:true,read_only:true,business_writes:0,browser_errors:0,product_id:found.id,variant_id:variant.id,checks},null,2))
}finally{await browser.close()}
