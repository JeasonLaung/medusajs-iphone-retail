import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {fileURLToPath} from 'node:url'

const previousOrders=[]
export async function startReviewModeration({browser,origin,errors,expect}){
 const context=await browser.newContext({reducedMotion:'reduce',viewport:{width:1440,height:1000}})
 context.setDefaultTimeout(15000)
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await context.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await page.goto(origin+'/account')
 await page.getByLabel('Email',{exact:true}).fill('browser@example.invalid')
 await page.getByLabel('Password',{exact:true}).fill('artificial-browser-password')
 await page.getByRole('button',{name:'Sign in',exact:true}).click()
 await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 return {page,context,origin,close:()=>context.close()}
}

// Every business response below comes from production Gin handlers backed by
// native MySQL/Redis. The only interception drops a genuine committed response.
export async function verifyCheckoutReviews({page,context,shop,segment,orderID,index,admin,control,expect}){
 const zh=segment.endsWith('-zh'),linked=index%2===0,title='Joint review '+index,rating=6-index
 const content='The actual purchased phone works well. <script>window.reviewInjected=true</script>'
 const facts=async()=>{const r=await control('review-facts');assert.equal(r.status,200);return (await r.json()).sha256}
 const before=await facts(),productURL=shop+`/${segment}/products/add-product`,detailURL=shop+`/${segment}/account/orders/details/${orderID}`
 const publicPage=async()=>{const r=await context.request.get(shop+'/api/store/product-reviews?product_id=add-product&offset=0&limit=5');assert.equal(r.status(),200);return r.json()}
 const beforePublic=await publicPage();assert.equal(beforePublic.count,index-1)
 const eligibility=await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/product-reviews')
 assert.equal(eligibility.status(),200);assert.equal((await eligibility.json()).eligible,true,'Paid unshipped order must already be eligible')
 if(previousOrders.length)assert.equal((await context.request.get(shop+'/api/store/customer/orders/'+previousOrders.at(-1)+'/product-reviews')).status(),404)
 if(linked){
  await expect(page.getByTestId('order-reviews').getByRole('button',{name:zh?'写评价':'Write a review',exact:true})).toBeEnabled()
  await page.getByTestId('order-reviews').getByRole('button',{name:zh?'写评价':'Write a review',exact:true}).click()
 }else{
  await page.goto(productURL)
  await page.waitForFunction(()=>{const island=document.querySelector('[data-testid="product-reviews"]')?.closest('astro-island');return island&&!island.hasAttribute('ssr')})
  const section=page.getByTestId('product-reviews')
  await section.getByRole('button',{name:zh?'写评价':'Write a review',exact:true}).click()
 }
 const form=page.getByTestId('review-form')
 if(!linked){
  const purchase=form.getByLabel(zh?'关联购买订单（选填）':'Link a purchase (optional)',{exact:true})
  await expect(purchase).toBeEnabled();await purchase.selectOption(orderID);await purchase.selectOption('')
 }
 await form.getByRole('radio',{name:zh?`${rating} 星，满分 5 星`:`${rating} out of 5 stars`,exact:true}).click()
 await form.getByLabel(zh?'评价标题':'Review title',{exact:true}).fill(title)
 await form.getByLabel(zh?'使用体验':'Your experience',{exact:true}).fill(content)
 const submissions=[],pattern=shop+'/api/store/customer/product-reviews'
 await page.route(pattern,async route=>{
  assert.equal(route.request().method(),'POST');submissions.push(route.request().postDataJSON())
  const response=await route.fetch();assert.equal(response.status(),201)
  if(submissions.length===1)await route.abort('failed');else await route.fulfill({response})
 })
 let reviewID
 try{
  await form.getByRole('button',{name:zh?'提交评价':'Submit review',exact:true}).click()
  await expect(form.getByRole('button',{name:zh?'重试同一请求':'Retry the same request',exact:true})).toBeVisible()
  await expect(form.getByLabel(zh?'评价标题':'Review title',{exact:true})).toBeDisabled()
  await expect(form.getByRole('button',{name:zh?/^取\s*消$/: 'Cancel',exact:true})).toBeDisabled()
  await form.getByRole('button',{name:zh?'重试同一请求':'Retry the same request',exact:true}).click()
  await expect(form).toHaveCount(0)
 }finally{await page.unroute(pattern)}
 assert.equal(submissions.length,2);assert.deepEqual(submissions[0],submissions[1]);assert.equal(submissions[0].order_id,linked?orderID:undefined)
 const purchases=await context.request.get(shop+'/api/store/customer/product-reviews/purchases?product_id=add-product&offset=0')
 assert.equal(purchases.status(),200);const own=(await purchases.json()).review;reviewID=own.id
 assert.equal(own.status,'pending');assert.equal(own.verified_purchase,linked)
 assert.equal((await publicPage()).count,index-1,'Pending review must remain private')
 const csrf=(await context.cookies(shop)).find(c=>c.name==='retail_customer_csrf')?.value;assert.ok(csrf)
 const duplicate=await context.request.post(pattern,{headers:{Origin:shop,'X-CSRF-Token':csrf},data:{...submissions[0],operation_id:randomUUID()}})
 assert.equal(duplicate.status(),409);assert.equal((await duplicate.json()).error.code,'review.already_submitted')
 await page.goto(detailURL);await expect(page.getByTestId('order-reviews')).toContainText(zh?'已提交，等待审核':'Submitted — awaiting moderation')

 const ap=admin.page
 await ap.setViewportSize({width:page.viewportSize().width,height:1100})
 await ap.evaluate(language=>localStorage.setItem('eurorefurb.admin.language',language),zh?'zh-CN':'en')
 await ap.goto(admin.origin+'/products/add-product')
 const section=ap.getByTestId('admin-product-reviews'),row=section.getByTestId('admin-product-review').filter({has:ap.getByText(title,{exact:true})})
 await expect(row).toHaveCount(1);await expect(row).toContainText(zh?'待审核':'Pending')
 await expect(row).toContainText(linked?(zh?'已验证购买':'Verified purchase'):(zh?'开放评价':'Open review'))
 const moderations=[],moderationPattern=admin.origin+'/api/admin/product-reviews/'+reviewID
 await ap.route(moderationPattern,async route=>{
  moderations.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200)
  if(moderations.length===1)await route.abort('failed');else await route.fulfill({response})
 })
 try{
  await row.getByRole('button',{name:zh?'批准发布':'Approve',exact:true}).click()
  const retry=section.getByRole('button',{name:zh?'重试本次保存':'Retry this save',exact:true})
  await expect(retry).toBeVisible();await expect(row.getByRole('button',{name:zh?'拒绝发布':'Reject',exact:true})).toBeDisabled()
  await retry.click();await expect(row).toContainText(zh?'已发布':'Published')
 }finally{await ap.unroute(moderationPattern)}
 assert.equal(moderations.length,2);assert.deepEqual(moderations[0],moderations[1])
 if(index===1){
  await row.getByRole('button',{name:'Reject',exact:true}).click();await expect(row).toContainText('Not published');assert.equal((await publicPage()).count,0)
  await row.getByRole('button',{name:'Set to pending',exact:true}).click();await expect(row).toContainText('Pending');assert.equal((await publicPage()).count,0)
  await row.getByRole('button',{name:'Approve',exact:true}).click();await expect(row).toContainText('Published')
 }
 const published=await publicPage();assert.equal(published.count,index)
 const current=published.reviews.find(r=>r.id===reviewID);assert.equal(current.title,title);assert.equal(current.rating,rating);assert.equal(current.author,'New');assert.equal(current.verified_purchase,linked)
 const me=await (await context.request.get(shop+'/api/store/customer/me')).json()
 for(const secret of [orderID,me.customer.id,me.customer.email])assert.ok(!JSON.stringify(published).includes(secret))
 assert.equal(published.average,Array.from({length:index},(_,i)=>5-i).reduce((a,b)=>a+b,0)/index)
 const html=await (await context.request.get(productURL)).text();assert.ok(html.includes(title));assert.ok(!html.includes('<script>window.reviewInjected=true</script>'))
 await page.goto(productURL)
 await page.waitForFunction(()=>{const island=document.querySelector('[data-testid="product-reviews"]')?.closest('astro-island');return island&&!island.hasAttribute('ssr')})
 const displayed=page.getByTestId('product-reviews')
 await expect(displayed.getByText(title,{exact:true})).toBeVisible()
 assert.equal(await page.evaluate(()=>window.reviewInjected),undefined)
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 assert.equal(await ap.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await displayed.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/review-joint-${index}-store.png`,import.meta.url))})
 await section.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/review-joint-${index}-admin.png`,import.meta.url))})
 assert.equal(await facts(),before,'Review UI must not change stock, reservation, price, device or payment facts')
 previousOrders.push(orderID)
 await page.goto(detailURL);await expect(page.getByTestId('order-reviews')).toContainText(zh?'已发布':'Published')
 console.log('PRODUCT_REVIEWS_JOINT_CASE '+JSON.stringify({index,segment,width:page.viewportSize().width,linked_purchase:linked,paid_unshipped:true,real_native_api:true,submission_original_retry:true,moderation_original_retry:true,public_privacy:true,escaped_content:true,exact_other_facts_unchanged:true}))
}
