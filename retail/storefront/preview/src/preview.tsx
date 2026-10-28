// Local visual preview only. No credentials, Gin writes or checkout.
import {useEffect,useMemo,useState} from 'react'
import ProductActions from '../../src/components/products/product-actions'
import ImageGallery from '../../src/components/products/image-gallery'
import ProductDetails from '../../src/components/products/product-details'
import CartTemplate from '../../src/components/cart'
import BrandLogo from '../../src/components/layout/brand-logo'
import type {Product} from '../../src/lib/products'
import type {CartDisplay} from '../../src/components/cart/types'
import {product as base} from '../../tests/selection-fixture/src/product'
import styles from '../../src/components/products/styles.module.css'

function sample():Product{
 const p=structuredClone(base)
 p.id='preview-iphone-14';p.title='iPhone 14';p.handle='iphone-14';p.description='Local preview / 本地测试商品。配置、价格和库存为演示数据。'
 p.options=[['storage','Storage',['128GB','256GB']],['condition','Condition',['S','A']],['battery','Battery',['85','100']],['color','Color',['Blue','Red']]].map(([id,title,values])=>({id:id as string,title:title as string,values:(values as string[]).map(value=>({id:`${id}-${value}`,value,rank:0}))}))
 p.images=[{id:'blue',url:'/iphone-thumbnails/iphone-14/blue.webp',rank:0},{id:'red',url:'/iphone-thumbnails/iphone-14/red.webp',rank:1}]
 p.display.colors=[{name:'Blue',hex:'#1677FF'},{name:'Red',hex:'#F5222D'}]
 p.variants=[]
 for(const storage of ['128GB','256GB'])for(const grade of ['S','A'])for(const battery of ['85','100'])for(const color of ['Blue','Red']){
  const v=structuredClone(base.variants[0]);v.id=[storage,grade,battery,color].join('-');v.title=[storage,color,grade,battery+'%'].join(' / ');v.sku=v.id
  v.options=[storage,grade,battery,color].map((value,i)=>({id:`${p.options[i].id}-${value}`,option_id:p.options[i].id,value}))
  v.images=[p.images[color==='Blue'?0:1]];v.display.slug=`iphone-14-${v.id.toLowerCase()}`
  v.calculated_price!.calculated_amount=storage==='128GB'?'499':'599'
  p.variants.push(v)
 }
 return p
}
export default function Preview({segment,initialLocation}:{segment:string;initialLocation:string}){
 const product=useMemo(sample,[]),zh=segment.endsWith('-zh')
 const [view,setView]=useState('product'),[ready,setReady]=useState(false)
 const [quantities,setQuantities]=useState<Record<string,string>>({[product.variants[0].id]:'1'})
 useEffect(()=>{const update=()=>setView(location.hash==='#cart'?'cart':'product');update();setReady(true);addEventListener('hashchange',update);return()=>removeEventListener('hashchange',update)},[])
 const variants=Object.fromEntries(product.variants.map(v=>[v.id,v]))
 const items=product.variants.filter(v=>Number(quantities[v.id]??0)>0).map(v=>({id:v.id,variant_id:v.id,title:product.title,product_title:product.title,product_handle:product.handle,thumbnail:v.images[0].url,variant_title:v.title,quantity:Number(quantities[v.id]),subtotal:(Number(v.calculated_price!.calculated_amount)*Number(quantities[v.id])).toString()}))
 const cart:CartDisplay={currency_code:'eur',items,shipping_methods:[],shipping_subtotal:'0',discount_total:'0',tax_total:'0',total:items.reduce((n,i)=>n+Number(i.subtotal),0).toString()}
 return <div data-preview-ready={ready}>
  <aside className="preview-notice">迁移组件预览 · 测试数据 · 操作仅在本页内存生效，刷新重置 · 结账未开放</aside>
  <nav className="preview-nav" aria-label="Preview navigation"><a href="#product"><BrandLogo/></a><div><a href="#product">{zh?'商品配置':'Product'}</a><a href="#cart">{zh?'购物车':'Cart'} ({items.reduce((n,i)=>n+i.quantity,0)})</a><a href={zh?'/dk-en/preview':'/dk-zh/preview'}>{zh?'English':'中文'}</a></div></nav>
  {view==='cart'?<main><CartTemplate cart={cart} customer={false} locale={segment} variants={variants} checkoutHref={null} commands={{enabled:true,updateLineItem:async({lineId,quantity})=>{setQuantities(q=>({...q,[lineId]:String(quantity)}))},deleteLineItem:async(id)=>{setQuantities(q=>{const next={...q};delete next[id];return next})}}}/></main>:<main className={styles.page}>
   <div className={styles.hero}><div className={styles.visual}><ImageGallery product={product} segment={segment} initialLocation={initialLocation}/></div><ProductActions product={product} segment={segment} initialLocation={initialLocation} cartQuantities={quantities} commerce={{enabled:true,addToCart:async({variantId})=>{const next={...quantities,[variantId]:String(Number(quantities[variantId]??0)+1)};setQuantities(next);return {ok:true,quantities:next}},navigate:()=>{location.hash='cart'}}}/></div>
   <ProductDetails product={product} locale={segment}/>
  </main>}
 </div>
}
