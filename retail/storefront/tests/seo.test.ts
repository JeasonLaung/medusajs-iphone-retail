import {test} from 'node:test'
import assert from 'node:assert/strict'
import {defaultSEO,jsonLD,privateSEOPath} from '../src/lib/seo.ts'
test('default canonical and three language alternates preserve locale-independent path and omit query',()=>{
 const result=defaultSEO(new URL('http://localhost:9300/dk-zh/blog/a/?q=private#x'),{title:'Title',description:'Description'})
 assert.equal(result.canonical,'http://localhost:9300/dk-zh/blog/a')
 assert.deepEqual(result.languages,[{lang:'en',href:'http://localhost:9300/dk-en/blog/a'},{lang:'zh-CN',href:'http://localhost:9300/dk-zh/blog/a'},{lang:'x-default',href:'http://localhost:9300/dk-en/blog/a'}])
 assert.equal(result.openGraph.url,result.canonical);assert.equal(result.openGraph.description,'Description')
 assert.equal(result.structured[0]['@type'],'Organization');assert.equal(result.structured[1]['@type'],'WebSite')
 assert.equal(defaultSEO(new URL('http://localhost/'),{title:'Home'}).languages.length,0)
})
test('source private families remain noindex/nofollow regardless of explicit public defaults',()=>{
 for(const path of ['/dk-en/account','/dk-en/account/orders/a','/dk-en/checkout','/dk-en/cart','/dk-en/order/a/confirmed','/dk-en/payment-result','/dk-en/verify-account','/dk-en/sell/iphone/model','/dk-en/sell/requests/a']){
  assert.ok(privateSEOPath(path));assert.equal(defaultSEO(new URL('http://localhost'+path),{title:'Private',robots:'index, follow'}).robots,'noindex, nofollow')
 }
 for(const path of ['/dk-en/store','/dk-en/accounting','/dk-en/orderly','/dk-en/sell'])assert.equal(privateSEOPath(path),false)
 // Preserve the source regex's segment matching, even for a product named cart.
 assert.equal(privateSEOPath('/dk-en/products/cart'),true)
 assert.equal(defaultSEO(new URL('http://localhost/dk-en/ui'),{title:'UI',robots:'noindex, nofollow'}).robots,'noindex, nofollow')
 assert.equal(defaultSEO(new URL('http://localhost/not-found'),{title:'Missing'},404).robots,'noindex')
})
test('product canonical and Open Graph image defaults remain explicit',()=>{
 const result=defaultSEO(new URL('http://localhost/dk-en/products/model?v_id=private'),{title:'Product',canonicalPath:'/dk-en/products/sku',openGraph:{title:'OG product',description:'Details',images:['/photo.jpg','https://images.example/photo.jpg']}})
 assert.equal(result.canonical,'http://localhost/dk-en/products/sku');assert.equal(result.openGraph.title,'OG product')
 assert.deepEqual(result.openGraph.images,['http://localhost/photo.jpg','https://images.example/photo.jpg'])
})
test('JSON-LD is script-safe and round-trips its original data',()=>{
 const value={'@type':'Organization',name:'</script><script>alert(1)</script>&>'},text=jsonLD(value)
 assert.equal(text.includes('<'),false);assert.equal(text.includes('>'),false);assert.equal(text.includes('&'),false);assert.deepEqual(JSON.parse(text),value)
})
