import {useCallback,useEffect,useMemo,useSyncExternalStore} from 'react'
import type {Product} from './products.ts'
import {completeVariants,galleryImages,initialVariant,preferredOptionVariant,purchaseOptions,selectionMetadata,selectionURL,variantOptions} from './product-variants.ts'

const locationEvent = 'eurorefurb:product-location'
const selections = new Map<string,{location: string; id: string}>()
const subscribers = new Map<string,number>()
function address(): string { return window.location.pathname + window.location.search + window.location.hash }
function snapshot(productID: string): string {
 const location = address(), selected = selections.get(productID)
 return JSON.stringify([location,selected?.location === location ? selected.id : null])
}
function subscribe(productID: string,onChange: () => void): () => void {
 subscribers.set(productID,(subscribers.get(productID) ?? 0)+1)
 const onPopState = () => { selections.delete(productID); onChange() }
 window.addEventListener('popstate',onPopState)
 window.addEventListener(locationEvent,onChange)
 return () => {
  window.removeEventListener('popstate',onPopState); window.removeEventListener(locationEvent,onChange)
  const remaining = (subscribers.get(productID) ?? 1)-1
  if (remaining) subscribers.set(productID,remaining); else { subscribers.delete(productID); selections.delete(productID) }
 }
}
function parse(location: string): URL {
 if (!location.startsWith('/') || location.startsWith('//') || /[\\\x00-\x1f\x7f]/.test(location)) throw new Error('Invalid product location')
 return new URL(location,'https://store.invalid')
}
export type ProductSelectionProps = {product: Product; segment: string; initialLocation: string; initialVariantID?: string; updateMetadata?: boolean}

// A single URL snapshot feeds independently hydrated action/gallery islands.
// The server and first hydration share initialLocation; later renders observe
// browser state. No global history monkey patch, network request or account data.
export function useProductSelection({product,segment,initialLocation,initialVariantID,updateMetadata = true}: ProductSelectionProps) {
 const serverSnapshot = useCallback(()=>JSON.stringify([initialLocation,null]),[initialLocation])
 const getSnapshot = useCallback(()=>snapshot(product.id),[product.id])
 const listen = useCallback((changed: () => void)=>subscribe(product.id,changed),[product.id])
 const token = useSyncExternalStore(listen,getSnapshot,serverSnapshot)
 const [location,transientID] = useMemo(()=>JSON.parse(token) as [string,string | null],[token])
 const current = useMemo(()=>parse(location),[location])
 const transient = useMemo(()=>completeVariants(product).find(v=>v.id === transientID),[product,transientID])
 const variant = useMemo(()=>transient ?? initialVariant(product,current.pathname,current.searchParams.get('v_id'),initialVariantID),[product,current,initialVariantID,transient])
 const options = useMemo(()=>variant ? variantOptions(variant) : {},[variant])
 const images = useMemo(()=>transient ? (transient.images.length ? transient.images : product.images).filter(im=>im.url) : galleryImages(product,current.pathname,current.searchParams.get('v_id'),initialVariantID),[product,current,initialVariantID,transient])
 const selectOption = useCallback((optionID: string,value: string): boolean => {
  // Read the current address again so consecutive events do not use a stale URL
  // or discard query/hash changes made since React's last committed render.
  const before = snapshot(product.id), latest = parse(address()), remembered = selections.get(product.id)
  const selected = (remembered?.location === address() ? completeVariants(product).find(v=>v.id === remembered.id) : undefined) ?? initialVariant(product,latest.pathname,latest.searchParams.get('v_id'),initialVariantID)
  const next = preferredOptionVariant(product,selected ? variantOptions(selected) : {},optionID,value)
  if (!next) return false
  const target = selectionURL(segment,latest.pathname,latest.search,latest.hash,next)
  if (target !== address()) window.history.replaceState(window.history.state,'',target)
  selections.set(product.id,{location:address(),id:next.id})
  if (snapshot(product.id) !== before) window.dispatchEvent(new Event(locationEvent))
  return true
 },[product,segment,initialVariantID])
 useEffect(()=>{
  if (!updateMetadata) return
  const metadata = selectionMetadata(product,variant,current.pathname)
  if (!metadata) return
  document.title = metadata.title
  const canonical = document.querySelector<HTMLLinkElement>('link[rel="canonical"]')
  if (canonical) canonical.href = new URL(metadata.canonicalPath,canonical.href).href
 },[product,variant,current.pathname,updateMetadata])
 return {variant,options,images,location,purchaseOptions:purchaseOptions(product),selectOption}
}
