import assert from 'node:assert/strict'
import {expect} from '../../admin/node_modules/@playwright/test/index.mjs'

// Called only by the owned Windows product-entry runner, after stock creation.
export async function verifyCatalogOperations({page,context,origin,productID,variantID,region,sku}){
 const get=async path=>{const response=await context.request.get(origin+'/api/'+path);assert.equal(response.status(),200);return response.json()}
 const base='admin/products/'+productID,variant=base+'/variants/'+variantID
 await page.goto(origin+'/option-definitions')
 await page.getByRole('button',{name:'Create shared option',exact:true}).click()
 let shared=page.getByRole('dialog')
 const sharedTitle='Native shared '+productID
 await shared.getByLabel('Name',{exact:true}).fill(sharedTitle)
 await shared.getByLabel('Option value',{exact:true}).fill('Verified value')
 await shared.getByRole('button',{name:'Save',exact:true}).click();await expect(shared).toHaveCount(0)
 const definitions=await get('admin/option-definitions?offset=0&q='+encodeURIComponent(sharedTitle))
 assert.ok(definitions.options.some(o=>o.title===sharedTitle&&o.enabled&&o.values.some(v=>v.value==='Verified value')))
 await page.goto(origin+'/products/'+productID)
 await page.getByRole('button',{name:'Manage images',exact:true}).click()
 let editor=page.getByRole('dialog',{name:'Manage images',exact:true})
 const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAE0lEQVR4nGP4z8DA8B+MGP4zAAAd8AP9pokTAAAAAABJRU5ErkJggg==','base64')
 await editor.locator('input[type=file]').setInputFiles({name:'catalog.png',mimeType:'image/png',buffer:png})
 await expect(editor.getByText('catalog.png',{exact:true})).toBeVisible()
 await editor.getByRole('radio',{name:'Use as thumbnail',exact:true}).check()
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const images=await get(base+'/images');assert.equal(images.enabled,true);assert.equal(images.images.length,1)
 const imageURL=images.images[0].url;assert.equal(images.thumbnail,imageURL)
 const image=await context.request.get(new URL(imageURL,origin).href);assert.equal(image.status(),200);assert.deepEqual(await image.body(),png)

 await page.getByRole('button',{name:'Manage options',exact:true}).click()
 editor=page.getByRole('dialog',{name:'Manage options',exact:true})
 const values=editor.getByRole('combobox',{name:'Option values Storage',exact:true})
 await values.fill('512 GB');await values.press('Enter');await values.press('Escape')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const options=await get(base+'/options');assert.ok(options.options.find(o=>o.title==='Storage').values.some(v=>v.value==='512 GB'&&v.selected))

 await page.getByRole('button',{name:'Edit SKU',exact:true}).click()
 editor=page.getByRole('dialog',{name:'Edit SKU',exact:true})
 await editor.getByLabel('Material',{exact:true}).fill('Native catalog verification')
 await editor.getByLabel('Storage',{exact:true}).focus();await editor.getByLabel('Storage',{exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText('256 GB',{exact:true}).click()
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const edited=await get(variant+'/edit');assert.equal(edited.variant.sku,sku);assert.equal(edited.variant.material,'Native catalog verification')
 assert.ok(edited.options.find(o=>o.title==='Storage').values.some(v=>v.value==='256 GB'&&edited.selected_ids.includes(v.id)))

 await page.getByRole('button',{name:'SKU images',exact:true}).click()
 editor=page.getByRole('dialog',{name:'SKU images',exact:true})
 await editor.getByRole('checkbox',{name:'Image 1',exact:true}).check()
 await editor.getByRole('radio',{name:'Thumbnail 1',exact:true}).check()
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const assigned=await get(variant+'/images');assert.deepEqual(assigned.selected_ids,[images.images[0].id]);assert.equal(assigned.thumbnail,imageURL)

 await page.getByRole('button',{name:'View inventory',exact:true}).click()
 await page.getByRole('button',{name:'Manage inventory links',exact:true}).click()
 editor=page.getByRole('dialog',{name:'Manage inventory links',exact:true})
 const before=await get(variant+'/inventory-items');assert.equal(before.items.length,1)
 await editor.getByRole('spinbutton',{name:'Required per product 1',exact:true}).fill('2')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const after=await get(variant+'/inventory-items');assert.equal(after.items[0].link_id,before.items[0].link_id);assert.equal(after.items[0].required_quantity,'2.000000000000000000')
 const product=(await get('store/products/'+productID+'?region_id='+region.id)).product
 assert.equal(product.variants[0].calculated_price.calculated_amount,'129.123456789012345678');assert.equal(String(product.variants[0].inventory_quantity),'1');assert.equal(product.thumbnail,imageURL)
 return {variant_id:variantID,image_url:imageURL,actual_shared_option_creation:true,actual_images_options_sku_inventory_edits:true,original_sku_and_link_preserved:true,uploaded_image_bytes_identical:true,public_inventory_quantity:1}
}
