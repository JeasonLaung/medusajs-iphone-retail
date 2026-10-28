import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {readFile,mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacyHeader} from './legacy-header.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const output=new URL('../../../logs/gin-migration/astro-header/',import.meta.url)
let server,browser
try{
 for(const [current,source] of [['header.module.css','store-header'],['region-language.module.css','region-language']])assert.equal((await readFile(new URL('../src/components/layout/'+current,import.meta.url),'utf8')).replaceAll('\r\n','\n'),(await readFile(new URL('../../../medusa-store/apps/storefront/src/modules/layout/components/'+source+'/styles.module.css',import.meta.url),'utf8')).replaceAll('\r\n','\n'))
 server=await dev({root:new URL('./header-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],publicDir:fileURLToPath(new URL('../public/',import.meta.url)),vite:{plugins:[legacyHeader()],resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 const origin=`http://127.0.0.1:${server.address.port}`
 browser=await chromium.launch({headless:true,args:['--disable-lcd-text'],...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({reducedMotion:'reduce'}),errors=[],comparisons=[]
 context.setDefaultTimeout(15000)
 await context.route('https://fonts.gstatic.com/**',r=>r.abort())
 const fresh=async(width)=>{const p=await context.newPage();await p.setViewportSize({width,height:900});p.on('pageerror',e=>errors.push(e.message));return p}
 async function ready(page,path){await page.goto(origin+path);await expect(page.getByTestId('ready')).toHaveAttribute('data-ready','true');await page.evaluate(()=>document.fonts.ready);await page.locator('header img').evaluateAll(async images=>Promise.all(images.map(im=>im.decode())));assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)}
 async function compare(a,b,label,selector='header'){
  for(const page of [a,b]){await page.mouse.move(0,890);await page.evaluate(()=>document.activeElement instanceof HTMLElement&&document.activeElement.blur())}
  assert.equal(await a.locator(selector).innerText(),await b.locator(selector).innerText(),label+' text')
  assert.deepEqual(await a.locator(selector+' a').evaluateAll(nodes=>nodes.map(a=>a.getAttribute('href'))),await b.locator(selector+' a').evaluateAll(nodes=>nodes.map(a=>a.getAttribute('href'))))
  const x=await a.locator(selector).screenshot(),y=await b.locator(selector).screenshot()
  await mkdir(output,{recursive:true});await writeFile(new URL(label+'-new.png',output),x);await writeFile(new URL(label+'-reference.png',output),y)
  assert.ok(x.equals(y),label+' pixels differ');comparisons.push(label)
 }
 for(const lang of ['en','zh'])for(const width of [390,768,1440]){
  const a=await fresh(width),b=await fresh(width),segment='dk-'+lang,path=`/${segment}/products/iphone-14?sort=price#specs`,preferences=lang==='en'?'Country and language':'国家与语言'
  await ready(a,path);await ready(b,path.replace('?','?reference=1&'));await compare(a,b,`${lang}-${width}-header`)
  for(const p of [a,b])await expect(p.getByTestId('nav-cart-link')).toHaveAttribute('aria-label',lang==='en'?'Cart (3)':'购物车 (3)')
  if(width<799){
   for(const p of [a,b])await p.getByTestId('nav-menu-button').click()
   await compare(a,b,`${lang}-${width}-mobile-menu`,'[data-testid="nav-mobile-panel"]')
   for(const p of [a,b]){await p.getByRole('button',{name:lang==='en'?'Close menu':'关闭菜单',exact:true}).focus();await p.keyboard.press('Escape');await expect(p.getByTestId('nav-menu-button')).toHaveAttribute('aria-expanded','false');await expect(p.getByTestId('nav-menu-button')).toBeFocused()}
  }else{
   for(const p of [a,b])await p.getByRole('button',{name:lang==='en'?'About us':'关于我们',exact:true}).click()
   await compare(a,b,`${lang}-${width}-dropdown`,'[data-testid="nav-desktop-panel"]')
   for(const p of [a,b])await p.keyboard.press('Escape')
  }
  for(const p of [a,b]){await p.getByRole('button',{name:preferences,exact:true}).click();await p.getByRole('dialog').locator('img').evaluateAll(async images=>Promise.all(images.map(im=>im.decode())))}
  await compare(a,b,`${lang}-${width}-preferences`,'[role="dialog"]')
  const releases=[]
  for(const p of [a,b]){await p.route('**/test-preferences',async r=>{await new Promise(resolve=>releases.push(resolve));await r.fulfill({status:503,json:{}})});await p.getByRole('button',{name:'Deutschland — English',exact:true}).click();await expect(p.getByRole('button',{name:'Deutschland — English',exact:true})).toBeDisabled()}
  await compare(a,b,`${lang}-${width}-pending`,'[role="dialog"]')
  assert.equal(releases.length,2);releases.forEach(resolve=>resolve())
  for(const p of [a,b])await expect(p.getByRole('alert')).toBeVisible()
  await compare(a,b,`${lang}-${width}-error`,'[role="dialog"]')
  let request
  await a.unroute('**/test-preferences');await a.route('**/test-preferences',r=>{request=r.request().postDataJSON();return r.fulfill({status:200,json:{}})})
  await a.getByRole('button',{name:'Deutschland — English',exact:true}).click();await expect(a.getByRole('dialog')).toHaveCount(0);await a.getByRole('button',{name:preferences,exact:true}).click();await a.keyboard.press('Escape');await expect(a.getByRole('dialog')).toHaveCount(0);await expect(a.getByRole('button',{name:preferences,exact:true})).toBeFocused()
  assert.deepEqual(request,{country:'de',locale:'en',regionID:'eu',currency:'eur',segment:'de-en',currentPath:'/products/iphone-14?sort=price#specs',location:'/de-en/products/iphone-14?sort=price#specs'})
  await a.close();await b.close()
 }
 for(const mode of ['zero','large','empty']){
  const a=await fresh(390),b=await fresh(390),path='/dk-en/store?mode='+mode
  await ready(a,path);await ready(b,path+'&reference=1');await compare(a,b,`en-390-${mode}`)
  if(mode==='large')await expect(a.getByTestId('nav-cart-link')).toHaveAttribute('aria-label','Cart (900719925474099312345)')
  if(mode==='empty'){for(const p of [a,b])await p.getByRole('button',{name:'Country and language',exact:true}).click();await compare(a,b,'en-390-empty-regions','[role="dialog"]')}
  await a.close();await b.close()
 }
 const closed=await fresh(390);let requests=0;closed.on('request',r=>{if(r.url().endsWith('/test-preferences'))requests++})
 await ready(closed,'/dk-en/store?mode=closed');await closed.getByRole('button',{name:'Country and language',exact:true}).click();await closed.getByRole('button',{name:'Deutschland — English',exact:true}).click();await expect(closed.getByRole('alert')).toBeVisible();assert.equal(requests,0);await closed.close()
 assert.deepEqual(errors,[]);await context.close()
 console.log('ASTRO_HEADER_BROWSER '+JSON.stringify({original_source_oracle:true,original_css:true,comparisons,menus_and_escape_focus:true,preferences_pending_error_retry:true,exact_cart_count:true,closed_mutation_no_request:true,preserved_path_query_hash:true,fonts:'controlled fallback only',identity:'anonymous only; notification/customer integration pending',real_region_mutation:false,production_page_mounted:false}))
}finally{await browser?.close();if(server)await server.stop()}
