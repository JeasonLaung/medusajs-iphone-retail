import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {readFile,mkdir,writeFile} from 'node:fs/promises'
import {createHash} from 'node:crypto'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacyPresentation} from './legacy-presentation.mjs'
import {legacyCart} from './legacy-cart.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const output=new URL('../../../logs/gin-migration/astro-cart/',import.meta.url),normalized=s=>s.replaceAll('\r\n','\n')
let server,browser
try{
 for(const name of ['styles.module.css','i18n.ts'])assert.equal(normalized(await readFile(new URL('../src/components/cart/'+name,import.meta.url),'utf8')),normalized(await readFile(new URL('../../../medusa-store/apps/storefront/src/modules/cart/'+name,import.meta.url),'utf8')))
 server=await dev({root:new URL('./cart-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],vite:{plugins:[legacyCart(),legacyPresentation()],resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 const origin=`http://127.0.0.1:${server.address.port}`
 const compiled=await fetch(origin+'/dk-en/cart',{signal:AbortSignal.timeout(60000)})
 assert.equal(compiled.status,200);assert.match(await compiled.text(),/Artificial cart fixture/)
 browser=await chromium.launch({headless:true,args:['--disable-lcd-text'],...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({reducedMotion:'reduce'}),errors=[],comparisons=[]
 context.setDefaultTimeout(15000)
 const fresh=async(width)=>{const page=await context.newPage();await page.setViewportSize({width,height:1400});page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error'&&!(m.location().url===origin+'/test-cart'&&m.text().includes('status of 500')))errors.push(m.text())});return page}
 const ready=async(page,path)=>{await page.goto(origin+path);await expect(page.locator('main')).toHaveAttribute('data-ready','true');await page.locator('img').evaluateAll(async images=>{await Promise.all(images.map(im=>im.decode()))});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)}
 const compare=async(a,b,label)=>{
  for(const p of [a,b]){await p.evaluate(()=>document.activeElement instanceof HTMLElement&&document.activeElement.blur());await p.mouse.move(0,0)}
  const x=a.getByTestId('cart-container'),y=b.getByTestId('cart-container')
  assert.equal(await x.innerText(),await y.innerText(),label+' text')
  const first=await x.screenshot(),second=await y.screenshot()
  await mkdir(output,{recursive:true});await writeFile(new URL(label+'-new.png',output),first);await writeFile(new URL(label+'-reference.png',output),second)
  assert.ok(first.equals(second),label+' cart pixels differ; inspect ignored screenshots')
  comparisons.push({case:label,identical_pixels:true,sha256:createHash('sha256').update(first).digest('hex')})
 }
 for(const language of ['en','zh'])for(const width of [390,768,1440]){
  const page=await fresh(width),old=await fresh(width),path=`/dk-${language}/cart`,increase=language==='en'?'Increase quantity':'增加数量',decrease=language==='en'?'Decrease quantity':'减少数量'
  await ready(page,path);await ready(old,path+'?reference=1');await compare(page,old,`${language}-${width}-initial`)
  await expect(page.getByTestId('product-row')).toHaveCount(2)
  await expect(page.getByTestId('checkout-button')).toHaveAttribute('href',`/dk-${language}/checkout?step=address`)
  await expect(page.getByTestId('product-title').first()).toHaveAttribute('href',`/dk-${language}/products/iphone-13-256gb-blue-A100`)
  // Hold both artificial transports to compare the original pending state.
  const releases=[]
  for(const p of [page,old])await p.route('**/test-cart',async route=>{await new Promise(resolve=>releases.push(resolve));await route.fulfill({status:200,json:{}})})
  for(const p of [page,old])await p.getByRole('button',{name:increase,exact:true}).first().click()
  for(const p of [page,old]){await expect(p.getByTestId('product-row').first()).toHaveAttribute('aria-busy','true');await expect(p.getByRole('button',{name:increase,exact:true}).last()).toBeDisabled()}
  await compare(page,old,`${language}-${width}-pending`)
  assert.equal(releases.length,2);releases.forEach(resolve=>resolve())
  for(const p of [page,old])await expect(p.getByTestId('product-select-button').first()).toHaveValue('3')
  for(const p of [page,old])await expect(p.getByTestId('product-row').first()).toHaveAttribute('aria-busy','false')
  await compare(page,old,`${language}-${width}-quantity`)
  for(const p of [page,old]){await p.unroute('**/test-cart');await p.route('**/test-cart',r=>r.fulfill({status:500,json:{}}));await p.getByRole('button',{name:decrease,exact:true}).first().click();await expect(p.getByTestId('product-error-message')).toBeVisible()}
  await compare(page,old,`${language}-${width}-update-error`)
  for(const p of [page,old]){await p.getByTestId('product-delete-button').last().click();await expect(p.getByTestId('product-error-message')).toHaveCount(2)}
  await compare(page,old,`${language}-${width}-remove-error`)
  for(const p of [page,old]){await p.getByTestId('product-select-button').first().fill('5');await p.getByTestId('product-select-button').first().press('Enter');await expect(p.getByTestId('product-select-button').first()).toHaveValue('3')}
  await compare(page,old,`${language}-${width}-invalid-input`)
  for(const p of [page,old]){await p.unroute('**/test-cart');await p.route('**/test-cart',r=>r.fulfill({status:200,json:{}}));await p.getByTestId('product-select-button').first().fill('4');await p.getByTestId('product-select-button').first().press('Enter');await expect(p.getByRole('button',{name:increase,exact:true}).first()).toBeDisabled();await expect(p.getByTestId('product-row').first()).toHaveAttribute('aria-busy','false')}
  await compare(page,old,`${language}-${width}-maximum`)
  const removals=[]
  for(const p of [page,old]){await p.unroute('**/test-cart');await p.route('**/test-cart',async route=>{await new Promise(resolve=>removals.push(resolve));await route.fulfill({status:200,json:{}})});await p.getByTestId('product-delete-button').first().click();await expect(p.getByTestId('product-delete-button').first()).toHaveText(language==='en'?'Removing…':'正在移除…')}
  await compare(page,old,`${language}-${width}-remove-pending`)
  assert.equal(removals.length,2);removals.forEach(resolve=>resolve())
  for(const p of [page,old]){await expect(p.getByTestId('product-row')).toHaveCount(1);await p.unroute('**/test-cart');await p.route('**/test-cart',r=>r.fulfill({status:200,json:{}}));await p.getByTestId('product-delete-button').click();await expect(p.getByTestId('empty-cart-message')).toBeVisible()}
  await compare(page,old,`${language}-${width}-removed-empty`)
  for(const mode of ['unavailable','missing','demo','empty','totals']){await ready(page,path+'?mode='+mode);await ready(old,path+'?reference=1&mode='+mode);await compare(page,old,`${language}-${width}-${mode}`)}
  await page.close();await old.close()
 }
 const page=await fresh(390)
 await ready(page,'/dk-en/cart?mode=closed');await expect(page.getByTestId('checkout-button')).toHaveCount(0);await expect(page.getByRole('status')).toHaveText('Proceed to checkout')
 await ready(page,'/dk-en/cart?mode=readonly');for(const input of await page.getByTestId('product-select-button').all())await expect(input).toBeDisabled()
 await ready(page,'/dk-en/cart?mode=large');await expect(page.getByTestId('product-price').first()).toHaveText('€9,007,199,254,740,993.01')
 assert.deepEqual(errors,[]);await context.close()
 console.log('ASTRO_CART_BROWSER '+JSON.stringify({scope:'isolated original CartTemplate/CartLine/Summary oracle; artificial command boundary, no Gin or full page acceptance',original_copy_and_css_preserved:true,comparisons,quantity_pending_and_failure:true,enter_input_and_maximum:true,remove_pending_failure_and_empty:true,saved_shipping_discount_and_signed_in_layout:true,closed_checkout:true,readonly_controls:true,exact_large_amount:true,hydration_errors:errors.length}))
}finally{await browser?.close();if(server)await server.stop()}
