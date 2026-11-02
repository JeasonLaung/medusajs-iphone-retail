import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
const port=Number(process.env.ADMIN_TEST_PORT),origin=`http://127.0.0.1:${port}`,astro=process.env.SEO_ASTRO_ORIGIN,output=process.env.SEO_BROWSER_OUTPUT
if(!port||!output||!/^http:\/\/127\.0\.0\.1:\d+$/.test(astro??''))throw Error('Owned SEO fixture required')
let server,browser,page
const errors=[],commands=[],saved=new Set()
try{
 server=await createServer({server:{port,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({channel:'chrome',headless:true})
 const context=await browser.newContext({reducedMotion:'reduce'})
 await context.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 page=await context.newPage();page.setDefaultTimeout(30000);page.on('pageerror',e=>errors.push(e.message))
 page.on('request',request=>{if(request.method()==='POST'&&request.url().includes('/api/admin/seo/'))commands.push(request.postDataJSON())})
 page.on('response',async r=>{if(r.request().method()==='POST'&&r.url().includes('/api/admin/seo/')&&r.status()===200){try{const c=r.request().postDataJSON();saved.add(c.operation_id)}catch{}}})
 const unauth=await context.request.get(origin+'/api/admin/seo/settings');assert.equal(unauth.status(),401)
 await page.goto(origin+'/account')
 await page.getByLabel('Email',{exact:true}).fill('browser@example.invalid')
 await page.getByLabel('Password',{exact:true}).fill('artificial-browser-password')
 await page.getByRole('button',{name:'Sign in',exact:true}).click()
 await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await page.getByRole('menuitem',{name:'SEO',exact:true}).click()
 await expect(page.getByRole('button',{name:'Add',exact:true})).toBeEnabled()
 const drawer=()=>page.locator('.ant-drawer-content:visible')
 async function setting(label,value,{drop=false}={}){
  await page.getByRole('button',{name:'Add',exact:true}).click()
  await drawer().locator('.ant-form-item').filter({has:page.getByLabel('Setting',{exact:true})}).locator('.ant-select-selector').click()
  const dropdown=page.locator('.ant-select-dropdown:visible'),option=dropdown.getByText(label,{exact:true})
  // Ant virtualizes options beyond the visible viewport; scroll as a user does.
  for(let i=0;i<10&&!await option.isVisible();i++){
   const holder=dropdown.locator('.rc-virtual-list-holder'),before=await holder.evaluate(el=>el.scrollTop)
   await holder.hover();await page.mouse.wheel(0,160)
   await expect.poll(()=>holder.evaluate(el=>el.scrollTop)).not.toBe(before)
  }
  await option.click()
  const input=drawer().getByLabel('Value',{exact:true})
  if(value==='true'||value==='false'){await drawer().locator('.ant-form-item').filter({has:page.getByLabel('Value',{exact:true})}).locator('.ant-select-selector').click();await page.locator('.ant-select-dropdown:visible').getByText(value==='true'?'Enabled':'Disabled',{exact:true}).click()}
  else await input.fill(value)
  let dropped=false
  if(drop)await page.route('**/api/admin/seo/settings',async route=>{
   if(route.request().method()!=='POST'){await route.continue();return}
   const response=await route.fetch();assert.equal(response.status(),200)
   if(!dropped){dropped=true;await route.abort('failed')}else await route.fulfill({response})
  })
  const before=commands.length
  await drawer().getByRole('button',{name:'Save',exact:true}).click()
  if(drop){
   await expect(drawer().getByText('The result is not confirmed. Retry the same operation.')).toBeVisible()
   await expect(drawer().getByLabel('Value',{exact:true})).toBeDisabled()
   await drawer().getByRole('button',{name:'Retry same operation',exact:true}).click()
  }
  await expect(page.locator('.ant-drawer-content:visible')).toHaveCount(0)
  await expect(page.getByRole('button',{name:'Add',exact:true})).toBeEnabled()
  if(drop){await page.unroute('**/api/admin/seo/settings');assert.equal(commands.length-before,2);assert.equal(commands[before].operation_id,commands[before+1].operation_id);assert.deepEqual(commands[before],commands[before+1])}
 }
 await setting('Site URL',astro)
 await setting('Site name','Native SEO Store',{drop:true})
 await setting('robots.txt',`User-agent: *\nDisallow: /private-native\nSitemap: ${astro}/sitemap.xml`)
 await setting('llms.txt','# Native SEO Store\n\n真实后台保存的公开内容')
 await setting('Bing verification XML','<?xml version="1.0"?><users><user>native-bing</user></users>')
 await setting('Enable Bing verification','true')
 await setting('IndexNow verification key','native-index-key-1234')
 await setting('Enable IndexNow verification','true')
 await setting('Google verification code','native-google-verification')
 await setting('Enable Google site verification','true')
 const fetchText=async(path,status=200)=>{const r=await context.request.get(astro+path);assert.equal(r.status(),status,path);assert.equal(r.headers()['cache-control'],'no-store');return r.text()}
 assert.equal(await fetchText('/robots.txt'),`User-agent: *\nDisallow: /private-native\nSitemap: ${astro}/sitemap.xml`)
 assert.equal(await fetchText('/llms.txt'),'# Native SEO Store\n\n真实后台保存的公开内容')
 assert.ok((await fetchText('/BingSiteAuth.xml')).includes('native-bing'))
 assert.equal(await fetchText('/api/seo-artifacts/native-index-key-1234'),'native-index-key-1234')
 await fetchText('/api/seo-artifacts/wrong-index-key-1234',404)
 await page.getByRole('tab',{name:'Page metadata',exact:true}).click()
 const structured='{"@context":"https://schema.org","@type":"Thing","value":900719925474099312345,"name":"</script>原文"}'
 async function addPage(path,canonical,robots){
  await page.getByRole('button',{name:'Add',exact:true}).click()
  for(const [label,value]of [['Page path',path],['Title','Native title 原文'],['Description','Native description 原文'],['Canonical URL',canonical],['Social image URL',astro+'/favicon.ico'],['Robots',robots],['Language links JSON',JSON.stringify({en:astro+'/dk-en/about','zh-CN':astro+'/dk-zh/about','x-default':astro+'/dk-en/about'})],['Structured data JSON',structured]])await drawer().getByLabel(label,{exact:true}).fill(value)
  await drawer().getByRole('button',{name:'Save',exact:true}).click();await expect(page.locator('.ant-drawer-content:visible')).toHaveCount(0);await expect(page.getByRole('button',{name:'Add',exact:true})).toBeEnabled()
 }
 await addPage('/dk-en/about',astro+'/dk-en/about','noindex,follow,noarchive')
 await addPage('/dk-en/help','https://canonical.example/help','index,follow')
 await addPage('/dk-en/seo-native-remove',astro+'/dk-en/seo-native-remove','index,follow')
 const removable=page.getByRole('row').filter({has:page.getByText('/dk-en/seo-native-remove',{exact:true})})
 await removable.getByRole('button',{name:'Delete',exact:true}).click()
 await page.locator('.ant-popconfirm:visible').getByRole('button',{name:'Delete',exact:true}).click()
 await expect(removable).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Add',exact:true})).toBeEnabled()
 const about=await context.newPage();about.on('pageerror',e=>errors.push(e.message))
 for(const width of [1440,390]){
  await about.setViewportSize({width,height:1000});await about.goto(astro+'/dk-en/about',{waitUntil:'domcontentloaded'})
  assert.equal(await about.title(),'Native title 原文')
  await expect(about.locator('meta[name="description"]')).toHaveAttribute('content','Native description 原文')
  await expect(about.locator('link[rel="canonical"]')).toHaveAttribute('href',astro+'/dk-en/about')
  await expect(about.locator('meta[name="robots"]')).toHaveAttribute('content','noindex, follow, noarchive')
  await expect(about.locator('meta[property="og:image"]')).toHaveAttribute('content',astro+'/favicon.ico')
  await expect(about.locator('meta[name="google-site-verification"]')).toHaveAttribute('content','native-google-verification')
  assert.equal(await about.locator('link[hreflang]').count(),3)
  const raw=await about.locator('script[type="application/ld+json"]').textContent();assert.ok(raw.includes('900719925474099312345'),'exact JSON-LD integer');assert.ok(raw.includes('\\u003c/script\\u003e'))
  assert.equal(await about.locator('#seo-ga4').count(),0)
 }
 let map=await fetchText('/sitemap.xml')
 assert.equal(map.includes(`<loc>${astro}/dk-en/about</loc>`),false);assert.equal(map.includes(`<loc>${astro}/dk-en/help</loc>`),false);assert.ok(map.includes(`<loc>${astro}/dk-zh/about</loc>`))
 // Edit an unchanged raw JSON document; the exact integer must survive form
 // round-trip, then removing noindex must republish this path in the sitemap.
 const row=page.getByRole('row').filter({has:page.getByText('/dk-en/about',{exact:true})})
 await row.getByRole('button',{name:'Edit',exact:true}).click()
 await expect(drawer().getByLabel('Structured data JSON',{exact:true})).toHaveValue(structured)
 await drawer().getByLabel('Robots',{exact:true}).fill('index,follow')
 await drawer().getByRole('button',{name:'Save',exact:true}).click();await expect(page.locator('.ant-drawer-content:visible')).toHaveCount(0)
 map=await fetchText('/sitemap.xml');assert.ok(map.includes(`<loc>${astro}/dk-en/about</loc>`));assert.ok(map.includes(`href="${astro}/dk-zh/about"`));assert.equal(map.includes(`<loc>${astro}/dk-en/help</loc>`),false)
 // Chinese narrow Admin exercises the same stored value and responsive drawer.
 await page.setViewportSize({width:390,height:900})
 await page.getByRole('combobox',{name:'Language',exact:true}).press('ArrowDown')
 await page.locator('.ant-select-dropdown:visible').getByTitle('简体中文',{exact:true}).click()
 await page.reload();await expect(page.getByRole('tab',{name:'页面元数据',exact:true})).toBeVisible()
 await page.getByRole('tab',{name:'页面元数据',exact:true}).click()
 await page.getByRole('row').filter({has:page.getByText('/dk-en/about',{exact:true})}).getByRole('button',{name:/编\s*辑/}).click()
 await expect(drawer().getByLabel('结构化数据 JSON',{exact:true})).toHaveValue(structured)
 await page.screenshot({path:join(output,'seo-admin-zh-mobile.png'),fullPage:true})
 assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1))
 await drawer().getByRole('button',{name:/取\s*消/}).click()
 const csrf=(await context.cookies()).find(c=>c.name==='retail_admin_csrf')?.value;assert.ok(csrf)
 const revision=(await(await context.request.get(origin+'/api/admin/seo/pages')).json()).revision
 const body={operation_id:crypto.randomUUID(),expected_revision:revision,resource:'pages',action:'delete',id:commands.find(c=>c.resource==='pages').id||'nonexistent'}
 for(const headers of [{Origin:'http://untrusted.invalid','X-CSRF-Token':csrf},{Origin:origin},{'X-CSRF-Token':csrf}])assert.equal((await context.request.post(origin+'/api/admin/seo/pages',{headers,data:body})).status(),403)
 assert.equal((await context.request.get(origin+'/api/admin/seo/pages?store_id=other')).status(),400)
 assert.deepEqual(errors,[])
 const successful=new Set(commands.map(c=>c.operation_id));assert.equal(successful.size,15)
 await writeFile(join(output,'browser-result.json'),JSON.stringify({actual_admin_gin_mysql_redis_astro:true,api_mocking:false,successful_response_dropped_then_same_operation_recovered:true,unique_successful_operations:successful.size,configured_files:4,configured_head_widths:[1440,390],configured_noindex_and_external_canonical_sitemap:true,noindex_removal_republishes:true,custom_hreflang:true,page_soft_delete_via_confirmation:true,jsonld_integer_exact_after_edit:true,raw_jsonld_xss_escaped:true,google_verification:true,analytics_not_enabled:true,chinese_mobile_admin:true,unauthenticated_401:true,origin_csrf_rejections:3,query_scope_override_rejected:true,errors,real_provider_called:false,goal_complete:false},null,2)+'\n')
 console.log('SEO_ADMIN_BROWSER '+JSON.stringify({passed:true,operations:successful.size}))
 await context.close()
}catch(error){
 if(page){await page.screenshot({path:join(output,'failure.png'),fullPage:true}).catch(()=>{});await writeFile(join(output,'failure.html'),await page.content()).catch(()=>{})}
 throw error
}finally{if(browser)await browser.close();if(server)await server.close()}
