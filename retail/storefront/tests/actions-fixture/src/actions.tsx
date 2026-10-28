import {useEffect,useMemo,useState} from 'react'
import LegacyActions from 'legacy:actions'
import LegacyTheme from 'legacy:theme'
import ProductActions from '../../../src/components/products/product-actions'
import ImageGallery from '../../../src/components/products/image-gallery'
import {product as base} from '../../selection-fixture/src/product'
import {Navigation} from '../../presentation-fixture/src/navigation'
import {fixtureAdd} from '../../presentation-fixture/src/cart'
import type {Product} from '../../../src/lib/products'
import styles from '../../../src/components/products/styles.module.css'

function fixture(mode:string):Product {
 const product=structuredClone(base)
 product.options=[['storage','Storage',['128GB','256GB','512GB']],['condition','Condition',['S','A']],['battery','Battery',['85','100']],['color','Color',['Blue','Red']]].map(([id,title,values])=>({id:id as string,title:title as string,values:(values as string[]).map(value=>({id:`${id}-${value}`,value,rank:0}))}))
 product.images=[{id:'test-model',url:'/images/test-model.svg',rank:0}]
 product.display.colors=[{name:'Blue',hex:'#1677FF'},{name:'Red',hex:'#F5222D'}]
 product.variants=[]
 for(const storage of ['128GB','256GB'])for(const grade of ['S','A'])for(const battery of ['85','100'])for(const color of ['Blue','Red']){
  const v=structuredClone(base.variants[0]),id=`${storage}-${grade}-${battery}-${color}`
  v.id=id;v.title=id;v.sku=id;v.options=[storage,grade,battery,color].map((value,i)=>({id:`${product.options[i].id}-${value}`,option_id:product.options[i].id,value}))
  v.images=product.images;v.display.slug=`iphone-13-${storage.toLowerCase()}-${color.toLowerCase()}-${grade}${battery}`
  v.calculated_price!.calculated_amount=storage==='128GB'?'499':'599'
  if(mode==='large')v.calculated_price!.calculated_amount=storage==='128GB'?'9007199254740993.005':'9007199254740993.015'
  if(mode==='missing')v.calculated_price=null
  if(mode==='sale'){v.calculated_price!.original_amount='799';v.calculated_price!.calculated_price.price_list_type='sale'}
  if(mode==='demo')v.display.demo_price_basis='model-starting-price'
  if(mode==='held'){v.availability={state:'unavailable',can_fulfill:false,max_fulfillable_quantity:null}}
  if(mode==='single')v.availability.max_fulfillable_quantity='2'
  product.variants.push(v)
 }
 const incomplete=structuredClone(product.variants[0]);incomplete.id='incomplete';incomplete.configuration_complete=false;incomplete.options=[];incomplete.calculated_price=null;product.variants.push(incomplete)
 if(mode==='empty')product.variants=[]
 return product
}
export default function ActionsProbe({reference,segment,initialLocation,mode}: {reference:boolean;segment:string;initialLocation:string;mode:string}) {
 const [ready,setReady]=useState(false),[navigated,setNavigated]=useState('')
 useEffect(()=>{setReady(true);const navigation=(event:Event)=>setNavigated((event as CustomEvent<string>).detail);window.addEventListener('fixture-navigation',navigation);return()=>window.removeEventListener('fixture-navigation',navigation)},[])
 const product=useMemo(()=>fixture(mode),[mode]),quantities=useMemo(()=>mode==='limit'?{[product.variants[0].id]:'10'}:mode==='single'?{[product.variants[0].id]:'2'}:{},[product,mode])
 const legacy=useMemo(()=>({...product,metadata:{card_colors:product.display.colors},variants:product.variants.map(v=>({...v,inventory_quantity:10,metadata:{slug:v.display.slug,demo_price_basis:v.display.demo_price_basis},calculated_price:v.calculated_price?{...v.calculated_price,calculated_amount:Number(v.calculated_price.calculated_amount),original_amount:v.calculated_price.original_amount===null?null:Number(v.calculated_price.original_amount)}:null}))}),[product])
 const props={product,segment,initialLocation,cartQuantities:quantities}
 return <Navigation.Provider value={{segment,location:initialLocation}}><header style={{height:80,position:'sticky',top:0,background:'white',zIndex:60}}>Artificial header</header><main className={styles.page} data-ready={ready}>
  <section data-testid="actions-region" style={{maxWidth:520}}>{reference?<LegacyTheme locale={segment.endsWith('-zh')?'zh-CN':'en'}><LegacyActions product={legacy} cartQuantities={Object.fromEntries(Object.entries(quantities).map(([id,n])=>[id,Number(n)]))}/></LegacyTheme>:<ProductActions {...props} commerce={mode==='closed'?{enabled:false}:{enabled:true,addToCart:fixtureAdd,navigate:setNavigated}}/>}</section>
  {!reference&&<div data-testid="linked-gallery"><ImageGallery product={product} segment={segment} initialLocation={initialLocation}/></div>}
  <output data-testid="navigated">{navigated}</output><div style={{height:2000}}/>
 </main></Navigation.Provider>
}
