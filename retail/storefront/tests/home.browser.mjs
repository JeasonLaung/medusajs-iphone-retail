import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacyHome} from './legacy-home.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
let server,browser
const output=new URL('../../../logs/gin-migration/astro-home/',import.meta.url),comparisons=[],errors=[]
try{
 server=await dev({root:new URL('./home-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],publicDir:fileURLToPath(new URL('../public/',import.meta.url)),vite:{plugins:[legacyHome()],resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({reducedMotion:'reduce'})
 await context.route('https://**/*',route=>route.abort())
 const origin=`http://127.0.0.1:${server.address.port}`
 for(const locale of ['en','zh'])for(const width of [390,768,1440]){
  const pages=await Promise.all([context.newPage(),context.newPage()])
  for(const [i,page]of pages.entries()){
   page.on('pageerror',e=>errors.push(e.message))
   await page.setViewportSize({width,height:900})
   await page.goto(`${origin}/?locale=${locale}${i?'&reference=1':''}`)
   await expect(page.getByTestId('home-fixture')).toHaveAttribute('data-ready','true')
   await page.locator('img').evaluateAll(async images=>{for(const im of images)im.loading='eager';await Promise.all(images.map(im=>im.decode()))})
   assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
  }
  for(const state of ['initial','next']){
   if(state==='next')for(const p of pages){await p.getByTestId('home-carousel').getByRole('button',{name:locale==='zh'?'下一张':'Next slide',exact:true}).click();await expect(p.getByTestId('home-carousel').locator('[aria-current="true"]')).toHaveAttribute('aria-label',locale==='zh'?'显示 iPhone Air':'Show iPhone Air');await p.mouse.move(0,0)}
   const a=pages[0].getByTestId('home-fixture'),b=pages[1].getByTestId('home-fixture')
   assert.equal(await a.innerText(),await b.innerText())
   const current=await a.screenshot(),reference=await b.screenshot(),label=`${locale}-${width}-${state}`
   await mkdir(output,{recursive:true});await writeFile(new URL(label+'-new.png',output),current);await writeFile(new URL(label+'-source.png',output),reference)
   assert.ok(current.equals(reference),label+' home source screenshot differs')
   comparisons.push({case:label,identical_pixels:true})
  }
  await Promise.all(pages.map(p=>p.close()))
 }
 assert.deepEqual(errors,[])
 console.log('ASTRO_HOME_BROWSER '+JSON.stringify({source_component_comparisons:comparisons,hydration_errors:0,no_overflow:true,scope:'Artificial identical home data and source component bodies/CSS with framework/data bridges; full storefront not accepted'}))
}finally{if(browser)await browser.close();if(server)await server.stop()}
