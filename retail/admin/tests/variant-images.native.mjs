import assert from 'node:assert/strict'
import {randomUUID,createHash} from 'node:crypto'
import {readFile} from 'node:fs/promises'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
const port=Number(process.env.ADMIN_TEST_PORT),origin=`http://127.0.0.1:${port}`,astro=process.env.PRODUCT_IMAGES_ASTRO_ORIGIN
if(!port||!process.env.ADMIN_TEST_API_TARGET||!process.env.ADMIN_EMAIL||!process.env.ADMIN_PASSWORD||!process.env.PRODUCT_IMAGE_FIXTURE||!/^http:\/\/127\.0\.0\.1:\d+$/.test(astro??''))throw Error('Dedicated native fixture required')
const bytes=await readFile(process.env.PRODUCT_IMAGE_FIXTURE),webp=await readFile(new URL('../../storefront/public/iphone-thumbnails/iphone-16/black.webp',import.meta.url)),errors=[]
let server,browser,page
try{
 server=await createServer({server:{port,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen();browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER});const context=await browser.newContext({reducedMotion:'reduce'});await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'));page=await context.newPage();page.setDefaultTimeout(15000);page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(process.env.ADMIN_EMAIL);await page.getByLabel('Password',{exact:true}).fill(process.env.ADMIN_PASSWORD);await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 const choicesResponse=await context.request.get(origin+'/api/admin/products/creation');assert.equal(choicesResponse.status(),200);const dirs=await choicesResponse.json(),csrf=(await context.cookies()).find(c=>c.name==='retail_admin_csrf')?.value;assert.ok(csrf)
 const title='Image native '+randomUUID().slice(0,8),handle='image-native-'+randomUUID(),createdResponse=await context.request.post(origin+'/api/admin/products',{headers:{Origin:origin,'X-CSRF-Token':csrf},data:{operation_id:randomUUID(),title,handle,status:'published',subtitle:null,description:null,collection_id:null,type_id:null,sales_channel_ids:[dirs.channels[0].id],shipping_profile_id:dirs.shipping_profiles[0].id,category_ids:[],image_urls:[],options:[]}});assert.equal(createdResponse.status(),200);const created=await createdResponse.json()

 const write=async(path,data)=>{const r=await context.request.post(origin+'/api/admin/'+path,{headers:{Origin:origin,'X-CSRF-Token':csrf},data});assert.equal(r.status(),200);return r.json()}
 const get=async(path)=>{const r=await context.request.get(origin+'/api/admin/'+path);assert.equal(r.status(),200);return r.json()}
 let gallery=await get('products/'+created.id+'/images')
 await write('products/'+created.id+'/images',{operation_id:randomUUID(),expected_revision:gallery.revision,images:[],thumbnail_index:0,upload:{filename:'blue.png',mime_type:'image/png',content:bytes.toString('base64')}})
 gallery=await get('products/'+created.id+'/images')
 await write('products/'+created.id+'/images',{operation_id:randomUUID(),expected_revision:gallery.revision,images:gallery.images.map(({id,url})=>({id,url})),thumbnail_index:0,upload:{filename:'black.webp',mime_type:'image/webp',content:webp.toString('base64')}})
 gallery=await get('products/'+created.id+'/images');assert.equal(gallery.images.length,2)
 const firstURL=gallery.images[0].url,secondURL=gallery.images[1].url,creation=await get('products/'+created.id+'/variant-creation')
 const variant=await write('products/'+created.id+'/variants',{operation_id:randomUUID(),expected_revision:creation.revision,title:'Gallery SKU',sku:'',option_value_ids:[]})
 await page.goto(origin+'/products/'+created.id);await page.getByRole('button',{name:'SKU images',exact:true}).click();let editor=page.getByRole('dialog',{name:'SKU images',exact:true})
 await editor.getByRole('checkbox',{name:'Image 2',exact:true}).check();await editor.getByRole('radio',{name:'Thumbnail 2',exact:true}).check();await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 let assigned=await get('products/'+created.id+'/variants/'+variant.id+'/images');assert.deepEqual(assigned.selected_ids,[gallery.images[1].id]);assert.equal(assigned.thumbnail,secondURL)
 const regions=await (await context.request.get(origin+'/api/store/regions')).json(),region=regions.regions[0]
 const publicProduct=await (await context.request.get(origin+'/api/store/products/'+created.id+'?region_id='+region.id)).json(),publicVariant=publicProduct.product.variants.find(v=>v.id===variant.id);assert.equal(publicVariant.images.length,1);assert.equal(publicVariant.images[0].url,secondURL);assert.equal(publicVariant.thumbnail,secondURL)
 const response=await page.goto(astro+'/dk-en/products/'+handle+'?v_id='+variant.id);assert.equal(response.status(),200);await expect(page.getByTestId('product-title')).toHaveText(title);await page.waitForFunction(url=>[...document.images].some(i=>i.src===location.origin+url&&i.complete&&i.naturalWidth>0),secondURL)
 const decoded=await context.request.get(astro+secondURL);assert.equal(createHash('sha256').update(await decoded.body()).digest('hex'),createHash('sha256').update(webp).digest('hex'))
 await page.goto(origin+'/products/'+created.id);await page.getByRole('button',{name:'SKU images',exact:true}).click();editor=page.getByRole('dialog',{name:'SKU images',exact:true});await expect(editor.getByRole('checkbox',{name:'Image 2',exact:true})).toBeChecked();await editor.getByRole('checkbox',{name:'Image 2',exact:true}).uncheck();await editor.getByRole('radio',{name:'Clear thumbnail',exact:true}).check();await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 assigned=await get('products/'+created.id+'/variants/'+variant.id+'/images');assert.deepEqual(assigned.selected_ids,[]);assert.equal(assigned.thumbnail,null)
 await page.getByRole('button',{name:'Manage images',exact:true}).click();editor=page.getByRole('dialog',{name:'Manage images',exact:true});await expect(editor.getByRole('button',{name:'Remove',exact:true}).last()).toBeEnabled();await editor.getByRole('button',{name:'Remove',exact:true}).last().click();await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 assert.equal((await context.request.get(origin+secondURL)).status(),404);assert.equal((await context.request.get(origin+firstURL)).status(),200)
 await page.reload();await page.getByRole('button',{name:'SKU images',exact:true}).click();await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('button',{name:'SKU 图片',exact:true}).click();await expect(page.getByRole('dialog').getByRole('radio',{name:'清空缩略图',exact:true})).toBeChecked();assert.deepEqual(errors,[])
 console.log('VARIANT_IMAGES_NATIVE '+JSON.stringify({real_browser_react_gin_astro_native_mysql_redis:true,api_mocking:false,assign_and_thumbnail:true,refresh_preserved:true,public_same_variant_images:true,astro_gallery_uses_selected_sku_image:true,astro_exact_webp_bytes:true,explicit_unlink_and_clear:true,product_image_removable_after_unlink:true,remaining_image_available:true,admin_en_zh_mobile:true,browser_errors:0}))
}catch(e){console.error('VISIBLE_DIALOGS',await page?.getByRole('dialog').allTextContents());throw e}finally{await browser?.close();await server?.close()}
