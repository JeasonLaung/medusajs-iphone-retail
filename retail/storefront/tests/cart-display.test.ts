import test from 'node:test'
import assert from 'node:assert/strict'
import {cartDisplay,formatCartMoney,variantMaximum,isZeroAmount} from '../src/components/cart/types.ts'
import type {CartView} from '../src/lib/cart-client.ts'
import type {Variant} from '../src/lib/products.ts'

function fixture():CartView{
 return {amounts:{cart_id:'cart',version:'2',currency_code:'eur',pricing_recalculated:false,checkout:false,totals:{total:'9007199254740993.005',shipping_subtotal:'0',discount_total:'0',tax_total:'0'},shipping_methods:[],lines:[{id:'older',variant_id:null,quantity:'1',unit_price:'1',is_tax_inclusive:false,totals:{subtotal:'1'}},{id:'newer',variant_id:'retired',quantity:'1000000000',unit_price:'0.1',is_tax_inclusive:false,totals:{subtotal:'99999999.999999999999999999'}}]},items:['newer','older'].map(id=>({id,title:id,product_title:null,product_handle:null,variant_title:null,thumbnail:null,option_values:{}}))}
}
test('cart display joins exact amounts by ID and preserves saved order and retired lines',()=>{
 const input=fixture(),before=structuredClone(input),view=cartDisplay(input)
 assert.deepEqual(view.items.map(item=>item.id),['newer','older'])
 assert.equal(view.items[0].quantity,1000000000)
 assert.equal(view.items[0].subtotal,'99999999.999999999999999999')
 assert.equal(view.items[1].variant_id,null)
 assert.equal(view.total,'9007199254740993.005')
 assert.deepEqual(input,before)
 for(const bad of [(()=>{const v=fixture();v.items.pop();return v})(),(()=>{const v=fixture();v.items[1]=v.items[0];return v})(),(()=>{const v=fixture();v.amounts.lines[0].quantity='1000000001';return v})()])assert.throws(()=>cartDisplay(bad))
})
test('cart money retains exact large and signed major units, zero discounts remain hidden',()=>{
 assert.equal(formatCartMoney('9007199254740993.005','eur','dk-en'),'€9,007,199,254,740,993.01')
 assert.equal(formatCartMoney('-2.005','eur','dk-en'),'-€2.01')
 assert.equal(formatCartMoney('12.50','eur','dk-zh'),'€12.50')
 assert.ok(isZeroAmount('0.000'));assert.ok(isZeroAmount('-0'));assert.ok(!isZeroAmount('0.001'))
 for(const value of ['1e3','1\n','NaN','01','+1'])assert.throws(()=>formatCartMoney(value,'eur','dk-en'))
})
test('cart stock UI bounds large authoritative capacity without floating point overflow',()=>{
 const v={manage_inventory:true,allow_backorder:false,availability:{max_fulfillable_quantity:'9007199254740993'}} as Variant
 assert.equal(variantMaximum(v),1000000000)
 v.availability.max_fulfillable_quantity='4';assert.equal(variantMaximum(v),4)
 v.availability.max_fulfillable_quantity=null;assert.equal(variantMaximum(v),0)
 v.allow_backorder=true;assert.equal(variantMaximum(v),undefined)
 assert.equal(variantMaximum(undefined),undefined)
})
