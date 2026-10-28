import type {CartView} from './cart-client.ts'
import {loadProduct,type Variant} from './products.ts'

// Like the original cart page's separate current-product lookup, this is only
// an observation. Saved labels/amounts and removal never depend on its success.
// No process cache: a new cart view gets a new stock observation.
export async function loadCartVariants(view:CartView,origin:string,regionID:string,currency:string,fetcher:typeof fetch=fetch,signal?:AbortSignal):Promise<Record<string,Variant>>{
 if(view.amounts.currency_code!==currency||view.items.length>1000||view.amounts.lines.length>1000)throw Error('Invalid cart observation context')
 const lines=new Map(view.amounts.lines.map(line=>[line.id,line])),wanted=new Map<string,Set<string>>()
 if(lines.size!==view.items.length)throw Error('Incomplete cart observation')
 for(const item of view.items){
  const line=lines.get(item.id)
  if(!line)throw Error('Incomplete cart observation')
  lines.delete(item.id)
  if(!line.variant_id||!item.product_id)continue
  let ids=wanted.get(item.product_id)
  if(!ids){ids=new Set();wanted.set(item.product_id,ids)}
  ids.add(line.variant_id)
 }
 const productIDs=[...wanted.keys()],result:Record<string,Variant>=Object.create(null)
 let next=0
 const transport:typeof fetch=(url,options)=>{
  if(signal?.aborted)return Promise.reject(Error('Observation cancelled'))
  return fetcher(url,{...options,signal:signal?(options?.signal?AbortSignal.any([signal,options.signal]):signal):options?.signal})
 }
 const worker=async()=>{
  while(next<productIDs.length&&!signal?.aborted){
   const productID=productIDs[next++]
   try{
    const product=await loadProduct(origin,productID,{region_id:regionID},transport)
    if(!product)continue
    for(const variant of product.variants){
     if(!wanted.get(productID)!.has(variant.id)||variant.calculated_price&&variant.calculated_price.currency_code!==currency)continue
     result[variant.id]=variant
    }
   }catch{/* Preserve original unknown-stock fallback, including retired SKUs. */}
  }
 }
 await Promise.all(Array.from({length:Math.min(4,productIDs.length)},worker))
 return result
}
