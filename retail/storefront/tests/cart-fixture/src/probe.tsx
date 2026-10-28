import {useEffect,useMemo,useState} from 'react'
import LegacyCart from 'cart-oracle:template'
import CartTemplate from '../../../src/components/cart'
import type {CartDisplay} from '../../../src/components/cart/types'
import {Navigation} from '../../presentation-fixture/src/navigation'
import {product} from '../../selection-fixture/src/product'
import type {Variant} from '../../../src/lib/products'
import {updateLineItem,deleteLineItem} from './commands'

function initial(mode:string):CartDisplay{
 const items=mode==='empty'?[]:[
  {id:'line-two',title:'Saved phone two',product_title:'iPhone 13',product_handle:'iphone-13',thumbnail:'/images/test-model.svg',variant_title:'256GB / Blue / A / 100%',variant_id:'cart-v',quantity:2,subtotal:'399'},
  {id:'line-one',title:'Saved phone one',product_title:'iPhone 12',product_handle:'iphone-12',thumbnail:null,variant_title:'128GB / Red / S / 85%',variant_id:'retired-v',quantity:1,subtotal:'120'},
 ]
 if(mode==='large'&&items.length)items[0].subtotal='9007199254740993.005'
 return {currency_code:'eur',items,shipping_methods:mode==='totals'?[{id:'saved-delivery'}]:[],shipping_subtotal:mode==='totals'?'10':'0',discount_total:mode==='totals'?'25':'0',tax_total:mode==='totals'?'50':'0',total:mode==='large'?'9007199254741113.005':mode==='totals'?'554':'519'}
}
export default function CartProbe({reference,segment,mode}:{reference:boolean;segment:string;mode:string}){
 const [ready,setReady]=useState(false),[cart,setCart]=useState(()=>initial(mode))
 const variants=useMemo<Record<string,Variant>>(()=>{
  const v=structuredClone(product.variants[0]);v.id='cart-v';v.title='256GB / Blue / A / 100%';v.configuration_complete=true
  v.options=['256GB','Blue','A','100%'].map((value,i)=>({id:'choice-'+i,option_id:'option-'+i,value}))
  v.display={slug:'iphone-13-256gb-blue-A100',demo_price_basis:mode==='demo'?'model-starting-price':null,apple_launch_price:null}
  v.manage_inventory=true;v.allow_backorder=false;v.inventory_quantity=mode==='unavailable'?'0':'4';v.availability={state:mode==='unavailable'?'out_of_stock':'available',can_fulfill:mode!=='unavailable',max_fulfillable_quantity:mode==='unavailable'?'0':'4'}
  const result:Record<string,Variant>={}
  if(mode!=='missing')result['cart-v']=v
  return result
 },[mode])
 useEffect(()=>{
  setReady(true)
  const changed=(e:Event)=>{
   const {kind,lineId,quantity}=(e as CustomEvent<{kind:string;lineId:string;quantity:number}>).detail
   setCart(current=>{
    const items=current.items.filter(item=>kind!=='remove'||item.id!==lineId).map(item=>item.id===lineId&&kind==='quantity'?{...item,quantity,subtotal:(199.5*quantity).toString()}:item)
    return {...current,items,total:items.reduce((n,item)=>n+Number(item.subtotal),0).toString()}
   })
  }
  window.addEventListener('cart-fixture-done',changed);return()=>window.removeEventListener('cart-fixture-done',changed)
 },[])
 const legacy={...cart,items:cart.items.map((item,i)=>({...item,created_at:i===0?'2026-01-02':'2026-01-01',subtotal:Number(item.subtotal),unit_price:Number(item.subtotal)/item.quantity})),shipping_subtotal:Number(cart.shipping_subtotal),discount_total:Number(cart.discount_total),tax_total:Number(cart.tax_total),total:Number(cart.total)}
 const oldVariants=Object.fromEntries(Object.entries(variants).map(([id,v])=>[id,{...v,inventory_quantity:mode==='unavailable'?0:4,metadata:{slug:v.display.slug,demo_price_basis:v.display.demo_price_basis}}]))
 return <Navigation.Provider value={{segment,location:`/${segment}/cart`}}><main data-ready={ready}>
  {reference?<LegacyCart cart={legacy} customer={mode==='totals'?{}:null} locale={segment} variants={oldVariants}/>:<CartTemplate cart={cart} customer={mode==='totals'} locale={segment} variants={variants} checkoutHref={mode==='closed'?null:'/checkout?step=address'} commands={{enabled:mode!=='readonly',updateLineItem,deleteLineItem}}/>}
 </main></Navigation.Provider>
}
