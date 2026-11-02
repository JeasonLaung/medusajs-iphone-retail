import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:20000}),shop='http://127.0.0.1:19300',origin='http://127.0.0.1:19200',logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER
assert.match(owner??'',/^[0-9a-f]{12}$/)
 const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')}),errors=[],newCartCases=[]
try{
 const adminContext=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),admin=await adminContext.newPage()
 admin.on('pageerror',e=>errors.push(e.message));await admin.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8'),credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
 await admin.goto(origin+'/account');await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'));await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 const lostGroup=async(action,root=admin)=>{
  const bodies=[],replies=[];await admin.route('**/api/admin/customer-groups',async route=>{if(route.request().method()!=='POST')return route.continue();bodies.push(route.request().postDataJSON());if(bodies.length===1){const rs=await Promise.all(Array.from({length:6},()=>route.fetch()));for(const r of rs){assert.equal(r.status(),200);replies.push(await r.json())}await route.abort('failed')}else{const r=await route.fetch();assert.equal(r.status(),200);assert.equal((await r.json()).replayed,true);await route.fulfill({response:r})}})
  await action();await expect(root.getByRole('button',{name:'Retry same operation',exact:true})).toBeVisible();await root.getByRole('button',{name:'Retry same operation',exact:true}).click();await expect(root.getByRole('button',{name:'Retry same operation',exact:true})).toHaveCount(0);await admin.unrouteAll({behavior:'wait'});assert.equal(bodies.length,2);assert.deepEqual(bodies[0],bodies[1]);assert.equal(new Set(replies.map(r=>r.group_id)).size,1);assert.equal(replies.filter(r=>!r.replayed).length,1);return replies[0]
 }
 await admin.goto(origin+'/customer-groups');await admin.getByRole('button',{name:'Create customer group',exact:true}).click();let drawer=admin.getByRole('dialog');await drawer.getByLabel('Group name',{exact:true}).fill('Verified members 测试客户组')
 const created=await lostGroup(()=>drawer.getByRole('button',{name:'Save',exact:true}).click(),drawer);await expect(drawer).not.toBeVisible()
 const group=admin.locator(`tr[data-row-key="${created.group_id}"]`);await expect(group).toContainText('Verified members')
 await group.getByRole('button',{name:'View members',exact:true}).click();drawer=admin.getByRole('dialog');await drawer.getByRole('button',{name:'Add members',exact:true}).click();await drawer.getByRole('searchbox',{name:'Search account email or ID'}).fill(`local-account-${owner}-1@example.invalid`);await drawer.getByRole('searchbox').press('Enter')
 const account=drawer.getByRole('row').filter({hasText:`local-account-${owner}-1@example.invalid`});await account.getByRole('checkbox').check();await lostGroup(()=>drawer.getByRole('button',{name:'Add selected members (1)',exact:true}).click(),drawer)
 await drawer.locator('.ant-drawer-close').click();await expect(group.getByRole('cell',{name:'1',exact:true})).toBeVisible()
 await admin.goto(origin+'/price-lists');await admin.getByRole('button',{name:'Create price list',exact:true}).click();drawer=admin.getByRole('dialog');await drawer.getByLabel('Title',{exact:true}).fill('Group-only cart price');await drawer.getByLabel('Description',{exact:true}).fill('Trusted new-account membership fixture')
 await drawer.locator('.ant-form-item').filter({hasText:'Customer groups (optional)'}).locator('.ant-select-selector').click();await admin.locator('.ant-select-item-option').filter({hasText:'Verified members 测试客户组'}).click();await drawer.getByLabel('Title',{exact:true}).click();await drawer.getByRole('button',{name:'Next',exact:true}).click()
 const search=drawer.getByRole('searchbox',{name:'Search products',exact:true});await search.fill('iPhone 17e');await search.press('Enter');const product=drawer.locator('tr[data-row-key]').filter({hasText:'iPhone 17e'});await product.getByRole('checkbox').check();await drawer.getByRole('button',{name:'Next',exact:true}).click()
 // Use the real multi-step UI to establish the group-qualified list; price
 // membership is added through its production API with exact monetary input.
 const listResponse=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/api/admin/price-lists'));await drawer.getByRole('button',{name:'Save',exact:true}).click();const lr=await listResponse;assert.equal(lr.status(),200);const listID=(await lr.json()).id;await expect(drawer).not.toBeVisible()
 const csrf=(await adminContext.cookies(origin)).find(c=>c.name==='retail_admin_csrf')?.value,headers={'Content-Type':'application/json','Origin':origin,'X-CSRF-Token':decodeURIComponent(csrf)}
 const region=(await (await admin.request.get(origin+'/api/store/regions')).json()).regions.find(r=>r.countries.some(c=>c.iso_2==='gb'))
 const publicProduct=(await (await admin.request.get(origin+'/api/store/products?handle=swappie-iphone-17e&region_id='+region.id)).json()).products[0]
 const variant='variant_01M51RN7H7HPV9RH69T32QMD2Q',base=publicProduct.variants.find(v=>v.id===variant).calculated_price.calculated_amount
 const addition=await (await admin.request.get(origin+'/api/admin/price-lists/'+listID+'/price-addition?product_id='+publicProduct.id)).json()
 const added=await admin.request.post(origin+'/api/admin/price-lists/'+listID+'/prices',{headers,data:{operation_id:crypto.randomUUID(),expected_revision:addition.revision,product_ids:[publicProduct.id],prices:[{product_id:publicProduct.id,variant_id:variant,currency_code:'eur',amount:'321.123456789012345678',region_id:region.id,min_quantity:'',max_quantity:''}]}});assert.equal(added.status(),200)
 const customerContext=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await customerContext.newPage();page.on('pageerror',e=>errors.push(e.message))
 const read=async path=>{const r=await customerContext.request.get(shop+'/api/store/'+path);assert.equal(r.status(),200);return r.json()}
 await page.goto(shop+'/gb-en/products/swappie-iphone-17e?v_id='+variant);await page.getByTestId('add-product-button').click();await expect(page.getByTestId('checkout-button')).toBeVisible();const cart=(await read('cart')).cart;assert.equal((await read('carts/'+cart.id+'/amounts')).lines[0].unit_price,base)
 await page.goto(shop+'/gb-en/account');await page.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`);await page.getByTestId('password-input').fill('isolated-account-'+owner+'-password');await page.getByTestId('sign-in-button').click();await expect(page.getByTestId('mobile-account-nav')).toBeVisible()
 assert.equal((await read('carts/'+cart.id+'/amounts')).lines[0].unit_price,'321.123456789012345678')
 // Same current account drives SSR, client product refresh and search prices.
 const catalogAmount=async(context,want)=>{const r=await context.request.get(shop+'/api/store/products/'+publicProduct.id+'?region_id='+region.id+'&quantity=2');assert.equal(r.status(),200);assert.equal(r.headers()['cache-control'],'private, no-store');assert.equal((await r.json()).product.variants.find(v=>v.id===variant).calculated_price.calculated_amount,want)}
 await catalogAmount(customerContext,'321.123456789012345678')
 const missing=await customerContext.request.get(shop+'/api/store/products/product_missing_group_fixture?region_id='+region.id);assert.equal(missing.status(),404);assert.equal(missing.headers()['cache-control'],'private, no-store')
 for(const language of ['en','zh']){const r=await page.goto(shop+'/gb-'+language+'/products/swappie-iphone-17e?v_id='+variant);assert.equal(r.status(),200);assert.equal(r.headers()['cache-control'],'private, no-store');await expect(page.getByTestId('product-price')).toHaveAttribute('data-value','321.123456789012345678');await page.screenshot({path:resolve(logs,'group-product-'+language+'.png'),fullPage:true})}
 const searchBody={region_id:region.id,queries:[{key:'0',query:'iPhone 17e',filters:{},sort:'relevance',page:1,limit:20}]}
 const searched=await customerContext.request.post(shop+'/api/store/search',{data:searchBody});assert.equal(searched.status(),200);assert.equal(searched.headers()['cache-control'],'private, no-store');assert.equal((await searched.json()).results[0].hits.find(p=>p.id===publicProduct.id).price.min,'321.123456789012345678')
 await page.goto(shop+'/gb-en/store');await expect(page.locator('main')).toContainText('321.12');await page.screenshot({path:resolve(logs,'group-store-en.png'),fullPage:true})
 const collections=await (await admin.request.get(origin+'/api/store/collections')).json(),collection=collections.collections.find(c=>c.id===publicProduct.collection_id)
 assert.ok(collection,'fixture product needs its original public collection')
 await page.goto(shop+'/gb-en/collections/'+collection.handle);await expect(page.getByTestId('products-list')).toContainText('321.12');await page.screenshot({path:resolve(logs,'group-collection-en.png'),fullPage:true})
 const publicStillAnonymous=await admin.request.get(origin+'/api/store/products/'+publicProduct.id+'?region_id='+region.id,{headers:{Cookie:(await customerContext.cookies(shop)).filter(c=>c.name==='retail_customer_session').map(c=>c.name+'='+c.value).join('; ')}});assert.equal(publicStillAnonymous.status(),200);assert.equal((await publicStillAnonymous.json()).product.variants.find(v=>v.id===variant).calculated_price.calculated_amount,base)
 const anonymous=await browser.newContext();await catalogAmount(anonymous,base)
 await anonymous.addCookies([{name:'retail_customer_session',value:'f'.repeat(64),url:shop}]);await catalogAmount(anonymous,base);await anonymous.close()
 for(const [index,mode] of ['normal','lost_create','lost_binding','lost_item'].entries()){
  const segment=index%2?'gb-zh':'gb-en',width=index===1?1440:390,context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}),fresh=await context.newPage(),attempts=[];fresh.on('pageerror',e=>errors.push(e.message))
  await fresh.goto(shop+'/'+segment+'/account');await fresh.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`);await fresh.getByTestId('password-input').fill('isolated-account-'+owner+'-password');await fresh.getByTestId('sign-in-button').click();await expect(fresh.getByTestId(width<600?'mobile-account-nav':'account-nav')).toBeVisible()
  const noCart=await context.request.get(shop+'/api/store/cart');assert.equal(noCart.status(),401)
  let lost=false
  await fresh.route(/\/api\/store\/(carts(?:\/[^/]+\/line-items)?|customer\/cart-transfer)$/,async route=>{
   if(route.request().method()!=='POST')return route.continue()
   const path=new URL(route.request().url()).pathname,kind=path.endsWith('/cart-transfer')?'binding':path.endsWith('/line-items')?'item':'create',r=await route.fetch();assert.ok([200,201].includes(r.status()))
   const result=await r.json();attempts.push({kind,body:route.request().postDataJSON(),key:route.request().headers()['idempotency-key'],version:result.version??result.cart?.version})
   if(mode==='lost_'+kind&&!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})
  })
  await fresh.goto(shop+'/'+segment+'/products/swappie-iphone-17e?v_id='+variant);await expect(fresh.getByTestId('product-price')).toHaveAttribute('data-value','321.123456789012345678');await fresh.getByTestId('add-product-button').click()
  if(mode!=='normal'){
   await expect.poll(()=>lost).toBe(true);await expect(fresh.getByTestId('product-container').getByRole('status')).toContainText(segment.endsWith('-zh')?'添加失败':'Could not add')
   const journal=await fresh.evaluate(()=>localStorage.getItem('retail.cart.intent.v1'));assert.ok(journal);assert.equal(JSON.parse(journal).kind,mode==='lost_create'?'create':mode==='lost_binding'?'bind':'add')
   await fresh.reload();await fresh.getByTestId('add-product-button').click()
  }
  await expect(fresh.getByTestId('checkout-button')).toBeVisible();await expect.poll(()=>fresh.evaluate(()=>localStorage.getItem('retail.cart.intent.v1'))).toBe(null)
  const currentResponse=await context.request.get(shop+'/api/store/cart');assert.equal(currentResponse.status(),200);const current=(await currentResponse.json()).cart
  const amounts=await (await context.request.get(shop+'/api/store/carts/'+current.id+'/amounts')).json();assert.equal(amounts.lines.length,1);assert.equal(amounts.lines[0].quantity,'1');assert.equal(amounts.lines[0].unit_price,'321.123456789012345678');assert.equal(current.version,'3')
  assert.ok(attempts.findIndex(a=>a.kind==='binding')<attempts.findIndex(a=>a.kind==='item'))
  for(const kind of ['create','binding','item']){const same=attempts.filter(a=>a.kind===kind);assert.equal(same.length,mode==='lost_'+kind?2:1);assert.ok(same.every(a=>JSON.stringify(a.body)===JSON.stringify(same[0].body)&&a.key===same[0].key))}
  await fresh.goto(shop+'/'+segment+'/cart');await expect(fresh.getByTestId('product-price')).toContainText('321.12');await fresh.screenshot({path:resolve(logs,'new-cart-'+mode+'.png'),fullPage:true})
  newCartCases.push({mode,cart_id:current.id,version:current.version,quantity:'1',unit_price:amounts.lines[0].unit_price,bind_before_first_add:true,retry_preserved:true});await context.close()
 }
 await page.goto(shop+'/gb-en/cart');await expect(page.getByTestId('product-row')).toBeVisible();await expect(page.getByTestId('product-price')).toContainText('321.12');await page.screenshot({path:resolve(logs,'group-cart-en.png'),fullPage:true})
 await admin.goto(origin+'/customer-groups');await group.getByRole('button',{name:'View members',exact:true}).click();drawer=admin.getByRole('dialog');await drawer.getByRole('row').filter({hasText:`local-account-${owner}-1@example.invalid`}).getByRole('checkbox').check();await lostGroup(()=>drawer.getByRole('button',{name:'Remove selected members (1)',exact:true}).click(),drawer);await drawer.locator('.ant-drawer-close').click()
 // Actual existing cart quantity command causes current membership repricing.
 await page.getByTestId('product-select-button').fill('2');await page.getByTestId('product-select-button').press('Enter');await expect.poll(async()=> (await read('carts/'+cart.id+'/amounts')).lines[0].quantity).toBe('2');assert.equal((await read('carts/'+cart.id+'/amounts')).lines[0].unit_price,base)
 await catalogAmount(customerContext,base)
 await page.goto(shop+'/gb-zh/cart');await expect(page.getByTestId('product-row')).toBeVisible();await expect(page.getByTestId('product-select-button')).toHaveValue('2');await page.screenshot({path:resolve(logs,'group-cart-zh-after-removal.png'),fullPage:true});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await admin.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh'));await admin.reload();await expect(group).toContainText('Verified members');await admin.screenshot({path:resolve(logs,'customer-groups-zh.png'),fullPage:true})
 assert.deepEqual(errors,[]);writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'customer-groups',group_id:created.group_id,price_list_id:listID,cart_id:cart.id,new_cart_cases:newCartCases,anonymous_amount:base,group_amount:'321.123456789012345678',create_add_remove_six_concurrent_lost_reply_recovered:true,actual_price_list_group_name_selector:true,login_reprices_original_cart:true,removed_membership_falls_back:true,personalized_product_en_zh:true,personalized_search_and_store:true,personalized_collection:true,private_no_store:true,anonymous_and_expired_session_no_price_leak:true,public_gin_ignores_customer_cookie:true,admin_en_zh:true,customer_mobile_en_zh:true,browser_errors:0},null,2))
}catch(e){for(const [i,p]of browser.contexts().flatMap(c=>c.pages()).entries())await p.screenshot({path:resolve(logs,`groups-failure-${i}.png`),fullPage:true}).catch(()=>{});throw e}finally{await browser.close()}
