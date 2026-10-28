import test from 'node:test'
import assert from 'node:assert/strict'
import {cartVariantQuantities,cartInventoryLimitReached} from '../src/lib/cart-quantities.ts'
import {product} from './selection-fixture/src/product.ts'

test('cart quantities aggregate exact whole units and keep arbitrary IDs as own keys',()=>{
 const result=cartVariantQuantities([{variant_id:'sku',quantity:'9007199254740993'},{variant_id:'sku',quantity:'2'},{variant_id:'__proto__',quantity:'7'},{variant_id:null,quantity:'1'}])
 assert.equal(result.sku,'9007199254740995');assert.equal(result.__proto__,'7');assert.equal(Object.getPrototypeOf(result),null)
 for(const quantity of ['-1','1.2','01','','1e3','9'.repeat(193)])assert.throws(()=>cartVariantQuantities([{variant_id:'sku',quantity}]))
})
test('cart limit honors Gin single-location bound instead of pooled inventory',()=>{
 const v=structuredClone(product.variants[0]);v.inventory_quantity='100';v.availability.max_fulfillable_quantity='2'
 assert.equal(cartInventoryLimitReached(v,{[v.id]:'1'}),false)
 assert.equal(cartInventoryLimitReached(v,{[v.id]:'2'}),true)
 v.availability.max_fulfillable_quantity='9007199254740994'
 assert.equal(cartInventoryLimitReached(v,{[v.id]:'9007199254740993'}),false)
 assert.equal(cartInventoryLimitReached(v,{[v.id]:'9007199254740994'}),true)
 assert.throws(()=>cartInventoryLimitReached(v,{[v.id]:'NaN'}))
})
test('unknown tracked capacity fails closed while backorder/untracked keep their separate availability gate',()=>{
 const v=structuredClone(product.variants[0]);v.availability.max_fulfillable_quantity=null
 assert.equal(cartInventoryLimitReached(v,{}),true)
 v.allow_backorder=true;assert.equal(cartInventoryLimitReached(v,{}),false)
 v.allow_backorder=false;v.manage_inventory=false;assert.equal(cartInventoryLimitReached(v,{}),false)
 assert.equal(cartInventoryLimitReached(undefined,{}),false)
})
