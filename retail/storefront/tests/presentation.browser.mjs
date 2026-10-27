import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {mkdir,writeFile,readFile} from 'node:fs/promises'
import {createHash} from 'node:crypto'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacyPresentation} from './legacy-presentation.mjs'
const require=createRequire(new URL('../../admin/package.json',import.meta.url))
const {chromium,expect}=require('@playwright/test')
const output=new URL('../../../logs/gin-migration/astro-presentation/',import.meta.url)
let server,browser
try {
 const source=new URL('../../../medusa-store/apps/storefront/src/modules/products/',import.meta.url)
 const normalized=text=>text.replaceAll('\r\n','\n')
 assert.equal(normalized(await readFile(new URL('i18n.ts',source),'utf8')),normalized(await readFile(new URL('../src/components/products/i18n.ts',import.meta.url),'utf8')))
 assert.ok(normalized(await readFile(new URL('../src/components/products/styles.module.css',import.meta.url),'utf8')).startsWith(normalized(await readFile(new URL('components/product-detail/styles.module.css',source),'utf8'))))
 server=await dev({root:new URL('./presentation-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],vite:{plugins:[legacyPresentation()],resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 const origin=`http://127.0.0.1:${server.address.port}`
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({reducedMotion:'reduce'}),errors=[],comparisons=[]
 const fresh=async()=>{const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});return page}
 const ready=async(page,path)=>{await page.goto(origin+path);await expect(page.locator('main')).toHaveAttribute('data-ready','true');await page.locator('img').evaluateAll(async images=>{await Promise.all(images.map(im=>im.decode()))});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)}
 const compare=async(page,reference,label)=>{
  for(const region of ['gallery','details']) {
   const current=page.getByTestId(region+'-region'),old=reference.getByTestId(region+'-region')
   assert.equal(await current.innerText(),await old.innerText(),label+' '+region+' text')
   const a=await current.screenshot(),b=await old.screenshot(),same=a.equals(b)
   await mkdir(output,{recursive:true});await writeFile(new URL(label+'-'+region+'-new.png',output),a);await writeFile(new URL(label+'-'+region+'-reference.png',output),b)
   assert.ok(same,label+' '+region+' screenshot differs; inspect ignored screenshots')
   comparisons.push({case:label,region,identical_pixels:true,sha256:createHash('sha256').update(a).digest('hex')})
  }
 }
 for(const locale of ['en','zh'])for(const width of [390,768,1440]) {
  const page=await fresh(),reference=await fresh(),path=`/dk-${locale}/products/swappie-iphone-13`
  await page.setViewportSize({width,height:900});await reference.setViewportSize({width,height:900})
  await ready(page,path);await ready(reference,path+'?reference=1');await compare(page,reference,`${locale}-${width}-initial`)
  const gallery=page.getByTestId('gallery-region'),oldGallery=reference.getByTestId('gallery-region'),next=locale==='zh'?'下一张图片':'Next image',previous=locale==='zh'?'上一张图片':'Previous image',image=locale==='zh'?'商品图片':'Product image'
  await gallery.getByRole('button',{name:previous,exact:true}).click();await oldGallery.getByRole('button',{name:previous,exact:true}).click()
  await expect(gallery.getByRole('button',{name:image+' 3',exact:true})).toHaveAttribute('aria-pressed','true')
  await gallery.getByRole('button',{name:next,exact:true}).click();await oldGallery.getByRole('button',{name:next,exact:true}).click()
  await expect(gallery.getByRole('button',{name:image+' 1',exact:true})).toHaveAttribute('aria-pressed','true')
  await gallery.getByRole('button',{name:image+' 2',exact:true}).click();await oldGallery.getByRole('button',{name:image+' 2',exact:true}).click()
  // Clear equivalent focus/hover before comparing pixels after interaction.
  for(const p of [page,reference]){await p.locator('body').click({position:{x:1,y:1}});await p.mouse.move(0,0)}
  await compare(page,reference,`${locale}-${width}-image-two`)
  await page.getByText(locale==='zh'?'关于这款 iPhone':'About this iPhone',{exact:true}).click()
  await reference.getByText(locale==='zh'?'关于这款 iPhone':'About this iPhone',{exact:true}).click()
  await expect(page.getByTestId('product-description')).toContainText('<script>no HTML</script>')
  await compare(page,reference,`${locale}-${width}-description`)
  const link=page.getByRole('link',{name:locale==='zh'?'Apple 技术规格':'Apple technical specifications'})
  await expect(link).toHaveAttribute('href','https://support.apple.com/en-us/111872');await expect(link).toHaveAttribute('rel','noopener noreferrer')
  await page.getByRole('button',{name:'Choose 256',exact:true}).click()
  await expect(gallery.locator('img')).toHaveCount(1);await expect(gallery.locator('img')).toHaveAttribute('src','/images/256.svg');await expect(gallery.getByRole('button')).toHaveCount(0)
  await page.getByRole('button',{name:'Choose 512',exact:true}).click()
  await expect(gallery.getByRole('button',{name:image+' 1',exact:true})).toHaveAttribute('aria-pressed','true');await expect(gallery.locator('img').first()).toHaveAttribute('src','/images/model.svg')
  // Keyboard focus remains visible in the migrated component.
  await gallery.getByRole('button',{name:previous,exact:true}).focus();await page.keyboard.press('Tab')
  assert.equal(await gallery.getByRole('button',{name:next,exact:true}).evaluate(el=>getComputedStyle(el).outlineWidth),'3px')
  await page.close();await reference.close()
 }
 for(const mode of ['empty','unsafe']) {
  const page=await fresh(),reference=await fresh(),path='/dk-en/products/swappie-iphone-13?mode='+mode
  await ready(page,path);await ready(reference,path+'&reference=1');await compare(page,reference,mode)
  await expect(page.getByRole('link')).toHaveCount(0)
  if(mode==='empty'){await expect(page.getByText('Product image coming soon')).toBeVisible();await expect(page.getByTestId('details-region').locator('details')).toHaveCount(0)}
  await page.close();await reference.close()
 }
 assert.deepEqual(errors,[])
 await context.close()
 console.log('ASTRO_PRESENTATION_BROWSER '+JSON.stringify({scope:'isolated artificial fixture; original source oracle with Next navigation/image bridges; not full page acceptance',original_copy_and_css_preserved:true,comparisons,gallery_wrap_thumbnail_and_selection_reset:true,description_and_safe_source:true,keyboard_focus:true,no_overflow:true,hydration_errors:0}))
}finally{if(browser)await browser.close();if(server)await server.stop()}
