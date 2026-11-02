import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync,readdirSync} from 'node:fs'
import {publicImageURL} from '../src/lib/public-image-url.ts'

test('all original catalog thumbnails are served by Astro with identical bytes',()=>{
 const local=new URL('../public/swappie/',import.meta.url)
 const source=new URL('../../../medusa-store/apps/storefront/public/swappie/',import.meta.url)
 const names=readdirSync(local).filter(name=>name.endsWith('.png'))
 assert.equal(names.length,30)
 for(const name of names){
  assert.equal(publicImageURL(`http://127.0.0.1:8000/swappie/${name}`),`/swappie/${name}`)
  assert.equal(publicImageURL(`http://localhost:8000/swappie/${name}`),`/swappie/${name}`)
  assert.deepEqual(readFileSync(new URL(name,local)),readFileSync(new URL(name,source)))
 }
})
test('other assets and external image URLs retain their existing meaning',()=>{
 for(const value of ['/swappie/swappie-iphone-11-purple.png','https://cdn.example/swappie/swappie-iphone-11-purple.png','http://127.0.0.1:8000/private.png','http://127.0.0.1:9001/swappie/swappie-iphone-11-purple.png','http://user:pass@localhost:8000/swappie/swappie-iphone-11-purple.png','http://localhost:8000/swappie/swappie-iphone-11-purple.png?token=x'])assert.equal(publicImageURL(value),value)
})
