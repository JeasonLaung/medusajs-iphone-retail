import {useEffect,useState} from 'react'
import LegacyGallery from 'legacy:gallery'
import LegacyDetails from 'legacy:details'
import ImageGallery from '../../../src/components/products/image-gallery'
import ProductDetails from '../../../src/components/products/product-details'
import {useProductSelection} from '../../../src/lib/use-product-selection'
import {product as initial} from '../../selection-fixture/src/product'
import {Navigation} from './navigation'
import styles from '../../../src/components/products/styles.module.css'

export default function Presentation({reference,segment,initialLocation,mode}: {reference:boolean;segment:string;initialLocation:string;mode:string}) {
 const [ready,setReady]=useState(false)
 useEffect(()=>setReady(true),[])
 const product=structuredClone(initial)
 product.description=mode==='empty'?null:'Original model description.\n第二行机型说明 <script>no HTML</script>'
 product.display.model_specs=mode==='empty'?null:{release_year:'2021',display_size_inches:'6.1',display_name:'Super Retina XDR',display_resolution_px:'2532 × 1170',chip:'A15 Bionic',rear_camera_system:'Dual 12MP',front_camera:'12MP',height_mm:'146.7',width_mm:'71.5',depth_mm:'7.65',weight_g:'174',connector:'Lightning',biometric:'Face ID',apple_model_numbers:['A2482','A2631'],tech_specs_url:mode==='unsafe'?'https://example.invalid/private':'https://support.apple.com/en-us/111872'}
 const image=(id:string)=>({id,url:`/images/${id}.svg`,rank:0})
 product.images=mode==='empty'?[]:[image('model'),image('model-two'),image('model-three')]
 product.variants[0].images=[image('128'),image('128-two')]
 product.variants[1].images=[image('256')]
 product.variants[2].images=[]
 const legacy={...product,metadata:{apple_model_specs:product.display.model_specs},variants:product.variants.map(v=>({...v,metadata:{slug:v.display.slug}}))}
 const props={product,segment,initialLocation}
 return <Navigation.Provider value={{segment,location:initialLocation}}><main data-ready={ready} className={styles.page}>
  <div data-testid="gallery-region" style={{maxWidth:560}}>{reference?<LegacyGallery images={product.images} variants={legacy.variants} title={product.title}/>:<ImageGallery {...props}/>}</div>
  <div data-testid="details-region">{reference?<LegacyDetails product={legacy} locale={segment}/>:<ProductDetails product={product} locale={segment}/>}</div>
  {!reference&&<Controls {...props}/>}
 </main></Navigation.Provider>
}
function Controls(props:Parameters<typeof useProductSelection>[0]) {
 const selection=useProductSelection({...props,updateMetadata:false})
 return <aside><button onClick={()=>selection.selectOption('storage','256')}>Choose 256</button><button onClick={()=>selection.selectOption('storage','512')}>Choose 512</button></aside>
}
