import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {join} from 'node:path'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
import {verifyNativeCollections} from './collections.native.mjs'
import {verifyNativeSearch} from './search.native.mjs'

const port=Number(process.env.ADMIN_TEST_PORT),origin=`http://127.0.0.1:${port}`,output=process.env.OPTION_BROWSER_OUTPUT
const astro=process.env.OPTION_ASTRO_ORIGIN
if(!port||!output||!process.env.ADMIN_TEST_API_TARGET||!process.env.ADMIN_EMAIL||!process.env.ADMIN_PASSWORD)throw Error('Owned option fixture required')
if(astro&&!/^http:\/\/127\.0\.0\.1:\d+$/.test(astro))throw Error('Owned Astro origin required')
let server,browser,page,policyProof,scopeProof,regionalProof,collectionProof,searchProof
const errors=[],commands=[]
try{
 server=await createServer({server:{port,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{channel:'chrome'})})
 const context=await browser.newContext({reducedMotion:'reduce'})
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 page=await context.newPage();page.setDefaultTimeout(45000);page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/account')
 await page.getByLabel('Email',{exact:true}).fill(process.env.ADMIN_EMAIL)
 await page.getByLabel('Password',{exact:true}).fill(process.env.ADMIN_PASSWORD)
 await page.getByRole('button',{name:'Sign in',exact:true}).click()
 await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 const csrf=(await context.cookies()).find(c=>c.name==='retail_admin_csrf')?.value;assert.ok(csrf)
 const get=async path=>{const response=await context.request.get(origin+'/api/admin/'+path);assert.equal(response.status(),200);return response.json()}
 const post=async(path,data,status=200,headers={Origin:origin,'X-CSRF-Token':csrf})=>{const response=await context.request.post(origin+'/api/admin/'+path,{headers,data});assert.equal(response.status(),status);return response.json()}

 if(astro){
  await page.setViewportSize({width:1440,height:1000})
  await page.goto(origin+'/buy-policies')
  await expect(page.getByLabel('Warranty months',{exact:true})).toBeEnabled()
  await page.getByLabel('Warranty months',{exact:true}).fill('36')
  await page.getByLabel('Return days',{exact:true}).fill('21')
  await page.getByLabel('English',{exact:true}).nth(0).fill('Warranty from Admin')
  await page.getByLabel('Chinese',{exact:true}).nth(0).fill('后台保修备注')
  await page.getByLabel('English',{exact:true}).nth(1).fill('Returns from Admin')
  await page.getByLabel('Chinese',{exact:true}).nth(1).fill('后台退货备注')
  const policyCommands=[];let policyDropped=false
  await page.route('**/api/admin/buy-policies',async route=>{
   if(route.request().method()!=='POST'){await route.continue();return}
   policyCommands.push(route.request().postDataJSON());const response=await route.fetch()
   assert.equal(response.status(),200)
   if(!policyDropped){policyDropped=true;await route.abort('failed')}else await route.fulfill({response})
  })
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByText('The result is not confirmed. Retry the same operation.',{exact:true})).toBeVisible()
  await page.getByRole('button',{name:'Retry same operation',exact:true}).click()
  await expect(page.getByText('Saved',{exact:true})).toBeVisible()
  await expect(page.getByLabel('Warranty months',{exact:true})).toHaveValue('36')
  assert.equal(policyCommands.length,2);assert.deepEqual(policyCommands[0],policyCommands[1])
  await page.screenshot({path:join(output,'purchase-policies-admin-en.png'),fullPage:true})
  await page.unroute('**/api/admin/buy-policies')
  const actual=await get('buy-policies');assert.equal(actual.policies.return_days,21)
  // Reject transport failures even for a previously completed UUID.
  await post('buy-policies',policyCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)})
  await post('buy-policies',policyCommands[0],403,{'X-CSRF-Token':csrf})
  await post('buy-policies',policyCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  const anonymousPolicy=await browser.newContext()
  assert.equal((await anonymousPolicy.request.get(origin+'/api/admin/buy-policies')).status(),401)
  await anonymousPolicy.close()
  assert.deepEqual(await get('buy-policies'),actual)
  const storefront=await context.newPage()
  for(const lang of ['en','zh'])for(const width of [1440,390]){
   await storefront.setViewportSize({width,height:1000});const response=await storefront.goto(astro+'/dk-'+lang+'/store')
   assert.equal(response.status(),200);const html=await response.text();assert.ok(html.includes('36'));assert.ok(html.includes(lang==='en'?'Warranty from Admin':'后台保修备注'))
   await expect(storefront.getByText(lang==='en'?'Warranty from Admin':'后台保修备注',{exact:true})).toBeVisible()
   assert.equal(await storefront.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
   await storefront.screenshot({path:join(output,'purchase-policies-'+lang+'-'+width+'.png'),fullPage:true})
  }
  await storefront.close()
  policyProof={actual_admin_save:true,actual_origin_csrf_anonymous_denials:true,same_uuid_response_loss_retry:true,operation_id:policyCommands[0].operation_id,actual_astro_en_zh_widths:[1440,390],periods:[36,21],notes_visible:true}
 }
 await page.goto(origin+'/option-definitions')
 const title='Browser shared %_! '+randomUUID(),section=page.getByTestId('option-definitions')
 const resetFilters=async()=>{
  await Promise.all([
   page.waitForResponse(response=>{const u=new URL(response.url());return response.request().method()==='GET'&&u.pathname==='/api/admin/option-definitions'&&u.search==='?offset=0'&&response.status()===200}),
   section.getByRole('button',{name:'Reset',exact:true}).click(),
  ])
  await expect(section.locator('.ant-spin-spinning')).toHaveCount(0)
  await expect(section.getByRole('button',{name:'Create shared option',exact:true})).toBeEnabled()
 }
 const chooseScope=async(label,value)=>{
  const input=section.getByLabel(label,{exact:true})
  await input.locator('xpath=ancestor::div[contains(@class,"ant-select-selector")][1]').click()
  const option=page.locator('.ant-select-dropdown:visible').getByTitle(value,{exact:true})
  await expect(option).toBeVisible();await option.click()
  await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await expect(input.locator('xpath=ancestor::div[contains(@class,"ant-select-selector")][1]').getByText(value,{exact:true})).toBeVisible()
 }
 // Inspect source scopes through the actual Ant filters and immutable records.
 await page.waitForLoadState('networkidle')
 await chooseScope('Definition type','Product-specific')
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 const scoped=section.getByRole('row').filter({hasText:'Scope browser product-specific'})
 await expect(scoped).toHaveCount(1)
 await expect(scoped.getByRole('button',{name:'Edit',exact:true})).toBeDisabled()
 assert.equal(new URL(page.url()).searchParams.get('is_exclusive'),'true')
 await page.reload();await expect(scoped).toHaveCount(1);await page.waitForLoadState('networkidle')
 await chooseScope('Definition type','Shared')
 await chooseScope('Deleted records','Included')
 await section.getByLabel('ID',{exact:true}).fill('scope-browser-deleted')
 const deletedFrom=section.getByRole('textbox',{name:'Deleted from',exact:true})
 await deletedFrom.fill('2000-01-01 00:00:00.000');await deletedFrom.press('Enter');await deletedFrom.press('Tab')
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 const deletedRow=section.getByRole('row').filter({hasText:'Scope browser deleted'})
 await expect(deletedRow).toHaveCount(1)
 await expect(deletedRow.getByRole('button',{name:'Edit',exact:true})).toBeDisabled()
 await expect(deletedRow.getByRole('button',{name:'Delete',exact:true})).toBeDisabled()
 assert.equal(new URL(page.url()).searchParams.get('with_deleted'),'true')
 await page.screenshot({path:join(output,'option-scopes-en.png'),fullPage:true})
 const nested=new URL(origin+'/option-definitions')
 nested.searchParams.set('with_deleted','true')
 nested.searchParams.set('$and[0][$or][0][id]','scope-browser-deleted')
 nested.searchParams.set('$and[0][$or][1][id]','scope-browser-exclusive')
 nested.searchParams.set('$and[1][deleted_at][$gte]','2000-01-01T00:00:00.123456Z')
 await page.goto(nested.href);await expect(deletedRow).toHaveCount(1)
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 await expect.poll(()=>new URL(page.url()).searchParams.get('$and[0][$or][1][id]')).toBe('scope-browser-exclusive')
 await page.reload();await expect(deletedRow).toHaveCount(1)
 scopeProof={actual_ant_scope_filters:true,exclusive_readonly:true,deleted_readonly:true,deleted_date_filter:true,nested_and_or_url_reload:true,scope_outside_or:true}
 await resetFilters()
 await section.getByRole('button',{name:'Create shared option',exact:true}).click()
 let editor=page.getByRole('dialog')
 await editor.getByLabel('Name',{exact:true}).fill(title)
 await editor.getByLabel('Option value',{exact:true}).fill('128GB')
 await editor.getByRole('button',{name:'Add value',exact:true}).click()
 await editor.getByLabel('Option value',{exact:true}).nth(1).fill('256GB')
 await editor.getByLabel('Rank',{exact:true}).nth(1).fill('2')
 let dropped=false
 await page.route('**/api/admin/option-definitions',async route=>{
  if(route.request().method()!=='POST')return route.continue()
  commands.push(route.request().postDataJSON())
  if(!dropped){const response=await route.fetch();assert.equal(response.status(),200);dropped=true;await route.abort('failed')}
  else await route.continue()
 })
 await editor.getByRole('button',{name:'Save',exact:true}).click()
 await expect(editor.getByText('The result is not confirmed. Retry the same operation.',{exact:true})).toBeVisible()
 await expect(editor.getByLabel('Name',{exact:true})).toBeDisabled()
 await editor.getByRole('button',{name:'Retry this save',exact:true}).click()
 await expect(editor).toHaveCount(0)
 assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1]);await page.unroute('**/api/admin/option-definitions')
 let listed=await get('option-definitions?offset=0'),definition=listed.options.find(d=>d.title===title)
 assert.ok(definition);assert.equal(listed.options.filter(d=>d.title===title).length,1)
 const ids=Object.fromEntries(definition.values.map(v=>[v.value,v.id]))
 // Real filter form, URL refresh, empty result and reset; no read mocks.
 await section.getByLabel('Search name',{exact:true}).fill(title)
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 await expect.poll(()=>new URL(page.url()).searchParams.get('q')).toBe(title)
 await page.reload()
 await expect(section.getByLabel('Search name',{exact:true})).toHaveValue(title)
 await expect(section.getByRole('row').filter({hasText:title})).toHaveCount(1)
 await section.getByLabel('ID',{exact:true}).fill('missing-filter-id')
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 await expect(section.getByText('No shared options',{exact:true})).toBeVisible()
 await resetFilters()
 await expect(section.getByRole('row').filter({hasText:title})).toHaveCount(1)
 await expect(section.getByLabel('Search name',{exact:true})).toHaveValue('')
 await section.getByLabel('Exact name',{exact:true}).fill(title)
 await section.getByLabel('ID',{exact:true}).fill(definition.id)
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 await expect.poll(async()=>{const data=await get('option-definitions?'+new URL(page.url()).searchParams);return data.count}).toBe(1)
 await resetFilters()
 const anyID=section.getByRole('combobox',{name:'Any ID',exact:true})
 await anyID.fill(definition.id);await anyID.press('Enter');await anyID.fill('missing-array-member');await anyID.press('Enter')
 const anyName=section.getByRole('combobox',{name:'Any name',exact:true})
 await anyName.fill(title);await anyName.press('Enter');await anyName.press('Tab')
 for(const [label,value] of [['Created from','2000-01-01 00:00:00.000'],['Created through','2100-01-01 00:00:00.000'],['Updated from','2000-01-01 00:00:00.000'],['Updated through','2100-01-01 00:00:00.000']]){
  const input=section.getByRole('textbox',{name:label,exact:true});await input.fill(value);await input.press('Enter');await input.press('Tab')
 }
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 await expect.poll(()=>new URL(page.url()).searchParams.getAll('id[]').length).toBe(2)
 await expect.poll(()=>new URL(page.url()).searchParams.get('created_at[$gte]')).not.toBe(null)
 await expect.poll(async()=>{const data=await get('option-definitions?'+new URL(page.url()).searchParams);return data.count}).toBe(1)
 await page.reload()
 await expect(section.getByRole('row').filter({hasText:title})).toHaveCount(1)
 await expect(section.getByRole('textbox',{name:'Created from',exact:true})).not.toHaveValue('')
 const microURL=new URL(page.url()),microBoundary='2000-01-01T00:00:00.123456Z'
 microURL.searchParams.set('created_at[$gte]',microBoundary)
 await page.goto(microURL.href)
 await expect(section.getByRole('row').filter({hasText:title})).toHaveCount(1)
 await section.getByRole('button',{name:'Apply filters',exact:true}).click()
 await expect.poll(()=>new URL(page.url()).searchParams.get('created_at[$gte]')).toBe(microBoundary)
 await resetFilters()
 let row=section.getByRole('row').filter({hasText:title})
 await row.getByRole('button',{name:'Edit',exact:true}).click();editor=page.getByRole('dialog')
 await editor.getByLabel('Name',{exact:true}).fill(title+' edited')
 await editor.getByLabel('Rank',{exact:true}).first().fill('-3')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 definition=await get('option-definitions/'+definition.id)
 assert.ok(definition);assert.deepEqual(Object.fromEntries(definition.values.map(v=>[v.value,v.id])),ids)
 assert.equal(definition.values.find(v=>v.value==='128GB').rank,-3)
 let currentTitle=title+' edited'
 // Metadata uses the real endpoint, shallow merge and immutable retry.
 await post('option-definitions/'+definition.id+'/metadata',{operation_id:randomUUID(),expected_revision:definition.revision,metadata:{keep:'unchanged',remove:'old',structured:{n:1}}})
 await page.reload()
 row=section.getByRole('row').filter({hasText:currentTitle})
 await row.getByRole('button',{name:'Metadata',exact:true}).click()
 let metadataEditor=page.getByRole('dialog',{name:'Edit metadata',exact:true})
 await expect(metadataEditor.getByLabel('Key',{exact:true})).toHaveCount(3)
 const metadataKeys=await metadataEditor.getByLabel('Key',{exact:true}).evaluateAll(inputs=>inputs.map(i=>i.value))
 const structuredIndex=metadataKeys.indexOf('structured'),removeIndex=metadataKeys.indexOf('remove')
 await expect(metadataEditor.getByLabel('Key',{exact:true}).nth(structuredIndex)).toBeDisabled()
 await expect(metadataEditor.getByRole('button',{name:'Remove row',exact:true}).nth(structuredIndex)).toBeDisabled()
 await metadataEditor.getByRole('button',{name:'Remove row',exact:true}).nth(removeIndex).click()
 await metadataEditor.getByRole('button',{name:'Add metadata',exact:true}).click()
 await metadataEditor.getByLabel('Key',{exact:true}).last().fill('enabled')
 await metadataEditor.getByLabel('Value',{exact:true}).last().fill('true')
 const metadataCommands=[],metadataPath='/api/admin/option-definitions/'+definition.id+'/metadata'
 let metadataDropped=false
 await page.route('**'+metadataPath,async route=>{
  metadataCommands.push(route.request().postDataJSON())
  if(!metadataDropped){const response=await route.fetch();assert.equal(response.status(),200);metadataDropped=true;await route.abort('failed')}
  else await route.continue()
 })
 await metadataEditor.getByRole('button',{name:'Save metadata',exact:true}).click()
 await expect(metadataEditor.getByText('The result is not confirmed. Retry the same operation.',{exact:true})).toBeVisible()
 await expect(metadataEditor.getByLabel('Key',{exact:true}).first()).toBeDisabled()
 await metadataEditor.getByRole('button',{name:'Retry same operation',exact:true}).click()
 await expect(metadataEditor).toHaveCount(0)
 assert.equal(metadataCommands.length,2);assert.deepEqual(metadataCommands[0],metadataCommands[1]);await page.unroute('**'+metadataPath)
 definition=await get('option-definitions/'+definition.id)
 assert.deepEqual(JSON.parse(definition.metadata_json),{enabled:true,keep:'unchanged',structured:{n:1}})
 const product=await get('products/option-browser-product/options')
 if(astro){
  await page.goto(origin+'/products/option-browser-product')
  await page.getByRole('button',{name:'Manage options',exact:true}).click();editor=page.getByRole('dialog',{name:'Manage options',exact:true})
  const choose=async(name,value)=>{const input=editor.getByRole('combobox',{name,exact:true});await input.click();await input.fill(value);await page.locator('.ant-select-dropdown:visible').getByTitle(value,{exact:true}).click()}
  await choose('Add shared option',definition.title)
  for(const value of definition.values)await choose('Option values '+definition.title,value.value)
  await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 }else await post('products/option-browser-product/options',{operation_id:randomUUID(),expected_revision:product.revision,options:[{id:definition.id,title:definition.title,values:definition.values.map(v=>({id:v.id,value:v.value}))}]})
 await page.goto(origin+'/option-definitions');row=section.getByRole('row').filter({hasText:currentTitle})
 await expect(row.getByRole('button',{name:'Delete',exact:true})).toBeDisabled()
 await row.getByRole('button',{name:'Manage option values',exact:true}).click()
 await expect(section.getByRole('button',{name:'Delete option value',exact:true})).toHaveCount(2)
 for(const button of await section.getByRole('button',{name:'Delete option value',exact:true}).all())await expect(button).toBeDisabled()
 await row.getByRole('button',{name:'Hide option values',exact:true}).click()
 await row.getByRole('button',{name:'Edit',exact:true}).click();editor=page.getByRole('dialog')
 await expect(editor.getByText('Associated with products',{exact:true})).toHaveCount(2)
 await editor.getByRole('button',{name:'Cancel',exact:true}).click()
 definition=await get('option-definitions/'+definition.id)
 await post('option-definitions/'+definition.id+'/values/'+ids['128GB']+'/remove',{operation_id:randomUUID(),expected_revision:definition.revision},409)
 const rejected={operation_id:randomUUID(),expected_revision:'0'.repeat(64),title:'Rejected '+randomUUID(),values:['x']}
 await post('option-definitions',rejected,403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)})
 await post('option-definitions',rejected,403,{'X-CSRF-Token':csrf})
 const anonymous=await browser.newContext()
 const denied=await anonymous.request.get(origin+'/api/admin/option-definitions?offset=0');assert.equal(denied.status(),401);await anonymous.close()
 await section.getByRole('button',{name:'Create shared option',exact:true}).click();editor=page.getByRole('dialog')
 const removable='Remove shared '+randomUUID();await editor.getByLabel('Name',{exact:true}).fill(removable)
 await editor.getByLabel('Option value',{exact:true}).fill('Unused')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const removableDefinition=(await get('option-definitions?offset=0')).options.find(d=>d.title===removable),removableID=removableDefinition.id
 await section.getByRole('row').filter({hasText:removable}).getByRole('button',{name:'Manage option values',exact:true}).click()
 const valueCommands=[];let valueResponseDropped=false
 const valuePath='/api/admin/option-definitions/'+removableID+'/values/'+removableDefinition.values[0].id+'/remove'
 await page.route('**'+valuePath,async route=>{
  valueCommands.push(route.request().postDataJSON())
  if(!valueResponseDropped){const response=await route.fetch();assert.equal(response.status(),200);valueResponseDropped=true;await route.abort('failed')}
  else await route.continue()
 })
 await section.getByRole('button',{name:'Delete option value',exact:true}).click()
 await page.getByRole('dialog').getByRole('button',{name:'Delete option value',exact:true}).click()
 await expect(section.getByText('The result is not confirmed. Retry the same operation.',{exact:true})).toBeVisible()
 await section.getByRole('button',{name:'Retry this save',exact:true}).click()
 await expect(section.getByText('No option values',{exact:true})).toBeVisible()
 assert.equal(valueCommands.length,2);assert.deepEqual(valueCommands[0],valueCommands[1]);await page.unroute('**'+valuePath)
 const emptied=await get('option-definitions/'+removableID)
 assert.equal(emptied.deleted,false);assert.deepEqual(emptied.values,[])
 assert.notEqual(emptied.revision,removableDefinition.revision)
 await section.getByRole('row').filter({hasText:removable}).getByRole('button',{name:'Metadata',exact:true}).click()
 metadataEditor=page.getByRole('dialog',{name:'Edit metadata',exact:true})
 await metadataEditor.getByLabel('Key',{exact:true}).fill('empty-definition')
 await metadataEditor.getByLabel('Value',{exact:true}).fill('0')
 await metadataEditor.getByRole('button',{name:'Save metadata',exact:true}).click();await expect(metadataEditor).toHaveCount(0)
 assert.deepEqual(JSON.parse((await get('option-definitions/'+removableID)).metadata_json),{'empty-definition':0})
 await section.getByRole('row').filter({hasText:removable}).getByRole('button',{name:'Metadata',exact:true}).click()
 metadataEditor=page.getByRole('dialog',{name:'Edit metadata',exact:true})
 await metadataEditor.getByRole('button',{name:'Remove row',exact:true}).click()
 await metadataEditor.getByRole('button',{name:'Save metadata',exact:true}).click();await expect(metadataEditor).toHaveCount(0)
 assert.equal((await get('option-definitions/'+removableID)).metadata_json,null)
 await section.getByRole('row').filter({hasText:removable}).getByRole('button',{name:'Edit',exact:true}).click();editor=page.getByRole('dialog')
 await editor.getByRole('button',{name:'Add value',exact:true}).click()
 await editor.getByLabel('Option value',{exact:true}).fill('Restored')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const restored=await get('option-definitions/'+removableID)
 assert.equal(restored.values.length,1);assert.equal(restored.values[0].value,'Restored')
 assert.notEqual(restored.values[0].id,removableDefinition.values[0].id)
 await section.getByRole('row').filter({hasText:removable}).getByRole('button',{name:'Delete',exact:true}).click()
 await page.getByRole('dialog').getByRole('button',{name:'Delete',exact:true}).click()
 await expect(section.getByRole('row').filter({hasText:removable})).toHaveCount(0)
 assert.equal((await context.request.get(origin+'/api/admin/option-definitions/'+removableID)).status(),404)
 let astroProof=null
 if(astro){
  const products=['option-browser-product','option-browser-product-two']
  const second=await get('products/'+products[1]+'/options')
  await post('products/'+products[1]+'/options',{operation_id:randomUUID(),expected_revision:second.revision,options:[{id:definition.id,title:definition.title,values:definition.values.map(v=>({id:v.id,value:v.value}))}]})
  const skuIDs=[]
  for(const productID of products)for(const value of definition.values){
   const creation=await get('products/'+productID+'/variant-creation')
   const variant=await post('products/'+productID+'/variants',{operation_id:randomUUID(),expected_revision:creation.revision,title:value.value,sku:'',option_value_ids:[value.id]})
   skuIDs.push(variant.id)
  }
  const publicPage=await context.newPage();publicPage.on('pageerror',e=>errors.push(e.message))
  const checkPage=async(productID,segment,expectedTitle,values)=>{
   const response=await publicPage.goto(astro+'/'+segment+'/products/'+productID);assert.equal(response.status(),200)
   assert.ok((await response.text()).includes(expectedTitle),'Shared title must be in initial server HTML')
   await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
   const group=publicPage.getByRole('group',{name:expectedTitle,exact:true});await expect(group).toBeVisible()
   await expect(group.getByTestId('option-radio')).toHaveCount(2)
   for(const radio of await group.getByTestId('option-radio').all()){
    await expect(radio).toBeVisible()
    const label=radio.locator('..');await expect(label).toBeVisible()
    const facts=await label.evaluate(node=>({height:node.getBoundingClientRect().height,width:node.getBoundingClientRect().width,display:getComputedStyle(node).display,visibility:getComputedStyle(node).visibility,opacity:getComputedStyle(node).opacity,text:node.textContent}))
    assert.ok(facts.height>=40&&facts.width>=100&&facts.opacity!=='0',JSON.stringify(facts))
   }
   const radioValues=await group.getByTestId('option-radio').evaluateAll(nodes=>nodes.map(node=>node.getAttribute('aria-label')))
   assert.deepEqual(radioValues,values)
   await expect(publicPage.getByTestId('add-product-button')).toBeDisabled()
   assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  }
  await checkPage(products[0],'dk-en',currentTitle,['128GB','256GB'])
  await page.reload()
  await section.getByRole('row').filter({hasText:currentTitle}).getByRole('button',{name:'Edit',exact:true}).click();editor=page.getByRole('dialog')
  currentTitle=title+' synchronized';await editor.getByLabel('Name',{exact:true}).fill(currentTitle)
  await editor.getByLabel('Rank',{exact:true}).nth(1).fill('-8')
  await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
  definition=await get('option-definitions/'+definition.id)
  assert.deepEqual(Object.fromEntries(definition.values.map(v=>[v.value,v.id])),ids)
  assert.equal(definition.active_products,2)
  assert.deepEqual(JSON.parse(definition.metadata_json),{enabled:true,keep:'unchanged',structured:{n:1}})
  for(const width of [1440,390]){
   await publicPage.setViewportSize({width,height:900})
   for(const segment of ['dk-en','dk-zh'])for(const productID of products)await checkPage(productID,segment,currentTitle,['256GB','128GB'])
  }
  for(const productID of products){
   const response=await context.request.get(origin+'/api/store/products/'+productID+'?region_id=option-region');assert.equal(response.status(),200)
   const publicProduct=(await response.json()).product
   assert.equal(publicProduct.options.find(o=>o.id===definition.id).title,currentTitle)
   assert.deepEqual(publicProduct.options.find(o=>o.id===definition.id).values.map(v=>v.id),[ids['256GB'],ids['128GB']])
   assert.equal(publicProduct.variants.length,2);assert.ok(publicProduct.variants.every(v=>skuIDs.includes(v.id)&&v.configuration_complete))
  }
  // Use the actual price drawer for a region-bound price, including lost response.
  await page.goto(origin+'/products/'+products[0])
  await page.getByRole('button',{name:'View prices',exact:true}).first().click()
  let pricesDrawer=page.getByRole('dialog',{name:'Configured prices',exact:true})
  await pricesDrawer.getByRole('button',{name:'Add base price',exact:true}).click()
  let priceDrawer=page.getByRole('dialog',{name:'Add base price',exact:true})
  const priceCommands=[];let priceDropped=false,createdPriceID,createdPricesPath
  await page.route('**/api/admin/products/*/variants/*/prices',async route=>{
   if(route.request().method()!=='POST'){await route.continue();return}
   priceCommands.push(route.request().postDataJSON());createdPricesPath=new URL(route.request().url()).pathname;const response=await route.fetch();assert.equal(response.status(),200);createdPriceID=(await response.json()).id
   if(!priceDropped){priceDropped=true;await route.abort('failed')}else await route.fulfill({response})
  })
  await priceDrawer.getByLabel('Region (optional)',{exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('Option native region · EUR',{exact:true}).click()
  await priceDrawer.getByLabel('Amount',{exact:true}).fill('123.45')
  await priceDrawer.getByRole('button',{name:'Save',exact:true}).click()
  await expect(priceDrawer.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
  await priceDrawer.getByRole('button',{name:'Retry this save',exact:true}).click()
  await expect(priceDrawer).toHaveCount(0)
  assert.equal(priceCommands.length,2);assert.deepEqual(priceCommands[0],priceCommands[1]);assert.equal(priceCommands[0].region_id,'option-region')
  await page.unroute('**/api/admin/products/*/variants/*/prices')
  await page.screenshot({path:join(output,'regional-price-admin-en.png'),fullPage:true})
  const pricedVariant=priceCommands[0]
  const actualProduct=await context.request.get(origin+'/api/store/products/'+products[0]+'?region_id=option-region')
  assert.equal(actualProduct.status(),200)
  assert.ok((await actualProduct.json()).product.variants.some(v=>v.calculated_price?.calculated_amount==='123.45'))
  for(const lang of ['en','zh'])for(const width of [1440,390]){
   await publicPage.setViewportSize({width,height:900})
   const response=await publicPage.goto(astro+'/dk-'+lang+'/products/'+products[0]);assert.equal(response.status(),200)
   assert.ok((await response.text()).includes('123.45'))
   await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
   await expect(publicPage.getByTestId('add-product-button')).toBeDisabled()
   assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
   await publicPage.screenshot({path:join(output,'regional-price-astro-'+lang+'-'+width+'.png'),fullPage:true})
  }
  regionalProof={actual_ant_save:true,region_id:pricedVariant.region_id,same_uuid_response_loss_retry:true,operation_id:pricedVariant.operation_id,public_quote:'123.45',astro_initial_html:true,en_zh_widths:[1440,390],zero_stock_still_disabled:true}
  // Remove the regional price through the real confirmation drawer. The
  // successful first response is lost; retry must recover its durable receipt.
  assert.ok(createdPriceID);const removalPath=createdPricesPath+'/'+encodeURIComponent(createdPriceID)+'/remove'
  const removeCommands=[];let removeDropped=false
  await page.route('**'+removalPath,async route=>{
   removeCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200)
   if(!removeDropped){removeDropped=true;await route.abort('failed')}else await route.fulfill({response})
  })
  // Inspect the new confirmation in Chinese at mobile width before committing.
  await pricesDrawer.getByRole('button',{name:'Cancel',exact:true}).click()
  await page.getByRole('combobox',{name:'Language',exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('简体中文',{exact:true}).click()
  await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await page.setViewportSize({width:390,height:844})
  await page.getByRole('button',{name:'查看价格',exact:true}).first().click()
  const zhPrices=page.getByRole('dialog',{name:'价格配置',exact:true})
  await zhPrices.getByRole('button',{name:'删除价格',exact:true}).click()
  const zhRemoval=page.getByRole('dialog',{name:'删除价格',exact:true})
  await expect(zhRemoval.getByText('123.45',{exact:true})).toBeVisible()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,'price-removal-confirm-zh-390.png'),fullPage:true})
  await zhRemoval.getByRole('button',{name:'取消',exact:true}).click()
  await zhPrices.getByRole('button',{name:'取消',exact:true}).click()
  await page.getByRole('combobox',{name:'语言',exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('English',{exact:true}).click()
  await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await page.setViewportSize({width:1440,height:1000})
  await page.getByRole('button',{name:'View prices',exact:true}).first().click()
  pricesDrawer=page.getByRole('dialog',{name:'Configured prices',exact:true})
  await pricesDrawer.getByRole('button',{name:'Remove price',exact:true}).click()
  const removalDrawer=page.getByRole('dialog',{name:'Remove price',exact:true})
  await expect(removalDrawer.getByText('123.45',{exact:true})).toBeVisible()
  await page.screenshot({path:join(output,'price-removal-confirm-en-1440.png'),fullPage:true})
  await removalDrawer.getByRole('button',{name:'Remove price',exact:true}).click()
  await expect(removalDrawer.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
  await removalDrawer.getByRole('button',{name:'Retry this save',exact:true}).click()
  await expect(removalDrawer).toHaveCount(0)
  assert.equal(removeCommands.length,2);assert.deepEqual(removeCommands[0],removeCommands[1])
  await expect(pricesDrawer.getByText('No configured prices',{exact:true})).toBeVisible()
  await page.unroute('**'+removalPath)
  await post(removalPath.replace('/api/admin/',''),removeCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)})
  await post(removalPath.replace('/api/admin/',''),removeCommands[0],403,{'X-CSRF-Token':csrf})
  await post(removalPath.replace('/api/admin/',''),removeCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  const anonymousRemoval=await browser.newContext()
  assert.equal((await anonymousRemoval.request.post(origin+removalPath,{data:removeCommands[0]})).status(),401)
  await anonymousRemoval.close()
  const deletedDetail=await context.request.get(origin+createdPricesPath+'/'+encodeURIComponent(createdPriceID));assert.equal(deletedDetail.status(),404)
  const removedProduct=await context.request.get(origin+'/api/store/products/'+products[0]+'?region_id=option-region');assert.equal(removedProduct.status(),200)
  assert.ok((await removedProduct.json()).product.variants.every(v=>v.calculated_price===null))
  await page.screenshot({path:join(output,'price-removed-admin-en.png'),fullPage:true})
  for(const lang of ['en','zh'])for(const width of [1440,390]){
   await publicPage.setViewportSize({width,height:900})
   const response=await publicPage.goto(astro+'/dk-'+lang+'/products/'+products[0]);assert.equal(response.status(),200)
   assert.ok(!(await response.text()).includes('123.45'))
   await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
   await expect(publicPage.getByTestId('add-product-button')).toBeDisabled()
   assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
   await publicPage.screenshot({path:join(output,'price-removed-astro-'+lang+'-'+width+'.png'),fullPage:true})
  }
  regionalProof.removal={actual_ant_remove:true,admin_en_desktop_zh_mobile:true,actual_origin_csrf_anonymous_denials:true,operation_id:removeCommands[0].operation_id,price_id:createdPriceID,same_uuid_response_loss_retry:true,deleted_detail_404:true,public_quote_null:true,astro_initial_html_no_old_price:true,en_zh_widths:[1440,390],purchase_disabled:true}
  // Add a region-bound quantity tier using the same real price form.
  await pricesDrawer.getByRole('button',{name:'Add base price',exact:true}).click()
  const tierDrawer=page.getByRole('dialog',{name:'Add base price',exact:true})
  await tierDrawer.getByLabel('Minimum quantity (optional)',{exact:true}).fill('2')
  await tierDrawer.getByLabel('Maximum quantity (optional)',{exact:true}).fill('3')
  await tierDrawer.getByLabel('Region (optional)',{exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('Option native region · EUR',{exact:true}).click()
  await tierDrawer.getByLabel('Amount',{exact:true}).fill('88.25')
  const tierCommands=[];let tierDropped=false,tierID
  await page.route('**/api/admin/products/*/variants/*/prices',async route=>{
   if(route.request().method()!=='POST'){await route.continue();return}
   tierCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);tierID=(await response.json()).id
   if(!tierDropped){tierDropped=true;await route.abort('failed')}else await route.fulfill({response})
  })
  await page.screenshot({path:join(output,'quantity-price-create-en.png'),fullPage:true})
  await tierDrawer.getByRole('button',{name:'Save',exact:true}).click()
  await expect(tierDrawer.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
  await expect(tierDrawer.getByLabel('Minimum quantity (optional)',{exact:true})).toBeDisabled()
  await expect(tierDrawer.getByLabel('Maximum quantity (optional)',{exact:true})).toBeDisabled()
  await tierDrawer.getByRole('button',{name:'Retry this save',exact:true}).click()
  await expect(tierDrawer).toHaveCount(0)
  assert.equal(tierCommands.length,2);assert.deepEqual(tierCommands[0],tierCommands[1]);assert.equal(tierCommands[0].min_quantity,'2');assert.equal(tierCommands[0].max_quantity,'3')
  await page.unroute('**/api/admin/products/*/variants/*/prices')
  const tierQuotes=[]
  for(const quantity of ['1','2','3','4']){
   const response=await context.request.get(astro+'/api/store/products/'+products[0]+'?region_id=option-region&quantity='+quantity);assert.equal(response.status(),200)
   const variants=(await response.json()).product.variants,quotes=variants.flatMap(v=>v.calculated_price?[v.calculated_price.calculated_amount]:[])
   if(quantity==='2'||quantity==='3')assert.deepEqual(quotes,['88.25']);else assert.deepEqual(quotes,[])
   tierQuotes.push({quantity,quotes})
  }
  for(const lang of ['en','zh'])for(const width of [1440,390]){
   await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/'+products[0]);assert.equal(response.status(),200)
   assert.ok(!(await response.text()).includes('88.25'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
   await expect(publicPage.getByTestId('add-product-button')).toBeDisabled()
   assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  }
  regionalProof.quantity={actual_ant_tier_save:true,operation_id:tierCommands[0].operation_id,price_id:tierID,immutable_bound_retry:true,astro_bridge_actual_quantities:tierQuotes,quantity_one_page_no_tier_price:true,en_zh_widths:[1440,390],purchase_disabled:true}
  await page.screenshot({path:join(output,'quantity-price-saved-en.png'),fullPage:true})
  await pricesDrawer.getByRole('button',{name:'Cancel',exact:true}).click()
  await page.getByRole('combobox',{name:'Language',exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('简体中文',{exact:true}).click()
  await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await page.setViewportSize({width:390,height:844})
  await page.getByRole('button',{name:'查看价格',exact:true}).first().click()
  await page.getByRole('dialog',{name:'价格配置',exact:true}).getByRole('button',{name:'新增基础价格',exact:true}).click()
  const zhTier=page.getByRole('dialog',{name:'新增基础价格',exact:true})
  await zhTier.getByLabel('最小数量（可选）',{exact:true}).fill('7')
  await zhTier.getByLabel('最大数量（可选）',{exact:true}).fill('6')
  await zhTier.getByLabel('地区（可选）',{exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('Option native region · EUR',{exact:true}).click()
  await zhTier.getByLabel('金额',{exact:true}).fill('55')
  const invalidTierRequests=[]
  const trackInvalid=request=>{if(request.method()==='POST'&&/\/api\/admin\/products\/[^/]+\/variants\/[^/]+\/prices$/.test(new URL(request.url()).pathname))invalidTierRequests.push(request.url())}
  page.on('request',trackInvalid)
  await zhTier.getByRole('button',{name:'保存',exact:true}).click()
  await expect(zhTier.getByText('请输入正整数，且最小数量不能超过最大数量。',{exact:true}).first()).toBeVisible()
  assert.equal(invalidTierRequests.length,0);page.off('request',trackInvalid)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,'quantity-price-invalid-zh-390.png'),fullPage:true})
  await zhTier.getByRole('button',{name:'取消',exact:true}).click()
  regionalProof.quantity.admin_en_desktop_zh_mobile=true;regionalProof.quantity.invalid_range_ui_rejected=true
  const editPath=createdPricesPath+'/'+encodeURIComponent(tierID)+'/quantity',editCommands=[];let editDropped=false
  await page.route('**'+editPath,async route=>{editCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);if(!editDropped){editDropped=true;await route.abort('failed')}else await route.fulfill({response})})
  await page.getByRole('dialog',{name:'价格配置',exact:true}).getByRole('button',{name:'编辑数量条件',exact:true}).click()
  const quantityEditor=page.getByRole('dialog',{name:'编辑数量条件',exact:true})
  await expect(quantityEditor.getByLabel('最小数量',{exact:true})).toHaveValue('2')
  await expect(quantityEditor.getByLabel('最大数量',{exact:true})).toHaveValue('3')
  await quantityEditor.getByLabel('最小数量',{exact:true}).fill('4');await quantityEditor.getByLabel('最大数量',{exact:true}).fill('5')
  await page.screenshot({path:join(output,'quantity-edit-zh-390.png'),fullPage:true})
  await quantityEditor.getByRole('button',{name:'保存',exact:true}).click()
  await expect(quantityEditor.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible()
  await expect(quantityEditor.getByLabel('最小数量',{exact:true})).toBeDisabled()
  await quantityEditor.getByRole('button',{name:'重试本次保存',exact:true}).click();await expect(quantityEditor).toHaveCount(0)
  assert.equal(editCommands.length,2);assert.deepEqual(editCommands[0],editCommands[1]);await page.unroute('**'+editPath)
  const quoteEdited=async(wants)=>{for(const [quantity,want]of Object.entries(wants)){const response=await context.request.get(astro+'/api/store/products/'+products[0]+'?region_id=option-region&quantity='+quantity);assert.equal(response.status(),200);const quotes=(await response.json()).product.variants.flatMap(v=>v.calculated_price?[v.calculated_price.calculated_amount]:[]);assert.deepEqual(quotes,want?[want]:[])}}
  await quoteEdited({'1':null,'2':null,'4':'88.25','5':'88.25','6':null})
  await post(editPath.replace('/api/admin/',''),editCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)})
  await post(editPath.replace('/api/admin/',''),editCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  await page.getByRole('dialog',{name:'价格配置',exact:true}).getByRole('button',{name:'取消',exact:true}).click()
  await page.getByRole('combobox',{name:'语言',exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('English',{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await page.setViewportSize({width:1440,height:1000});await page.getByRole('button',{name:'View prices',exact:true}).first().click()
  await page.getByRole('dialog',{name:'Configured prices',exact:true}).getByRole('button',{name:'Edit quantity bounds',exact:true}).click()
  const clearEditor=page.getByRole('dialog',{name:'Edit quantity bounds',exact:true})
  await expect(clearEditor.getByLabel('Minimum quantity',{exact:true})).toHaveValue('4');await expect(clearEditor.getByLabel('Maximum quantity',{exact:true})).toHaveValue('5')
  await clearEditor.getByLabel('Minimum quantity',{exact:true}).fill('');await clearEditor.getByLabel('Maximum quantity',{exact:true}).fill('')
  let clearCommand
  await page.route('**'+editPath,async route=>{clearCommand=route.request().postDataJSON();await route.continue()})
  await clearEditor.getByRole('button',{name:'Save',exact:true}).click();await expect(clearEditor).toHaveCount(0);assert.equal(clearCommand.min_quantity,null);assert.equal(clearCommand.max_quantity,null);await page.unroute('**'+editPath)
  await quoteEdited({'1':'88.25','2':'88.25','4':'88.25','6':'88.25'})
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/'+products[0]);assert.equal(response.status(),200);assert.ok((await response.text()).includes('88.25'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);await expect(publicPage.getByTestId('add-product-button')).toBeDisabled();assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await publicPage.screenshot({path:join(output,'quantity-cleared-astro-'+lang+'-'+width+'.png'),fullPage:true})}
  regionalProof.quantity.edit={operation_id:editCommands[0].operation_id,clear_operation_id:clearCommand.operation_id,same_uuid_response_loss_retry:true,actual_zh_mobile_edit_en_desktop_clear:true,edited_boundary_quotes:true,explicit_null_clear:true,cleared_quantity_one_astro_en_zh:true,origin_csrf_denied:true}
  // Actual price-list lifecycle through Ant; seeded prices/rules are never mocked.
  const listPath='/api/admin/price-lists/list-browser-sale',listCommands=[]
  const initialList=await get('price-lists/list-browser-sale');assert.equal(initialList.status,'draft');assert.equal(initialList.price_count,1)
  assert.equal(initialList.starts_at,'2000-01-01T00:00:00.123456Z')
  const listQuote=async want=>{const response=await context.request.get(astro+'/api/store/products/list-browser-product?region_id=option-region');assert.equal(response.status(),200);const p=(await response.json()).product;assert.equal(p.variants.length,1);assert.equal(p.variants[0].calculated_price.calculated_amount,want)}
  await listQuote('100')
  await page.goto(origin+'/price-lists')
  const listRow=page.getByRole('row').filter({hasText:'Browser list draft'})
  await listRow.getByRole('button',{name:'View price list',exact:true}).click()
  let listEditor=page.getByRole('dialog',{name:'Price list settings',exact:true})
  await listEditor.getByLabel('Title',{exact:true}).fill('Browser list active')
  await listEditor.getByLabel('Description',{exact:true}).fill('Browser edited description')
  const selectList=async(label,option)=>{const input=listEditor.getByLabel(label,{exact:true});await input.locator('xpath=ancestor::div[contains(@class,"ant-select-selector")][1]').click();await page.locator('.ant-select-dropdown:visible').getByTitle(option,{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)}
  await selectList('Status','Active')
  let loseList=true
  await page.route('**'+listPath,async route=>{if(route.request().method()!=='POST'){await route.continue();return};listCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);if(loseList){loseList=false;await route.abort('failed')}else await route.fulfill({response})})
  await listEditor.getByRole('button',{name:'Save',exact:true}).click();await expect(listEditor.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await expect(listEditor.getByLabel('Title',{exact:true})).toBeDisabled()
  await listEditor.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(listEditor).toHaveCount(0);assert.equal(listCommands.length,2);assert.deepEqual(listCommands[0],listCommands[1]);assert.equal(listCommands[0].starts_at,initialList.starts_at);await page.unroute('**'+listPath)
  await listQuote('42.75')
  const activeList=await get('price-lists/list-browser-sale');assert.deepEqual(activeList.rules,initialList.rules);assert.equal(activeList.starts_at,initialList.starts_at)
  await post('price-lists/list-browser-sale',listCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)})
  await post('price-lists/list-browser-sale',listCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  const anonList=await browser.newContext();assert.equal((await anonList.request.post(origin+listPath,{headers:{Origin:origin},data:listCommands[0]})).status(),401);await anonList.close()
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/list-browser-product');assert.equal(response.status(),200);assert.ok((await response.text()).includes('42.75'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await publicPage.screenshot({path:join(output,'price-list-active-astro-'+lang+'-'+width+'.png'),fullPage:true})}
  const scheduleCommands=[]
  await page.route('**'+listPath,async route=>{if(route.request().method()==='POST')scheduleCommands.push(route.request().postDataJSON());await route.continue()})
  await page.getByRole('row').filter({hasText:'Browser list active'}).getByRole('button',{name:'View price list',exact:true}).click();listEditor=page.getByRole('dialog',{name:'Price list settings',exact:true})
  const starts=listEditor.getByLabel('Starts at (UTC)',{exact:true})
  await starts.fill('2099-01-01 00:00:00');await page.locator('.ant-picker-dropdown:visible').getByRole('button',{name:'OK',exact:true}).click()
  await page.screenshot({path:join(output,'price-list-scheduled-en-1440.png'),fullPage:true})
  await listEditor.getByRole('button',{name:'Save',exact:true}).click();await expect(listEditor).toHaveCount(0)
  assert.equal(scheduleCommands.length,1);assert.equal(scheduleCommands[0].starts_at,'2099-01-01T00:00:00.000Z');await listQuote('100')
  await page.getByRole('row').filter({hasText:'Browser list active'}).getByRole('button',{name:'View price list',exact:true}).click();listEditor=page.getByRole('dialog',{name:'Price list settings',exact:true})
  await listEditor.getByLabel('Starts at (UTC)',{exact:true}).hover();await listEditor.locator('.ant-picker-clear').first().click()
  await selectList('Price list type','Override')
  await listEditor.getByRole('button',{name:'Save',exact:true}).click();await expect(listEditor).toHaveCount(0)
  assert.equal(scheduleCommands.length,2);assert.equal(scheduleCommands[1].starts_at,null);assert.equal(scheduleCommands[1].type,'override');await listQuote('42.75');await page.unroute('**'+listPath)
  await page.getByRole('combobox',{name:'Language',exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('简体中文',{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0);await page.setViewportSize({width:390,height:900})
  await page.getByRole('row').filter({hasText:'Browser list active'}).getByRole('button',{name:'查看价目表',exact:true}).click();listEditor=page.getByRole('dialog',{name:'价目表设置',exact:true})
  await selectList('状态','草稿');await listEditor.getByLabel('标题',{exact:true}).fill('Browser list draft again')
  await expect(listEditor.getByLabel('开始时间（UTC）',{exact:true})).toHaveAttribute('placeholder','请选择日期')
  await expect(listEditor.getByLabel('结束时间（UTC）',{exact:true})).toHaveAttribute('placeholder','请选择日期')
  await page.screenshot({path:join(output,'price-list-edit-zh-390.png'),fullPage:true})
  let draftCommand
  await page.route('**'+listPath,async route=>{if(route.request().method()==='POST')draftCommand=route.request().postDataJSON();await route.continue()})
  await listEditor.getByRole('button',{name:'保存',exact:true}).click();await expect(listEditor).toHaveCount(0);await page.unroute('**'+listPath);await listQuote('100')
  assert.equal(draftCommand.starts_at,null);assert.deepEqual((await get('price-lists/list-browser-sale')).rules,initialList.rules)
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/list-browser-product');assert.equal(response.status(),200);assert.ok(!(await response.text()).includes('42.75'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)}
  regionalProof.price_list={schedule_operation_ids:scheduleCommands.map(c=>c.operation_id),actual_utc_picker_schedule_clear:true,actual_override_selection:true,chinese_date_controls:true,operation_id:listCommands[0].operation_id,draft_operation_id:draftCommand.operation_id,actual_ant_active_draft:true,immutable_retry:true,unchanged_microseconds:true,rules_preserved:true,actual_astro_active_draft_en_zh_widths:[1440,390],origin_csrf_anonymous_denied:true}
  // Delete the active list through its actual confirmation and recover a lost response.
  await page.getByRole('row').filter({hasText:'Browser list draft again'}).getByRole('button',{name:'查看价目表',exact:true}).click();listEditor=page.getByRole('dialog',{name:'价目表设置',exact:true})
  await selectList('状态','启用')
  let reactivateCommand
  await page.route('**'+listPath,async route=>{if(route.request().method()==='POST')reactivateCommand=route.request().postDataJSON();await route.continue()})
  await listEditor.getByRole('button',{name:'保存',exact:true}).click();await expect(listEditor).toHaveCount(0);await page.unroute('**'+listPath);assert.ok(reactivateCommand.operation_id);await listQuote('42.75')
  await page.getByRole('combobox',{name:'语言',exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('English',{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0);await page.setViewportSize({width:1440,height:1000})
  const removeListPath=listPath+'/remove',removeListCommands=[]
  await page.getByRole('row').filter({hasText:'Browser list draft again'}).getByRole('button',{name:'Remove price list',exact:true}).click()
  let removeListDialog=page.getByRole('dialog',{name:'Remove price list',exact:true})
  await expect(removeListDialog.getByText('Browser list draft again',{exact:true})).toBeVisible();await expect(removeListDialog.getByRole('button',{name:'Remove price list',exact:true})).toBeEnabled()
  await page.screenshot({path:join(output,'price-list-remove-en-1440.png'),fullPage:true});await removeListDialog.getByRole('button',{name:'Cancel',exact:true}).click();await expect(removeListDialog).toHaveCount(0);await listQuote('42.75')
  await page.getByRole('combobox',{name:'Language',exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('简体中文',{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0);await page.setViewportSize({width:390,height:900})
  await page.getByRole('row').filter({hasText:'Browser list draft again'}).getByRole('button',{name:'删除价目表',exact:true}).click();removeListDialog=page.getByRole('dialog',{name:'删除价目表',exact:true})
  await expect(removeListDialog.getByRole('button',{name:'删除价目表',exact:true})).toBeEnabled();await page.screenshot({path:join(output,'price-list-remove-zh-390.png'),fullPage:true})
  let loseRemoveList=true
  await page.route('**'+removeListPath,async route=>{removeListCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);if(loseRemoveList){loseRemoveList=false;await route.abort('failed')}else await route.fulfill({response})})
  await removeListDialog.getByRole('button',{name:'删除价目表',exact:true}).click();await expect(removeListDialog.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible();await removeListDialog.getByRole('button',{name:'重试本次保存',exact:true}).click();await expect(removeListDialog).toHaveCount(0);await page.unroute('**'+removeListPath)
  assert.equal(removeListCommands.length,2);assert.deepEqual(removeListCommands[0],removeListCommands[1]);await expect(page.getByRole('row').filter({hasText:'Browser list draft again'})).toHaveCount(0)
  assert.equal((await context.request.get(origin+listPath)).status(),404);await listQuote('100')
  await post('price-lists/list-browser-sale/remove',removeListCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)})
  await post('price-lists/list-browser-sale/remove',removeListCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  const anonRemoveList=await browser.newContext();assert.equal((await anonRemoveList.request.post(origin+removeListPath,{headers:{Origin:origin},data:removeListCommands[0]})).status(),401);await anonRemoveList.close()
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/list-browser-product');assert.equal(response.status(),200);const html=await response.text();assert.ok(!html.includes('42.75'));assert.ok(html.includes('100.00'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);await expect(publicPage.getByTestId('add-product-button')).toBeDisabled();assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await publicPage.screenshot({path:join(output,'price-list-removed-astro-'+lang+'-'+width+'.png'),fullPage:true})}
  regionalProof.price_list.removal={operation_id:removeListCommands[0].operation_id,reactivate_operation_id:reactivateCommand.operation_id,actual_ant_en_cancel_zh_remove:true,active_quote_before_remove:'42.75',fallback_quote:'100',same_uuid_response_loss_retry:true,removed_detail_404:true,actual_astro_en_zh_widths:[1440,390],origin_csrf_anonymous_denied:true}
  // Add actual SKU membership prices to a separately seeded list.
  await page.goto(origin+'/products/list-browser-product')
  await page.getByRole('combobox',{name:'Language',exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('简体中文',{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await page.getByRole('button',{name:'查看价格',exact:true}).click()
  let memberPrices=page.getByRole('dialog',{name:'价格配置',exact:true})
  await memberPrices.getByRole('button',{name:'新增价目表价格',exact:true}).click()
  let memberDrawer=page.getByRole('dialog',{name:'新增价目表价格',exact:true})
  await expect(memberDrawer.getByLabel('价目表',{exact:true})).toBeEnabled()
  await memberDrawer.getByLabel('价目表',{exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('Browser member list · 启用 · 促销',{exact:true}).click()
  await memberDrawer.getByLabel('地区（可选）',{exact:true}).press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByTitle('Option native region · EUR',{exact:true}).click()
  await memberDrawer.getByLabel('金额',{exact:true}).fill('37.25')
  await page.screenshot({path:join(output,'price-list-member-zh-390.png'),fullPage:true})
  const memberCommands=[],memberIDs=[];let loseMember=true
  await page.route('**/api/admin/products/list-browser-product/variants/list-browser-variant/prices',async route=>{
   memberCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);memberIDs.push((await response.json()).id)
   if(loseMember){loseMember=false;await route.abort('failed')}else await route.fulfill({response})
  })
  await memberDrawer.getByRole('button',{name:'保存',exact:true}).click()
  await expect(memberDrawer.getByRole('button',{name:'重试本次保存',exact:true})).toBeVisible()
  await expect(memberDrawer.getByLabel('价目表',{exact:true})).toBeDisabled()
  await memberDrawer.getByRole('button',{name:'重试本次保存',exact:true}).click();await expect(memberDrawer).toHaveCount(0)
  assert.equal(memberCommands.length,2);assert.deepEqual(memberCommands[0],memberCommands[1]);assert.equal(memberIDs[0],memberIDs[1]);assert.equal(memberCommands[0].price_list_id,'list-browser-member')
  await listQuote('37.25')
  await memberPrices.getByRole('button',{name:'新增价目表价格',exact:true}).click();memberDrawer=page.getByRole('dialog',{name:'新增价目表价格',exact:true})
  await memberDrawer.getByLabel('价目表',{exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('Browser member list · 启用 · 促销',{exact:true}).click()
  await memberDrawer.getByLabel('地区（可选）',{exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('Option native region · EUR',{exact:true}).click()
  await memberDrawer.getByLabel('最小数量（可选）',{exact:true}).fill('2');await memberDrawer.getByLabel('最大数量（可选）',{exact:true}).fill('3');await memberDrawer.getByLabel('金额',{exact:true}).fill('32.50')
  await memberDrawer.getByRole('button',{name:'保存',exact:true}).click();await expect(memberDrawer).toHaveCount(0);await page.unroute('**/api/admin/products/list-browser-product/variants/list-browser-variant/prices')
  assert.equal(memberCommands.length,3)
  for(const [quantity,want]of Object.entries({'1':'37.25','2':'32.5','3':'32.5','4':'37.25'})){const response=await context.request.get(astro+'/api/store/products/list-browser-product?region_id=option-region&quantity='+quantity);assert.equal(response.status(),200);assert.equal((await response.json()).product.variants[0].calculated_price.calculated_amount,want)}
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/list-browser-product');assert.equal(response.status(),200);assert.ok((await response.text()).includes('37.25'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);await expect(publicPage.getByTestId('add-product-button')).toBeDisabled();assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await publicPage.screenshot({path:join(output,'price-list-member-astro-'+lang+'-'+width+'.png'),fullPage:true})}
  regionalProof.price_list.membership={operation_id:memberCommands[0].operation_id,tier_operation_id:memberCommands[2].operation_id,price_id:memberIDs[0],tier_price_id:memberIDs[2],actual_ant_zh_mobile:true,same_uuid_response_loss_retry:true,immutable_list_selection:true,existing_regional_base_coexists:true,actual_astro_quantities:{'1':'37.25','2':'32.5','3':'32.5','4':'37.25'},actual_astro_en_zh_widths:[1440,390],zero_stock_disabled:true}
  // Full create wizard: actual product selection across searches and 3 initial prices.
  await page.goto(origin+'/price-lists');await page.setViewportSize({width:1440,height:1000})
  await page.getByRole('button',{name:'Create price list',exact:true}).click()
  const createListDrawer=page.getByRole('dialog',{name:'Create price list',exact:true}),createdListTitle='Browser created initial list'
  await createListDrawer.getByLabel('Title',{exact:true}).fill(createdListTitle);await createListDrawer.getByLabel('Description',{exact:true}).fill('Two products with regional quantity prices')
  await createListDrawer.getByRole('button',{name:'Next',exact:true}).click()
  const chooseProduct=async id=>{const search=createListDrawer.getByLabel('Search products',{exact:true});await search.fill(id);await search.press('Enter');const row=createListDrawer.getByRole('row').filter({has:page.getByText(id,{exact:true})});await expect(row).toBeVisible();await row.getByRole('checkbox').check()}
  await chooseProduct('list-browser-product');await chooseProduct(products[0]);await createListDrawer.getByRole('button',{name:'Next',exact:true}).click()
  const nativeProduct=(await get('products/'+products[0])).product
  const addInitialPrice=async(variantLabel,amount,min='',max='',targetDrawer=createListDrawer)=>{
   await targetDrawer.getByRole('button',{name:'Add initial price',exact:true}).click()
   await targetDrawer.getByLabel('Product / SKU',{exact:true}).last().press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle(variantLabel,{exact:true}).click()
   const regionControl=targetDrawer.getByLabel('Region (optional)',{exact:true}).last();await regionControl.scrollIntoViewIfNeeded();await regionControl.press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('Option native region · EUR',{exact:true}).click()
   await targetDrawer.getByLabel('Amount',{exact:true}).last().fill(amount)
   if(min)await targetDrawer.getByLabel('Minimum quantity (optional)',{exact:true}).last().fill(min)
   if(max)await targetDrawer.getByLabel('Maximum quantity (optional)',{exact:true}).last().fill(max)
  }
  await addInitialPrice('Price list browser product / Default','25.00')
  await addInitialPrice('Price list browser product / Default','15.50','2','3')
  const createdScope=await get('price-list-creation?product_id='+encodeURIComponent(products[0])),targetChoice=createdScope.products[0].variants.find(v=>v.title===definition.values.find(value=>value.id===ids['256GB']).value);assert.ok(targetChoice)
  await addInitialPrice(nativeProduct.title+' / '+targetChoice.title+(targetChoice.sku?' · '+targetChoice.sku:'')+(targetChoice.shared_variants>1?' · Shared SKUs '+targetChoice.shared_variants:''),'21.25')
  await page.screenshot({path:join(output,'price-list-create-en-1440.png'),fullPage:true})
  await page.setViewportSize({width:390,height:900});await page.screenshot({path:join(output,'price-list-create-en-390.png'),fullPage:true})
  const createListCommands=[],createListResults=[];let loseCreateList=true
  await page.route('**/api/admin/price-lists',async route=>{if(route.request().method()!=='POST'){await route.continue();return};createListCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);createListResults.push(await response.json());if(loseCreateList){loseCreateList=false;await route.abort('failed')}else await route.fulfill({response})})
  await createListDrawer.getByRole('button',{name:'Save',exact:true}).click();await expect(createListDrawer.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await expect(createListDrawer.getByLabel('Amount',{exact:true}).first()).toBeDisabled();await createListDrawer.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(createListDrawer).toHaveCount(0);await page.unroute('**/api/admin/price-lists')
  assert.equal(createListCommands.length,2);assert.deepEqual(createListCommands[0],createListCommands[1]);assert.equal(createListResults[0].id,createListResults[1].id);assert.equal(createListResults[0].replayed,false);assert.equal(createListResults[1].replayed,true)
  await expect(page.getByRole('row').filter({hasText:createdListTitle})).toHaveCount(1)
  const createdList=await get('price-lists/'+createListResults[0].id);assert.equal(createdList.price_count,3);assert.equal(createdList.status,'active');assert.equal(createdList.type,'sale');assert.equal(createdList.starts_at,null);assert.equal(createdList.ends_at,null)
  // A matching channel-specific list retains source specificity precedence.
  await listQuote('37.25')
  let disableExistingCommand
  const existingListPath='/api/admin/price-lists/list-browser-member'
  await page.getByRole('row').filter({hasText:'Browser member list'}).getByRole('button',{name:'View price list',exact:true}).click()
  const existingListEditor=page.getByRole('dialog',{name:'Price list settings',exact:true})
  await existingListEditor.getByLabel('Description',{exact:true}).fill('Browser membership fixture')
  await existingListEditor.getByLabel('Status',{exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('Draft',{exact:true}).click()
  await page.route('**'+existingListPath,async route=>{if(route.request().method()==='POST')disableExistingCommand=route.request().postDataJSON();await route.continue()})
  await existingListEditor.getByRole('button',{name:'Save',exact:true}).click();await expect(existingListEditor).toHaveCount(0);await page.unroute('**'+existingListPath);assert.ok(disableExistingCommand.operation_id)
  for(const [quantity,want]of Object.entries({'1':'25','2':'15.5','3':'15.5','4':'25'})){const response=await context.request.get(astro+'/api/store/products/list-browser-product?region_id=option-region&quantity='+quantity);assert.equal(response.status(),200);assert.equal((await response.json()).product.variants[0].calculated_price.calculated_amount,want)}
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/list-browser-product');assert.equal(response.status(),200);assert.ok((await response.text()).includes('25.00'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);await expect(publicPage.getByTestId('add-product-button')).toBeDisabled();assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await publicPage.screenshot({path:join(output,'price-list-created-astro-'+lang+'-'+width+'.png'),fullPage:true})}
  const secondQuote=await context.request.get(astro+'/api/store/products/'+products[0]+'?region_id=option-region');assert.equal(secondQuote.status(),200);assert.ok((await secondQuote.json()).product.variants.some(v=>v.calculated_price?.calculated_amount==='21.25'))
  await post('price-lists',createListCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)});await post('price-lists',createListCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  const anonCreateList=await browser.newContext();assert.equal((await anonCreateList.request.post(origin+'/api/admin/price-lists',{headers:{Origin:origin},data:createListCommands[0]})).status(),401);await anonCreateList.close()
  regionalProof.price_list.creation={operation_id:createListCommands[0].operation_id,list_id:createListResults[0].id,actual_ant_wizard:true,product_selection_across_search:true,selected_product_ids:createListCommands[0].product_ids,initial_prices:createListCommands[0].prices,initial_price_count:3,same_uuid_response_loss_retry:true,more_specific_existing_quote:'37.25',disable_existing_operation_id:disableExistingCommand.operation_id,actual_disable_existing_list:true,actual_astro_quantities:{'1':'25','2':'15.5','3':'15.5','4':'25'},second_product_quote:'21.25',actual_astro_en_zh_widths:[1440,390],zero_stock_disabled:true,origin_csrf_anonymous_denied:true}
  // Add two more prices to the same existing list in one real command.
  await page.setViewportSize({width:1440,height:1000})
  const beforeBatch=await get('price-lists/'+createListResults[0].id)
  await page.getByRole('row').filter({hasText:createdListTitle}).getByRole('button',{name:'Add prices',exact:true}).click()
  const addDrawer=page.getByRole('dialog',{name:'Add prices',exact:true})
  const chooseBatch=async id=>{const search=addDrawer.getByLabel('Search products',{exact:true});await search.fill(id);await search.press('Enter');const row=addDrawer.getByRole('row').filter({has:page.getByText(id,{exact:true})});await expect(row).toBeVisible();await row.getByRole('checkbox').check()}
  await chooseBatch('list-browser-product');await chooseBatch(products[0]);await addDrawer.getByRole('button',{name:'Next',exact:true}).click()
  await addInitialPrice('Price list browser product / Default','12.25','4','5',addDrawer)
  const otherChoice=createdScope.products[0].variants.find(v=>v.title===definition.values.find(value=>value.id===ids['128GB']).value);assert.ok(otherChoice)
  await addInitialPrice(nativeProduct.title+' / '+otherChoice.title+(otherChoice.sku?' · '+otherChoice.sku:'')+(otherChoice.shared_variants>1?' · Shared SKUs '+otherChoice.shared_variants:''),'17.75','','',addDrawer)
  await page.screenshot({path:join(output,'price-list-add-en-1440.png'),fullPage:true});await page.setViewportSize({width:390,height:900});await page.screenshot({path:join(output,'price-list-add-en-390.png'),fullPage:true})
  const addCommands=[],addResults=[],addPath='/api/admin/price-lists/'+createListResults[0].id+'/prices';let loseAdd=true
  await page.route('**'+addPath,async route=>{addCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);addResults.push(await response.json());if(loseAdd){loseAdd=false;await route.abort('failed')}else await route.fulfill({response})})
  await addDrawer.getByRole('button',{name:'Save',exact:true}).click();await expect(addDrawer.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await expect(addDrawer.getByLabel('Product / SKU',{exact:true}).first()).toBeDisabled();await addDrawer.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(addDrawer).toHaveCount(0);await page.unroute('**'+addPath)
  assert.equal(addCommands.length,2);assert.deepEqual(addCommands[0],addCommands[1]);assert.equal(addResults[0].id,createListResults[0].id);assert.equal(addResults[0].replayed,false);assert.equal(addResults[1].replayed,true)
  const afterBatch=await get('price-lists/'+createListResults[0].id);assert.equal(afterBatch.price_count,5);for(const key of ['title','description','status','type','starts_at','ends_at','rules_count'])assert.deepEqual(afterBatch[key],beforeBatch[key]);assert.deepEqual(afterBatch.rules,beforeBatch.rules)
  for(const [quantity,want]of Object.entries({'1':'25','2':'15.5','3':'15.5','4':'12.25','5':'12.25','6':'25'})){const response=await context.request.get(astro+'/api/store/products/list-browser-product?region_id=option-region&quantity='+quantity);assert.equal(response.status(),200);assert.equal((await response.json()).product.variants[0].calculated_price.calculated_amount,want)}
  const otherQuotesResponse=await context.request.get(astro+'/api/store/products/'+products[0]+'?region_id=option-region');assert.equal(otherQuotesResponse.status(),200);const otherQuotes=(await otherQuotesResponse.json()).product.variants.map(v=>v.calculated_price?.calculated_amount).sort();assert.deepEqual(otherQuotes,['17.75','21.25'])
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/'+products[0]);assert.equal(response.status(),200);assert.ok((await response.text()).includes('17.75'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);await expect(publicPage.getByTestId('add-product-button')).toBeDisabled();assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await publicPage.screenshot({path:join(output,'price-list-added-astro-'+lang+'-'+width+'.png'),fullPage:true})}
  await post('price-lists/'+createListResults[0].id+'/prices',addCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)});await post('price-lists/'+createListResults[0].id+'/prices',addCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  const anonAdd=await browser.newContext();assert.equal((await anonAdd.request.post(origin+addPath,{headers:{Origin:origin},data:addCommands[0]})).status(),401);await anonAdd.close()
  regionalProof.price_list.addition={list_id:createListResults[0].id,operation_id:addCommands[0].operation_id,initial_prices:addCommands[0].prices,selected_product_ids:addCommands[0].product_ids,actual_existing_list_batch_ui:true,immutable_response_loss_retry:true,settings_rules_preserved:true,old_price_count:3,new_price_count:5,quantity_quotes:{'1':'25','2':'15.5','3':'15.5','4':'12.25','5':'12.25','6':'25'},second_product_quotes:otherQuotes,actual_astro_en_zh_widths:[1440,390],zero_stock_disabled:true,origin_csrf_anonymous_denied:true}
  // A separate real UI-created list keeps earlier creation/addition receipts
  // verifiable while exercising one mixed edit/delete/create command.
  await page.setViewportSize({width:1440,height:1000})
  await page.getByRole('button',{name:'Create price list',exact:true}).click()
  const batchTitle='Browser mixed batch list'
  await createListDrawer.getByLabel('Title',{exact:true}).fill(batchTitle);await createListDrawer.getByLabel('Description',{exact:true}).fill('Atomic existing and replacement prices')
  await createListDrawer.getByRole('button',{name:'Next',exact:true}).click();await chooseProduct('list-browser-product');await chooseProduct(products[0]);await createListDrawer.getByRole('button',{name:'Next',exact:true}).click()
  await addInitialPrice('Price list browser product / Default','7.00');await addInitialPrice('Price list browser product / Default','5.00','2','3')
  await addInitialPrice(nativeProduct.title+' / '+otherChoice.title+(otherChoice.sku?' · '+otherChoice.sku:'')+(otherChoice.shared_variants>1?' · Shared SKUs '+otherChoice.shared_variants:''),'9.00')
  const batchCreateResponse=page.waitForResponse(r=>r.url()===origin+'/api/admin/price-lists'&&r.request().method()==='POST')
  await createListDrawer.getByRole('button',{name:'Save',exact:true}).click();const batchCreated=await (await batchCreateResponse).json();assert.ok(batchCreated.id);await expect(createListDrawer).toHaveCount(0)
  const batchID=batchCreated.id,batchPath='/api/admin/price-lists/'+batchID+'/price-batch',batchQuery='?product_id=list-browser-product&product_id='+encodeURIComponent(products[0])
  const batchBefore=await get('price-lists/'+batchID),batchBeforeRows=await get('price-lists/'+batchID+'/price-batch'+batchQuery)
  assert.equal(batchBeforeRows.prices.length,3)
  const originalBase=batchBeforeRows.prices.find(p=>p.product_id==='list-browser-product'&&!p.min_quantity),originalTier=batchBeforeRows.prices.find(p=>p.min_quantity==='2'),originalOther=batchBeforeRows.prices.find(p=>p.product_id===products[0]);assert.ok(originalBase&&originalTier&&originalOther)
  await page.getByRole('row').filter({hasText:batchTitle}).getByRole('button',{name:'Edit prices',exact:true}).click()
  const batchDrawer=page.getByRole('dialog',{name:'Edit prices',exact:true})
  const chooseEdit=async id=>{const search=batchDrawer.getByLabel('Search products',{exact:true});await search.fill(id);await search.press('Enter');const row=batchDrawer.getByRole('row').filter({has:page.getByText(id,{exact:true})});await expect(row).toBeVisible();await row.getByRole('checkbox').check()}
  await chooseEdit('list-browser-product');await chooseEdit(products[0]);await batchDrawer.getByRole('button',{name:'Next',exact:true}).click()
  await batchDrawer.getByLabel('Amount '+originalTier.id,{exact:true}).fill('4.125');await batchDrawer.getByLabel('Minimum quantity '+originalTier.id,{exact:true}).fill('4');await batchDrawer.getByLabel('Maximum quantity '+originalTier.id,{exact:true}).fill('5')
  await batchDrawer.getByLabel('Amount '+originalOther.id,{exact:true}).fill('7.75')
  const removeRow=batchDrawer.getByRole('row').filter({hasText:originalBase.id})
  await removeRow.getByRole('button',{name:'Remove',exact:true}).click();await expect(batchDrawer.getByLabel('Amount '+originalBase.id,{exact:true})).toBeDisabled()
  await removeRow.getByRole('button',{name:'Undo removal',exact:true}).click();await expect(batchDrawer.getByLabel('Amount '+originalBase.id,{exact:true})).toBeEnabled();await removeRow.getByRole('button',{name:'Remove',exact:true}).click()
  await batchDrawer.getByRole('heading',{name:'Existing prices',exact:true}).scrollIntoViewIfNeeded();await batchDrawer.screenshot({path:join(output,'price-list-batch-existing-en-1440.png')})
  const addBatchPrice=async(amount,min='',max='')=>{
   await batchDrawer.getByRole('button',{name:'Add new price',exact:true}).click();const sku=batchDrawer.getByLabel('Product / SKU',{exact:true}).last();await sku.scrollIntoViewIfNeeded();await sku.press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('Price list browser product / Default',{exact:true}).click()
   const region=batchDrawer.getByLabel('Region (optional)',{exact:true}).last();await region.scrollIntoViewIfNeeded();await region.press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('Option native region · EUR',{exact:true}).click()
   await batchDrawer.getByLabel('Amount',{exact:true}).last().fill(amount);if(min)await batchDrawer.getByLabel('Minimum quantity (optional)',{exact:true}).last().fill(min);if(max)await batchDrawer.getByLabel('Maximum quantity (optional)',{exact:true}).last().fill(max)
  }
  await addBatchPrice('8.00');await addBatchPrice('6.00','2','3')
  await batchDrawer.screenshot({path:join(output,'price-list-batch-en-1440.png')});await page.setViewportSize({width:390,height:900});await batchDrawer.screenshot({path:join(output,'price-list-batch-en-390.png')})
  const batchCommands=[],batchResults=[];let loseBatch=true
  await page.route('**'+batchPath,async route=>{if(route.request().method()!=='POST'){await route.continue();return};batchCommands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);batchResults.push(await response.json());if(loseBatch){loseBatch=false;await route.abort('failed')}else await route.fulfill({response})})
  await batchDrawer.getByRole('button',{name:'Save',exact:true}).click();await expect(batchDrawer.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await expect(batchDrawer.getByLabel('Amount '+originalTier.id,{exact:true})).toBeDisabled();await expect(batchDrawer.getByRole('button',{name:'Add new price',exact:true})).toBeDisabled();await batchDrawer.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(batchDrawer).toHaveCount(0);await page.unroute('**'+batchPath)
  assert.equal(batchCommands.length,2);assert.deepEqual(batchCommands[0],batchCommands[1]);assert.equal(batchCommands[0].create.length,2);assert.equal(batchCommands[0].update.length,2);assert.equal(batchCommands[0].delete.length,1);assert.equal(batchResults[0].replayed,false);assert.equal(batchResults[1].replayed,true)
  const batchAfter=await get('price-lists/'+batchID),batchAfterRows=await get('price-lists/'+batchID+'/price-batch'+batchQuery);assert.equal(batchAfter.price_count,4);assert.equal(batchAfterRows.prices.length,4)
  for(const key of ['title','description','status','type','starts_at','ends_at','rules_count','rules'])assert.deepEqual(batchAfter[key],batchBefore[key])
  assert.ok(!batchAfterRows.prices.some(p=>p.id===originalBase.id));assert.ok(batchAfterRows.prices.some(p=>p.id===originalTier.id&&p.min_quantity==='4'&&p.max_quantity==='5'));assert.ok(batchAfterRows.prices.some(p=>p.id===originalOther.id));assert.deepEqual(batchAfterRows.prices.find(p=>p.id===originalTier.id).rules,originalTier.rules)
  const batchQuotes={'1':'8','2':'6','3':'6','4':'4.125','5':'4.125','6':'8'}
  for(const [quantity,want]of Object.entries(batchQuotes)){const response=await context.request.get(astro+'/api/store/products/list-browser-product?region_id=option-region&quantity='+quantity);assert.equal(response.status(),200);assert.equal((await response.json()).product.variants[0].calculated_price.calculated_amount,want)}
  for(const lang of ['en','zh'])for(const width of [1440,390]){await publicPage.setViewportSize({width,height:900});const response=await publicPage.goto(astro+'/dk-'+lang+'/products/'+products[0]);assert.equal(response.status(),200);const relatedHTML=await response.text();assert.ok(relatedHTML.includes('7.75'));assert.ok(relatedHTML.includes('data-testid="related-products-container"'));await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0);const recommendations=publicPage.getByTestId('related-products-container');const relatedLink=recommendations.locator('a[href="/dk-'+lang+'/products/list-browser-product"]');await relatedLink.scrollIntoViewIfNeeded();await expect(relatedLink).toBeVisible();await expect(relatedLink.getByTestId('related-product-price')).toHaveText('€8.00');assert.equal(await recommendations.locator('a[href="/dk-'+lang+'/products/'+products[0]+'"]').count(),0);await expect(publicPage.getByTestId('add-product-button')).toBeDisabled();assert.equal(await publicPage.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await publicPage.screenshot({path:join(output,'price-list-batched-astro-'+lang+'-'+width+'.png'),fullPage:true})}
  await post('price-lists/'+batchID+'/price-batch',batchCommands[0],403,{Origin:origin,'X-CSRF-Token':'0'.repeat(64)});await post('price-lists/'+batchID+'/price-batch',batchCommands[0],403,{Origin:'https://untrusted.example.test','X-CSRF-Token':csrf})
  const batchAnon=await browser.newContext();assert.equal((await batchAnon.request.post(origin+batchPath,{headers:{Origin:origin},data:batchCommands[0]})).status(),401);await batchAnon.close()
  assert.equal((await context.request.get(origin+batchPath+'?unexpected=1')).status(),400)
  await page.getByRole('combobox',{name:'Language',exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('简体中文',{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await page.getByRole('row').filter({hasText:batchTitle}).getByRole('button',{name:'批量编辑价格',exact:true}).click();const batchChinese=page.getByRole('dialog',{name:'批量编辑价格',exact:true})
  for(const id of ['list-browser-product',products[0]]){const search=batchChinese.getByLabel('搜索商品',{exact:true});await search.fill(id);await search.press('Enter');const row=batchChinese.getByRole('row').filter({has:page.getByText(id,{exact:true})});await expect(row).toBeVisible();await row.getByRole('checkbox').check()}
  await batchChinese.getByRole('button',{name:'下一步',exact:true}).click();await expect(batchChinese.getByLabel('金额 '+originalOther.id,{exact:true})).toHaveValue('7.75')
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await batchChinese.screenshot({path:join(output,'price-list-batch-zh-390.png')});await batchChinese.getByRole('button',{name:'取消',exact:true}).click();await expect(batchChinese).toHaveCount(0)
  await page.getByRole('combobox',{name:'语言',exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:visible').getByTitle('English',{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  regionalProof.price_list.batch={list_id:batchID,operation_id:batchCommands[0].operation_id,command:batchCommands[0],original_base_id:originalBase.id,original_tier_id:originalTier.id,original_other_id:originalOther.id,actual_ant_mixed_batch:true,chinese_mobile_readback:true,undo_removal:true,immutable_response_loss_retry:true,updated_ids_rules_preserved:true,settings_preserved:true,old_price_count:3,new_price_count:4,quantity_quotes:batchQuotes,other_product_quote:'7.75',astro_en_zh_widths:[1440,390],zero_stock_disabled:true,origin_csrf_anonymous_denied:true}
  await page.goto(origin+'/option-definitions')
  await publicPage.goto(astro+'/dk-zh/products/'+products[0]);await expect(publicPage.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
  const visibleGroup=publicPage.getByRole('group',{name:currentTitle,exact:true})
  await visibleGroup.evaluate(node=>node.scrollIntoView({block:'center',behavior:'instant'}))
  for(const label of await visibleGroup.locator('label').all())await expect(label).toBeInViewport({ratio:1})
  await publicPage.screenshot({path:join(output,'shared-options-astro-zh-mobile-viewport.png')})
  await publicPage.screenshot({path:join(output,'shared-options-astro-zh-mobile.png'),fullPage:true});await publicPage.close()
  astroProof={real_built_astro:true,related_products_initial_html_and_hydration:true,related_current_product_excluded:true,related_exact_price:"8",related_localized_links:true,admin_shared_membership_ui:true,associated_products:2,actual_sku_commands:4,shared_title_rank_refresh:true,source_value_ids_preserved:true,variant_ids_configuration_preserved:true,initial_html_contains_updated_title:true,language_en_zh_widths:[1440,390],zero_stock_purchase_disabled:true,commerce_provider_not_exercised:true}
 }
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'))
 await page.setViewportSize({width:390,height:844});await page.reload()
 await expect(section.getByRole('button',{name:'新建共享选项',exact:true})).toBeVisible()
 await section.getByRole('row').filter({hasText:currentTitle}).getByRole('button',{name:'管理选项值',exact:true}).click()
 for(const button of await section.getByRole('button',{name:'删除选项值',exact:true}).all())await expect(button).toBeDisabled()
 assert.equal(await section.getByRole('button',{name:'删除选项值',exact:true}).count(),2)
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await page.screenshot({path:join(output,'shared-option-values-zh-mobile.png'),fullPage:true})
 await section.getByRole('row').filter({hasText:currentTitle}).getByRole('button',{name:'收起选项值',exact:true}).click()
 await page.locator('.ant-table-content').first().evaluate(node=>{node.scrollLeft=node.scrollWidth})
 await section.getByRole('row').filter({hasText:currentTitle}).getByRole('button',{name:'元数据',exact:true}).click()
 metadataEditor=page.getByRole('dialog',{name:'编辑元数据',exact:true})
 const zhKeys=await metadataEditor.getByLabel('键',{exact:true}).evaluateAll(inputs=>inputs.map(i=>i.value))
 await metadataEditor.getByLabel('值',{exact:true}).nth(zhKeys.indexOf('enabled')).fill('false')
 await expect(metadataEditor.getByLabel('值',{exact:true}).nth(zhKeys.indexOf('structured'))).toBeDisabled()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await page.screenshot({path:join(output,'shared-option-metadata-zh-mobile.png'),fullPage:true})
 await metadataEditor.getByRole('button',{name:'保存元数据',exact:true}).click();await expect(metadataEditor).toHaveCount(0)
 assert.deepEqual(JSON.parse((await get('option-definitions/'+definition.id)).metadata_json),{enabled:false,keep:'unchanged',structured:{n:1}})
 await section.getByRole('row').filter({hasText:currentTitle}).getByRole('button',{name:/编\s*辑/}).click()
 await expect(page.getByRole('dialog').getByText('已关联商品',{exact:true})).toHaveCount(2)
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 assert.deepEqual(errors,[]);await page.screenshot({path:join(output,'shared-options-zh-mobile.png'),fullPage:true})
 if(astro){
  await page.goto(origin+'/buy-policies')
  await expect(page.getByLabel('保修月数',{exact:true})).toHaveValue('36')
  await expect(page.getByLabel('退货天数',{exact:true})).toHaveValue('21')
  await expect(page.getByLabel('中文',{exact:true}).nth(0)).toHaveValue('后台保修备注')
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,'purchase-policies-admin-zh-mobile.png'),fullPage:true})
  policyProof.admin_en_desktop_zh_mobile=true
 }
 if(astro)collectionProof=await verifyNativeCollections(context,astro,output,errors,process.env.ADMIN_TEST_API_TARGET)
 if(astro)searchProof=await verifyNativeSearch(context,astro,output,errors)
 assert.deepEqual(errors,[])
 console.log('OPTION_DEFINITIONS_BROWSER '+JSON.stringify({search:searchProof,collection:collectionProof,policy:policyProof,scope:scopeProof,regional_price:regionalProof,real_ant_gin_mysql_redis:true,multi_value_time_filters_url_reload:true,unchanged_microsecond_boundary_preserved:true,metadata_merge_retry_and_empty_definition:true,metadata_zh_mobile_saved:true,metadata_preserved_after_title_rank_edit:true,metadata_operation_id:metadataCommands[0].operation_id,api_mocking:false,actual_response_dropped:true,same_operation_retried:true,created_once:true,edit_identity_rank_preserved:true,product_membership_enforced:true,associated_delete_disabled:true,unassociated_delete:true,last_unassociated_value_ui_deleted:true,value_delete_response_dropped_retried:true,value_delete_operation_id:valueCommands[0].operation_id,value_delete_definition_id:removableID,value_delete_value_id:removableDefinition.values[0].id,csrf_origin_auth_checked:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0,operation_id:commands[0].operation_id,definition_id:definition.id,astro:astroProof}))
}catch(error){console.error('PAGE_ERRORS',errors);console.error('TABLE_ROWS',await page?.getByTestId('option-definitions').getByRole('row').allTextContents());console.error('POPOVER_STATE',await page?.locator('.ant-popover').evaluateAll(nodes=>nodes.map(n=>({html:n.outerHTML,rect:n.getBoundingClientRect().toJSON(),style:getComputedStyle(n).cssText}))));await page?.screenshot({path:join(output,'browser-failure.png'),fullPage:true});throw error}
finally{await browser?.close();await server?.close()}
