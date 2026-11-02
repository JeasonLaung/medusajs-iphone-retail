import assert from 'node:assert/strict'
import {chromium,expect} from '@playwright/test'

const origin=process.env.ADMIN_UI_ORIGIN
if(!/^http:\/\/127\.0\.0\.1:\d+$/.test(origin??''))throw Error('Local Admin origin required')
const browser=await chromium.launch({channel:'chrome',headless:true})
const cases=[]
try{
 for(const reducedMotion of ['no-preference','reduce'])for(const width of [1440,390]){
  const context=await browser.newContext({reducedMotion,viewport:{width,height:900}})
  await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
  const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));page.setDefaultTimeout(10000)
  await page.goto(origin+'/account')
  const select=page.locator('.app-header .ant-select')
  for(const language of ['简体中文','English']){
   await select.click()
   const option=page.locator('.ant-select-dropdown:visible').getByText(language,{exact:true})
   await expect(option).toBeInViewport();await option.click()
   await expect(page.getByLabel(language==='English'?'Email':'邮箱',{exact:true})).toBeVisible()
  }
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  assert.deepEqual(errors,[]);cases.push({reducedMotion,width,mouse_language_selection:true,browser_errors:0})
  await context.close()
 }
 console.log('ADMIN_REDUCED_MOTION_NATIVE '+JSON.stringify({api_mocking:false,login_not_required:true,public_language_selection_only:true,cases}))
}finally{await browser.close()}
