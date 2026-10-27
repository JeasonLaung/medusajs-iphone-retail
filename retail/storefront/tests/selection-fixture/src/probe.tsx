import {useEffect,useState} from 'react'
import {useProductSelection,type ProductSelectionProps} from '../../../src/lib/use-product-selection.ts'
import {variantPrice} from '../../../src/lib/products.ts'

// Test-only view for exercising the real hook in two hydrated Astro islands.
// These simple controls are never shipped as the storefront product UI.
export default function SelectionProbe(props: ProductSelectionProps & {kind: 'actions' | 'gallery'}) {
 const state = useProductSelection(props), [ready,setReady] = useState(false)
 useEffect(()=>setReady(true),[])
 return <section data-testid={props.kind} data-ready={ready}>
  <output data-testid={`${props.kind}-variant`}>{state.variant?.id ?? 'none'}</output>
  {props.kind === 'actions' ? <>
   <output data-testid="price">{variantPrice(state.variant)?.calculated_price ?? 'pending'}</output>
   {['128','256','512','1024','unknown'].map(value=><button key={value} onClick={()=>state.selectOption('storage',value)}>Select {value}</button>)}
  </> : <output data-testid="gallery-image">{state.images[0]?.url ?? 'none'}</output>}
 </section>
}
