import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {mkdir,writeFile} from 'node:fs/promises'
import {createHash} from 'node:crypto'
import {execFileSync} from 'node:child_process'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacyPresentation} from './legacy-presentation.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const output=new URL('../../../logs/gin-migration/astro-actions/',import.meta.url)
let server,browser
try {
 server=await dev({root:new URL('./actions-fixture/',import.meta.url),publicDir:fileURLToPath(new URL('../public/',import.meta.url)),configFile:false,output:'server',integrations:[react()],vite:{plugins:[legacyPresentation()],resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 browser=await chromium.launch({headless:true,args:['--disable-lcd-text'],...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const origin=`http://127.0.0.1:${server.address.port}`,context=await browser.newContext({reducedMotion:'reduce'}),errors=[],comparisons=[]
 context.setDefaultTimeout(15000)
 const fresh=async()=>{const p=await context.newPage();p.on('pageerror',e=>errors.push(e.message));p.on('console',m=>{if(m.type()==='error')errors.push(m.text())});p.on('response',r=>{if(r.status()>=400)console.error('Fixture HTTP',r.status(),r.url())});return p}
 const ready=async(p,path)=>{await p.goto(origin+path);await expect(p.locator('main')).toHaveAttribute('data-ready','true');await p.locator('img').evaluateAll(async images=>{await Promise.race([Promise.all(images.filter(im=>{const r=im.getBoundingClientRect();return r.width>0&&r.height>0&&r.top<innerHeight&&r.bottom>0}).map(im=>im.decode())),new Promise((_,reject)=>setTimeout(()=>reject(new Error('Visible fixture image decode timeout')),5000))])});assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)}
 const compare=async(a,b,label,selector='[data-testid="actions-region"]')=>{
  const viewport=a.viewportSize()
  if(selector==='[data-testid="actions-region"]')for(const p of [a,b]){await p.setViewportSize({width:viewport.width,height:2200});await p.evaluate(()=>scrollTo(0,0))}
  for(const p of [a,b]){await p.evaluate(()=>document.activeElement instanceof HTMLElement&&document.activeElement.blur());await p.mouse.move(0,0)}
  if(selector==='.ant-modal-content')for(const p of [a,b]){
   await expect(p.locator('.ant-modal')).toHaveCSS('transform','none')
   await p.getByRole('dialog').evaluate(async el=>{
    const ancestors=[];for(let n=el;n;n=n.parentElement)ancestors.push(n)
    await Promise.all(ancestors.flatMap(n=>n.getAnimations()).map(animation=>animation.finished.catch(()=>{})))
   })
  }
  const current=a.locator(selector),old=b.locator(selector)
  assert.equal(await current.innerText(),await old.innerText(),label+' text')
  const capture=async(p,element)=>{
   await p.bringToFront()
   await p.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))))
   return element.screenshot()
  }
  const x=await capture(a,current),y=await capture(b,old);await mkdir(output,{recursive:true});await writeFile(new URL(label+'-new.png',output),x);await writeFile(new URL(label+'-reference.png',output),y)
  if(!x.equals(y))for(const [name,p] of [['new',a],['reference',b]])await writeFile(new URL(label+'-'+name+'-styles.json',output),JSON.stringify(await p.locator(selector).evaluate(el=>[...el.querySelectorAll('*')].map(node=>{const cs=getComputedStyle(node),r=node.getBoundingClientRect();return {tag:node.tagName,text:node.childElementCount?null:node.textContent,box:[r.x,r.y,r.width,r.height],style:Object.fromEntries(['fontFamily','fontSize','fontWeight','lineHeight','color','backgroundColor','letterSpacing','padding','margin','border','transform','opacity','position'].map(k=>[k,cs[k]]))}})),null,2))
  const imageComparison=JSON.parse(execFileSync(process.env.PYTHON??'python',[fileURLToPath(new URL('./compare-actions-images.py',import.meta.url)),fileURLToPath(new URL(label+'-new.png',output)),fileURLToPath(new URL(label+'-reference.png',output))],{encoding:'utf8'}))
  comparisons.push({case:label,...imageComparison,sha256:createHash('sha256').update(x).digest('hex')})
  if(selector==='[data-testid="actions-region"]')for(const p of [a,b])await p.setViewportSize(viewport)
 }
 for(const language of ['en','zh'])for(const width of [390,768,1440]){
  const page=await fresh(),old=await fresh(),path=`/dk-${language}/products/swappie-iphone-13`
  for(const p of [page,old])await p.setViewportSize({width,height:900})
  await ready(page,path);await ready(old,path+'?reference=1');await compare(page,old,`${language}-${width}-initial`)
  await expect(page.getByTestId('add-product-button')).toBeEnabled()
  await expect(page.getByRole('radio',{name:'512GB',exact:true})).toBeDisabled()
  for(const p of [page,old])await p.getByRole('radio',{name:'256GB',exact:true}).check()
  await compare(page,old,`${language}-${width}-256`)
  await expect(page.getByTestId('product-price')).toHaveAttribute('data-value','599')
  // The guide remains the existing Ant Modal, including focus restoration.
  for(const p of [page,old])await p.getByTestId('grade-guide-trigger').click()
  const dialog=page.getByRole('dialog'),oldDialog=old.getByRole('dialog')
  await expect(dialog).toBeVisible();await expect(oldDialog).toBeVisible()
  for(const p of [page,old])await p.getByRole('dialog').locator('img').evaluateAll(async images=>{await Promise.all(images.map(im=>im.decode()))})
  await compare(page,old,`${language}-${width}-grade-s`,'.ant-modal-content')
  for(const p of [page,old]){
   const modal=p.getByRole('dialog')
   await modal.getByRole('button',{name:language==='zh'?'A 级':'Grade A',exact:true}).click()
   await modal.getByRole('button',{name:language==='zh'?'屏幕':'Screen',exact:true}).click()
   await modal.locator('img').evaluateAll(async images=>{await Promise.all(images.map(im=>im.decode()))})
  }
  await compare(page,old,`${language}-${width}-grade-a-screen`,'.ant-modal-content')
  await expect(dialog.locator('img')).toHaveAttribute('src','/images/grade-guide/a-screen.png')
  // Closing with Escape restores focus to the trigger.
  for(const p of [page,old]){
   const close=p.getByRole('dialog').getByRole('button',{name:language==='zh'?'关闭等级说明':'Close grade guide',exact:true})
   await close.focus();await p.keyboard.press('Shift+Tab')
   await expect(p.getByRole('dialog').getByRole('button',{name:language==='zh'?'屏幕':'Screen',exact:true})).toBeFocused()
   await p.keyboard.press('Tab');await expect(close).toBeFocused();await p.keyboard.press('Escape')
  }
  await expect(dialog).not.toBeVisible();await expect(page.getByTestId('grade-guide-trigger')).toBeFocused()
  await page.getByTestId('product-title').scrollIntoViewIfNeeded();await page.evaluate(()=>scrollTo(0,900))
  if(width>=768){await expect(page.getByTestId('sticky-purchase-bar')).toBeVisible();await expect(page.getByTestId('sticky-purchase-bar')).toHaveCSS('top','80px')}
  else await expect(page.getByTestId('mobile-purchase-bar')).toBeVisible()
  await page.close();await old.close()
 }
 const page=await fresh()
 for(const [mode,label] of [['missing','Price pending'],['limit','Stock limit reached'],['single','Stock limit reached'],['held','Out of stock'],['closed','Add to cart'],['empty','Choose your options']]){
  await ready(page,`/dk-en/products/swappie-iphone-13?mode=${mode}`)
  await expect(page.getByTestId('add-product-button')).toBeDisabled();await expect(page.getByTestId('add-product-button')).toHaveText(label)
 }
 await ready(page,'/dk-en/products/swappie-iphone-13?mode=large')
 await expect(page.getByTestId('product-price')).toHaveText('€9,007,199,254,740,993.01')
 await expect(page.getByRole('radio',{name:'256GB',exact:true}).locator('..')).toContainText('+ €0.01')
 await page.getByRole('radio',{name:'256GB',exact:true}).check();await expect(page.getByTestId('product-price')).toHaveText('€9,007,199,254,740,993.02')
 await ready(page,'/dk-en/products/swappie-iphone-13?mode=sale')
 await expect(page.getByTestId('product-price')).toHaveText('€499.00');await expect(page.locator('s[aria-label="Original price"]')).toHaveText('€799.00');await expect(page.getByTestId('apple-launch-price')).toContainText('€869.00')
 await ready(page,'/dk-en/products/swappie-iphone-13?mode=demo')
 await expect(page.getByText('Demo price: this configuration uses the model starting price and has not been individually priced.',{exact:true})).toBeVisible()
 // Test adapter results exercise the UI only, never imply a live cart exists.
 for(const result of ['error','inventory','success']){
  await ready(page,'/dk-en/products/swappie-iphone-13')
  let requestBody,release
  const gate=new Promise(resolve=>{release=resolve})
  await page.route('**/test-cart',async route=>{requestBody=route.request().postDataJSON();await gate;await route.fulfill({json:result==='success'?{ok:true,quantities:{'128GB-S-85-Blue':'10'}}:{ok:false,error:result}})})
  await page.getByTestId('add-product-button').click();await expect(page.getByTestId('add-product-button')).toHaveText('Adding…');await expect(page.getByTestId('add-product-button')).toBeDisabled()
  release();await expect(page.getByTestId('add-product-button')).not.toHaveText('Adding…')
  assert.deepEqual(requestBody,{variantId:'128GB-S-85-Blue',quantity:'1',countryCode:'dk-en'})
  if(result==='success'){await expect(page.getByTestId('navigated')).toHaveText('/dk-en/cart');await expect(page.getByTestId('add-product-button')).toHaveText('Stock limit reached')}
  else await expect(page.getByTestId('actions-region').getByRole('status')).toContainText(result==='error'?'Could not add this item':'There is not enough stock')
  await page.unroute('**/test-cart')
 }
 await page.close();assert.deepEqual(errors,[]);await context.close()
 console.log('ASTRO_ACTIONS_BROWSER '+JSON.stringify({scope:'artificial isolated adapter and old source comparison; no live cart acceptance',comparisons,ant_grade_guide:true,sticky_and_mobile_bars:true,exact_price_and_delta:true,cart_capacity_single_location:true,checkout_false_disabled:true,test_adapter_loading_error_inventory_success:true,hydration_errors:0}))
}finally{if(browser)await browser.close();if(server)await server.stop()}
