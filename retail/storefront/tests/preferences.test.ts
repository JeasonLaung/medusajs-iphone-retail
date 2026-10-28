import test from 'node:test'
import assert from 'node:assert/strict'
import {preferenceTarget} from '../src/lib/preferences.ts'
import type {Region} from '../src/lib/regions.ts'

const region:Region={id:'region-eu',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:['dk','de'].map(iso=>({iso_2:iso,iso_3:iso,num_code:'1',name:iso,display_name:iso,region_id:'region-eu'}))}
const input={regions:[region],locales:[],segment:'dk-en',pathname:'/dk-en/products/iphone-14',search:'?v_id=sku%2B1&sort=price',hash:'#configuration',country:'de',language:'zh-CN'}
test('preferences preserve exact path/query/hash and resolve the actual assigned region without side effects',()=>{
 const before=JSON.stringify(input)
 assert.deepEqual(preferenceTarget(input),{country:'de',locale:'zh-CN',regionID:'region-eu',currency:'eur',segment:'de-zh',currentPath:'/products/iphone-14?v_id=sku%2B1&sort=price#configuration',location:'/de-zh/products/iphone-14?v_id=sku%2B1&sort=price#configuration'})
 assert.equal(JSON.stringify(input),before)
 assert.equal(preferenceTarget({...input,pathname:'/dk-en',search:'',hash:'',language:'en'}).location,'/de-en/')
})
test('preferences reject foreign path/country, ambiguous assignments and unavailable locales',()=>{
 for(const change of [{pathname:'//foreign.example'},{pathname:'/dk-enemy/store'},{pathname:'/dk-en/store\\evil'},{search:'?bad\nheader'},{hash:'fragment'},{search:'sort=x'},{country:'fr'},{country:'DE'},{language:'fr'},{language:'en\n'},{regions:[region,region]},{regions:[{...region,countries:[region.countries[0],region.countries[0]]}],country:'dk'}])assert.throws(()=>preferenceTarget({...input,...change}))
})
test('configured language names/variants stay explicit; original routes keep en/zh language mapping',()=>{
 assert.equal(preferenceTarget({...input,locales:[{code:'de-DE',name:'Deutsch'}],language:'de-DE'}).location,'/de-en/products/iphone-14?v_id=sku%2B1&sort=price#configuration')
 assert.throws(()=>preferenceTarget({...input,locales:[{code:'en-GB',name:'English'}],language:'en'}))
 assert.equal(preferenceTarget({...input,locales:[{code:'zh-Hans',name:'中文'}],language:'zh-Hans'}).locale,'zh-Hans')
})
