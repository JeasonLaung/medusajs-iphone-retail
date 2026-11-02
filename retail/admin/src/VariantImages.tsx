import {catalogImageURL} from './catalog-image-url'
import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Checkbox,Drawer,Empty,Image,Radio,Space,Spin,Typography} from 'antd'
import {APIError,readVariantImages,saveVariantImages} from './api'
import type {VariantDetail,VariantImageContext,VariantImageInput} from './api'
import type {Copy} from './i18n'
export default function VariantImages({productID,variant,copy,onClose,onSaved,onExpired}:{productID:string;variant:VariantDetail;copy:Copy;onClose():void;onSaved():void;onExpired():void}){
 const text=copy.language==='语言'?{title:'SKU 图片',intro:'从商品图库选择此 SKU 的图片。新图请先在商品图片管理中上传。',keep:'保留当前缩略图',none:'清空缩略图',image:'图片',thumbnail:'缩略图',disabled:'图片管理暂未启用。',conflict:'商品或 SKU 图片已变更，请重新加载后核对。',invalid:'请检查图片关联和缩略图选择。',reload:'重新加载图片'}:{title:'SKU images',intro:'Choose this SKU’s images from the product gallery. Upload new images in Manage images first.',keep:'Keep current thumbnail',none:'Clear thumbnail',image:'Image',thumbnail:'Thumbnail',disabled:'Image management is currently disabled.',conflict:'Product or SKU images changed. Reload and review before saving.',invalid:'Check image assignments and thumbnail selection.',reload:'Reload images'}
 const [context,setContext]=useState<VariantImageContext>(),[selected,setSelected]=useState<string[]>([]),[thumb,setThumb]=useState('none'),[loadingError,setLoadingError]=useState(false),[reload,setReload]=useState(0),[error,setError]=useState<string>(),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<VariantImageInput>()
 const active=useRef(false)
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadingError(false);setError(undefined);setAttempt(undefined);readVariantImages(productID,variant.id,abort.signal).then(d=>{if(!abort.signal.aborted){setContext(d);setSelected(d.selected_ids);setThumb(d.thumbnail?'keep':'none')}}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadingError(true)}});return()=>abort.abort()},[productID,variant.id,reload,onExpired])
 const disabled=saving||!!attempt||!context?.enabled
 const save=async(body:VariantImageInput)=>{if(active.current)return;active.current=true;setSaving(true);setAttempt(body);setError(undefined);try{await saveVariantImages(productID,variant.id,body);onSaved()}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else setError('uncertain')}finally{active.current=false;setSaving(false)}}
 return <Drawer open title={text.title} width="min(680px,100vw)" closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={()=>{if(!active.current)onClose()}}><Space direction="vertical" size={24} className="full-width">
  <Typography.Text>{variant.title} · {variant.sku??'—'}</Typography.Text><Alert type="info" showIcon message={text.intro}/>
  {loadingError?<><Alert type="error" message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></>:!context?<Spin/>:<>
   {!context.enabled&&<Alert type="warning" message={text.disabled}/>}
   {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='conflict'?text.conflict:error==='invalid'?text.invalid:error==='denied'?copy.accessDenied:copy.saveUncertain}/>}
   <Radio.Group value={thumb} disabled={disabled} onChange={e=>setThumb(e.target.value)} className="full-width"><Space direction="vertical" size={16} className="full-width">
    <Radio value="none">{text.none}</Radio>{context.thumbnail&&<Radio value="keep">{text.keep}</Radio>}
    {context.images.length===0&&<Empty description={copy.noImages}/>}
    {context.images.map((image,i)=><Space key={image.id} wrap><Image src={catalogImageURL(image.url)} width={88} height={88} style={{objectFit:'contain'}}/><Checkbox disabled={disabled} checked={selected.includes(image.id)} onChange={e=>{setSelected(ids=>e.target.checked?[...ids,image.id]:ids.filter(id=>id!==image.id));if(!e.target.checked&&thumb===image.id)setThumb('none')}}>{text.image} {i+1}</Checkbox><Radio value={image.id} disabled={disabled||!selected.includes(image.id)}>{text.thumbnail} {i+1}</Radio></Space>)}
   </Space></Radio.Group>
   {!attempt&&context.enabled&&<Button type="primary" disabled={disabled} loading={saving} onClick={()=>void save({operation_id:crypto.randomUUID(),expected_revision:context.revision,image_ids:selected,thumbnail_image_id:thumb!=='none'&&thumb!=='keep'?thumb:null,keep_thumbnail:thumb==='keep'})}>{copy.save}</Button>}
   {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void save(attempt)}>{copy.retrySave}</Button>}
   {error&&error!=='uncertain'&&<Button disabled={saving} onClick={()=>setReload(n=>n+1)}>{text.reload}</Button>}
  </>}
  <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
 </Space></Drawer>
}
