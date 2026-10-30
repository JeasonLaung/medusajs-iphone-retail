import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacySell} from './legacy-sell.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const output=new URL('../../../logs/gin-migration/astro-sell/',import.meta.url),cases=[],errors=[]
let server,browser
try{
 server=await dev({root:new URL('./sell-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],publicDir:fileURLToPath(new URL('../public/',import.meta.url)),vite:{plugins:[legacySell()],resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({reducedMotion:'reduce'})
 await context.route('https://**/*',r=>r.abort())
 const origin=`http://127.0.0.1:${server.address.port}`
 for(const locale of ['en','zh'])for(const width of [390,768,1440]){
  const pages=await Promise.all([context.newPage(),context.newPage()])
  for(const [i,page]of pages.entries()){
   page.on('pageerror',e=>errors.push(e.message));await page.setViewportSize({width,height:900})
   await page.goto(`${origin}/?locale=${locale}${i?'&reference=1':''}`)
   await expect(page.getByTestId('sell-fixture')).toHaveAttribute('data-ready','true')
   await page.locator('img').evaluateAll(async images=>{await Promise.all(images.map(i=>i.decode()))})
   assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
  }
  for(const state of ['initial','expanded']){
   if(state==='expanded')for(const p of pages){await p.locator('.ant-collapse-header').nth(1).click();await p.mouse.move(0,0)}
   const a=pages[0].getByTestId('sell-fixture'),b=pages[1].getByTestId('sell-fixture')
   assert.equal(await a.innerText(),await b.innerText())
   for(const page of pages){assert.deepEqual(await page.locator('a').evaluateAll(links=>links.map(l=>l.getAttribute('href'))),[`/dk-${locale}/sell/iphone/model`,`/dk-${locale}/account/buyback`])}
   const current=await a.screenshot(),reference=await b.screenshot(),label=`${locale}-${width}-${state}`
   await mkdir(output,{recursive:true});await writeFile(new URL(label+'-new.png',output),current);await writeFile(new URL(label+'-source.png',output),reference)
   assert.ok(current.equals(reference),label+' sell source screenshot differs');cases.push({case:label,identical_pixels:true})
  }
  await Promise.all(pages.map(p=>p.close()))
 }
 assert.deepEqual(errors,[])
 console.log('ASTRO_SELL_BROWSER '+JSON.stringify({cases,hydration_errors:0,no_overflow:true,scope:'Original landing body/CSS, link and collapse parity only; questionnaire/submission not accepted'}))
}finally{if(browser)await browser.close();if(server)await server.stop()}
