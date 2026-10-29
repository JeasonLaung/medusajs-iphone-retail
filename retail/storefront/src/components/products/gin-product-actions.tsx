import {useEffect,useState} from 'react'
import ProductActions,{type ProductActionsProps,type ProductCommerce} from './product-actions'
import {browserCartClient} from '../../lib/cart-client.ts'
import {browserCartCommerce} from '../../lib/cart-commerce.ts'
import {cartVariantQuantities,type CartQuantities} from '../../lib/cart-quantities.ts'

export type GinProductActionsProps=Omit<ProductActionsProps,'commerce'|'cartQuantities'|'cartError'> & {
 regionID:string;currency:string;commerceEnabled?:boolean
}
type State={context:string;kind:'ready';commerce:ProductCommerce;quantities:CartQuantities}|{context:string;kind:'error'}|null
const empty:CartQuantities={}

// SSR/first hydration keep purchasing disabled until the private cart has been
// resolved. Catalog configuration remains usable independently of that cart.
// No credential or private cart is serialized in props.
export default function GinProductActions(props:GinProductActionsProps){
 const {regionID,currency,segment,commerceEnabled=false}=props
 const context=JSON.stringify([regionID,currency,segment,commerceEnabled])
 const [state,setState]=useState<State>(null)
 useEffect(()=>{
  if(!commerceEnabled)return
  let active=true
  const initialize=async()=>{
   try{
    const client=browserCartClient(),cart=await client.current()
    let quantities=empty
    if(cart){
     if(cart.region_id!==regionID||cart.currency_code!==currency)throw Error('Cart region unavailable')
     const amounts=await client.retrieveAmounts(cart.id)
     if(!amounts||amounts.currency_code!==currency||BigInt(amounts.version)<BigInt(cart.version))throw Error('Cart amounts unavailable')
     quantities=cartVariantQuantities(amounts.lines)
    }
    const commerce=browserCartCommerce({enabled:true,regionID,currency,segment})
    if(!commerce.enabled)throw Error('Cart capabilities unavailable')
    if(active)setState({context,kind:'ready',commerce,quantities})
   }catch{if(active)setState({context,kind:'error'})}
  }
  void initialize()
  return()=>{active=false}
 },[regionID,currency,segment,commerceEnabled,context])
 const current=state?.context===context?state:null
 const ready=current?.kind==='ready'?current:null
 return <ProductActions {...props} commerce={ready?.commerce??{enabled:false}} cartQuantities={ready?.quantities??empty} disabled={props.disabled} cartError={current?.kind==='error'}/>
}
