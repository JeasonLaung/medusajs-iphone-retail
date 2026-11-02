import assert from 'node:assert/strict'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'
import {getUiCopy} from '../../storefront/src/components/ui-showcase/i18n.ts'

const output=fileURLToPath(new URL('../../../logs/gin-migration/ui-showcase-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true,mode:0o700})
const browser=await chromium.launch({channel:'chrome',headless:true})
const rows=[],errors=[],writes=[],developmentDiagnostics=[]
try{
 for(const platform of ['source','astro'])for(const lang of ['en','zh'])for(const width of [390,1440]){
  const context=await browser.newContext({reducedMotion:'reduce'})
  await context.route('https://**/*',r=>r.abort())
  const page=await context.newPage();page.setDefaultTimeout(15000)
  page.on('pageerror',e=>errors.push({platform,lang,width,message:e.message}))
  page.on('request',r=>{
   if(['GET','HEAD','OPTIONS'].includes(r.method()))return
   const url=new URL(r.url()),record={platform,method:r.method(),path:url.pathname}
   // Next's development overlay reads source stack frames with POST. Record
   // this exact source-only diagnostic separately from business/API writes.
   if(platform==='source'&&url.origin==='http://127.0.0.1:8000'&&url.pathname==='/__nextjs_original-stack-frames'&&r.method()==='POST')developmentDiagnostics.push(record)
   else writes.push(record)
  })
  await page.setViewportSize({width,height:900})
  const c=getUiCopy(lang==='zh'?'zh-CN':'en')
  const url=`http://127.0.0.1:${platform==='source'?8000:9300}/dk-${lang}/ui`
  const response=await page.goto(url);assert.equal(response.status(),200)
  await expect(page.getByRole('heading',{name:c.title,level:1})).toBeVisible()
  await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
  await page.waitForLoadState('networkidle')
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute('content',/noindex.*nofollow/)
  const sections=await page.locator('section[id]').evaluateAll(nodes=>nodes.map(n=>({id:n.id,heading:n.querySelector('h2')?.textContent})))
  assert.deepEqual(sections.map(s=>s.id),['actions','forms','shared','commerce','feedback','data','overlays'])
  const initialText=await page.locator('#shared').innerText()
  const country=page.getByTestId('ui-european-country-search')
  await country.fill('Denmark')
  await page.getByRole('option',{name:/Denmark|丹麦/}).click()
  await expect(page.getByTestId('ui-european-country')).toHaveValue('dk')
  await page.locator('#forms input[type="file"]').setInputFiles({name:'demo.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGMQK///HwAEvwKM9Ew8SgAAAABJRU5ErkJggg==','base64')})
  await expect(page.locator('#forms .ant-upload-list-item-name')).toHaveText('demo.png')
  await expect.poll(()=>page.locator('#forms .ant-upload-list-item-thumbnail img').evaluate(img=>img.complete&&img.naturalWidth>0)).toBe(true)
  await page.getByRole('group',{name:c.rating,exact:true}).getByRole('radio',{name:c.stars[4],exact:true}).click()
  await expect(page.getByRole('group',{name:c.rating,exact:true})).toContainText(c.stars[4])
  await page.locator('#shared select').selectOption('iPhone 14')
  await page.locator('#actions').getByRole('button',{name:c.primary,exact:true}).click()
  await expect(page.locator('#actions').getByRole('alert')).toContainText(c.clicked)
  const selections=page.getByTestId('selection-row-demo')
  await expect(selections.locator('input[name="ui-selection-storage"][value="512 GB"]')).toBeChecked()
  await selections.locator('label').filter({has:page.locator('input[value="256 GB"]')}).click()
  await expect(selections.locator('input[name="ui-selection-storage"][value="256 GB"]')).toBeChecked()
  await expect(selections.locator('input[name="ui-selection-storage"]:checked')).toHaveCount(1)
  const charger=selections.locator('input[value="charger"]')
  await charger.check();await expect(charger).toBeChecked();await charger.uncheck()
  await expect(selections.locator('input[value="disabled"]')).toBeDisabled()
  await page.locator('#data').getByRole('listitem',{name:'2',exact:true}).click()
  await expect(page.locator('#data .ant-pagination-item-2')).toHaveAttribute('class',/ant-pagination-item-active/)
  await page.locator('#data').getByRole('menuitem',{name:c.details,exact:true}).click()
  await expect(page.locator('#data .ant-timeline')).toContainText(c.timeline[0])
  const form=page.locator('#overlays form')
  await form.getByRole('button',{name:c.submit,exact:true}).click()
  await expect(form.getByText(c.required,{exact:true}).first()).toBeVisible()
  await form.getByLabel(c.name,{exact:true}).fill('Local demo')
  await form.getByLabel(c.email,{exact:true}).fill('invalid')
  await form.getByRole('button',{name:c.submit,exact:true}).click()
  await expect(form.getByText(c.invalidEmail,{exact:true})).toBeVisible()
  await form.getByLabel(c.email,{exact:true}).fill('demo@example.test')
  await form.getByRole('button',{name:c.submit,exact:true}).click()
  await expect(page.getByRole('status').filter({hasText:c.saved})).toBeVisible()
  // Ant Design inserts a visual space in two-character Chinese button labels.
  await form.getByRole('button',{name:new RegExp('^'+c.reset.split('').join('\\s*')+'$')}).click()
  await expect(form.getByLabel(c.name,{exact:true})).toHaveValue('')
  for(const button of [c.openAccount,c.openShared]){
   const trigger=page.getByRole('button',{name:button,exact:true})
   await trigger.click()
   // Headless UI's dialog root has zero geometry; its fixed-position panel is visible.
   await expect(page.getByRole('dialog').getByText(c.modalBody,{exact:true})).toBeVisible()
   await expect.poll(()=>page.getByRole('dialog').evaluate(node=>node.contains(document.activeElement))).toBe(true)
   await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0)
  }
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,`${platform}-${lang}-${width}.png`)})
  await page.reload()
  await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
  await expect(page.locator('input[name="ui-selection-storage"][value="512 GB"]')).toBeChecked()
  rows.push({platform,lang,width,status:200,sections,initialText,country_search:true,upload_local_only:true,rating:true,native_select:true,selection:true,pagination:true,timeline:true,form_validation_and_reset:true,dialogs_focus_and_escape:true,reload_resets:true,overflow:false})
  console.log(JSON.stringify({platform,lang,width,passed:true}))
  await context.close()
 }
 const comparisons=rows.filter(r=>r.platform==='source').map(s=>{
  const t=rows.find(t=>t.platform==='astro'&&t.lang===s.lang&&t.width===s.width)
  return {lang:s.lang,width:s.width,section_headings_equal:JSON.stringify(s.sections)===JSON.stringify(t.sections),shared_initial_text_equal:s.initialText===t.initialText}
 })
 const report={output,rows,comparisons,errors,writes,developmentDiagnostics,actual_local_services:true,api_mocking:false,business_writes:false,full_storefront_accepted:false,goal_complete:false}
 await writeFile(join(output,'report.json'),JSON.stringify(report,null,2)+'\n')
 console.log(JSON.stringify({output,cases:rows.length,comparisons,errors,writes,developmentDiagnostics}))
 assert.deepEqual(errors,[]);assert.deepEqual(writes,[])
 assert.ok(comparisons.every(c=>c.section_headings_equal&&c.shared_initial_text_equal))
}finally{await browser.close()}
