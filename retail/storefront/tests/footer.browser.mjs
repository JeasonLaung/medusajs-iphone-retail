import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {readFile,mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacyFooter} from './legacy-footer.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const output=new URL('../../../logs/gin-migration/astro-footer/',import.meta.url)
let server,browser
try{
 for(const [current,original] of [['footer.module.css','templates/footer/styles.module.css'],['i18n.ts','i18n.ts']])assert.equal((await readFile(new URL('../src/components/layout/'+current,import.meta.url),'utf8')).replaceAll('\r\n','\n'),(await readFile(new URL('../../../medusa-store/apps/storefront/src/modules/layout/'+original,import.meta.url),'utf8')).replaceAll('\r\n','\n'))
 server=await dev({root:new URL('./footer-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],publicDir:fileURLToPath(new URL('../public/',import.meta.url)),vite:{plugins:[legacyFooter()]},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 const origin=`http://127.0.0.1:${server.address.port}`
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({reducedMotion:'reduce'}),comparisons=[],errors=[]
 // Stable local fallback for both sides; font declarations are separately
 // preserved in compiled original CSS. This does not claim font rendering QA.
 await context.route('https://fonts.gstatic.com/**',r=>r.abort())
 for(const language of ['en','zh'])for(const width of [390,768,1440])for(const mode of ['full','short','empty']){
  const pages=await Promise.all([context.newPage(),context.newPage()])
  for(const [i,page] of pages.entries()){
   page.on('pageerror',e=>errors.push(e.message));await page.setViewportSize({width,height:900})
   await page.goto(`${origin}/dk-${language}/store?mode=${mode}${i?'&reference=1':''}`)
   await page.locator('img').evaluateAll(async images=>Promise.all(images.map(im=>im.decode())))
   await page.evaluate(()=>document.fonts.ready)
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
   await expect(page.locator('footer')).toBeVisible()
  }
  const [page,reference]=pages
  assert.equal(await page.locator('footer').innerText(),await reference.locator('footer').innerText())
  assert.deepEqual(await page.locator('footer a').evaluateAll(links=>links.map(a=>a.getAttribute('href'))),await reference.locator('footer a').evaluateAll(links=>links.map(a=>a.getAttribute('href'))))
  const a=await page.locator('footer').screenshot(),b=await reference.locator('footer').screenshot(),label=`${language}-${width}-${mode}`
  await mkdir(output,{recursive:true});await writeFile(new URL(label+'-new.png',output),a);await writeFile(new URL(label+'-reference.png',output),b)
  assert.ok(a.equals(b),label+' footer differs');comparisons.push(label)
  if(mode==='full')await expect(page.getByTestId('category-link').first()).toHaveAttribute('href',`/dk-${language}/store?series=iPhone+13+%26+Pro`)
  for(const p of pages)await p.close()
 }
 assert.deepEqual(errors,[]);await context.close()
 console.log('ASTRO_FOOTER_BROWSER '+JSON.stringify({original_source_oracle:true,original_css_and_copy:true,comparisons,links_identical:true,responsive_no_overflow:true,local_brand_image:true,fonts:'same controlled fallback; remote font rendering not accepted',production_page_mounted:false}))
}finally{await browser?.close();if(server)await server.stop()}
