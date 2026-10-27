import { useEffect, useState } from 'react'
import { Alert, Button, Descriptions, Empty, Image, Result, Space, Spin, Table, Tag, Typography } from 'antd'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { PictureOutlined } from '@ant-design/icons'
import { APIError, productDetail, productVariants } from './api'
import type { ProductDetail as Product, VariantDetail, VariantList } from './api'
import type { Copy } from './i18n'
import ProductEdit from './ProductEdit'

// Render text from the database as text; do not inject description HTML.
function imageURL(value:string){try{const url=new URL(value,location.origin);return ['http:','https:'].includes(url.protocol)&&!url.username&&!url.password?url.href:undefined}catch{return undefined}}
function CatalogImage({url,alt,size,copy}:{url:string;alt:string;size:number;copy:Copy}) {
  const [failed,setFailed]=useState(false)
  const src=imageURL(url)
  if(!src||failed)return <span role="img" aria-label={copy.imageUnavailable} title={copy.imageUnavailable} className="unavailable-image" style={{width:size,height:size}}><PictureOutlined/></span>
  return <Image src={src} alt={alt} width={size} height={size} style={{objectFit:'contain'}} onError={()=>setFailed(true)}/>
}
export default function ProductDetail({copy,onExpired}:{copy:Copy;onExpired():void}) {
  const {id=''}=useParams()
  const [params,setParams]=useSearchParams()
  const query=params.toString()
  const [attempt,setAttempt]=useState(0)
  const [editing,setEditing]=useState(false)
  const [state,setState]=useState<{loading:boolean;product?:Product;variants?:VariantList;error?:unknown}>({loading:true})
  useEffect(()=>{
    const controller=new AbortController();setState({loading:true})
    Promise.all([productDetail(id,controller.signal),productVariants(id,new URLSearchParams(query),controller.signal)]).then(([product,variants])=>{
      if(!controller.signal.aborted)setState({loading:false,product,variants})
    }).catch(error=>{
      if(controller.signal.aborted)return
      if(error instanceof APIError&&error.status===401)onExpired()
      else setState({loading:false,error})
    })
    return()=>controller.abort()
  },[id,query,attempt,onExpired])
  const p=state.product,v=state.variants
  const statuses:Record<string,string>={draft:copy.draft,proposed:copy.proposed,published:copy.published,rejected:copy.rejected}
  return <section className="account-panel product-detail">
    <Link to="/products" className="back-link">{copy.backProducts}</Link>
    {state.loading?<Spin aria-label={copy.loadingProduct}/>:state.error!==undefined?
      state.error instanceof APIError&&state.error.status===404?<Result status="404" title={copy.productNotFound}/>:<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError&&state.error.status===400?copy.invalidFilters:copy.unavailable}/><Space><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button><Button onClick={()=>setParams(new URLSearchParams())}>{copy.reset}</Button></Space></Space>:
      p&&v&&<Space direction="vertical" size={32} className="full-width">
        <div><Space wrap><Typography.Title level={3}>{p.title}</Typography.Title><Button onClick={()=>setEditing(true)}>{copy.editProduct}</Button></Space><Descriptions column={{xs:1,sm:2}} items={[
          {key:'id',label:'ID',children:p.id},{key:'handle',label:'Handle',children:p.handle},
          {key:'status',label:copy.status,children:<Tag color={p.status==='published'?'success':p.status==='rejected'?'error':undefined}>{statuses[p.status]}</Tag>},
          {key:'count',label:copy.variants,children:p.variant_count},
          {key:'subtitle',label:copy.subtitle,children:p.subtitle||'—'},
        ]}/>{p.description&&<Typography.Paragraph className="product-description">{p.description}</Typography.Paragraph>}</div>
        <div><Typography.Title level={4}>{copy.images}</Typography.Title>{p.images.length?<Image.PreviewGroup><Space wrap>{p.images.map(i=><CatalogImage key={i.id} url={i.url} alt={p.title} size={96} copy={copy}/>)}</Space></Image.PreviewGroup>:<Typography.Text type="secondary">{copy.noImages}</Typography.Text>}</div>
        <div><Typography.Title level={4}>{copy.options}</Typography.Title>{p.options.length?<Descriptions column={1} items={p.options.map(o=>({key:o.id,label:o.title,children:o.values.length?o.values.map(v=>v.value).join(' / '):'—'}))}/>:<Typography.Text type="secondary">{copy.noOptions}</Typography.Text>}</div>
        <div><Typography.Title level={4}>{copy.variantDetails}</Typography.Title><Table<VariantDetail> rowKey="id" dataSource={v.variants} scroll={{x:860}} locale={{emptyText:<Empty description={copy.noVariants}/>}} expandable={{expandedRowRender:variant=><Space direction="vertical"><Typography.Text type="secondary">ID: {variant.id}</Typography.Text>{variant.unassigned_option_value_ids.length>0&&<Alert type="warning" showIcon message={copy.unassignedOptions} description={variant.unassigned_option_value_ids.join(', ')}/>}<Image.PreviewGroup><Space wrap>{variant.images.map(i=><CatalogImage key={i.id} url={i.url} alt={variant.title} size={80} copy={copy}/>)}</Space></Image.PreviewGroup></Space>}} columns={[
          {title:copy.productTitle,dataIndex:'title'},
          {title:'SKU',dataIndex:'sku',render:(value:string|null)=><Typography.Text className="identifier">{value??'—'}</Typography.Text>},
          {title:copy.options,dataIndex:'options',render:(values:VariantDetail['options'])=>values.map(v=>v.value).join(' / ')||'—'},
          {title:copy.configuration,dataIndex:'configuration_complete',render:(complete:boolean)=>complete?<Tag color="success">{copy.complete}</Tag>:<Tag color="warning">{copy.incomplete}</Tag>},
          {title:copy.manageInventory,dataIndex:'manage_inventory',render:(enabled:boolean)=>enabled?copy.yes:copy.no},
          {title:copy.allowBackorder,dataIndex:'allow_backorder',render:(enabled:boolean)=>enabled?copy.yes:copy.no},
        ]} pagination={{current:Math.floor(v.offset/v.limit)+1,pageSize:v.limit,total:v.count,showSizeChanger:true,pageSizeOptions:[20,50,100],onChange:(page,size)=>{const next=new URLSearchParams();next.set('limit',String(size));next.set('offset',String((page-1)*size));setParams(next)}}}/></div>
      </Space>}
    {editing&&p&&<ProductEdit key={p.id} product={p} copy={copy} onExpired={onExpired} onClose={()=>{setEditing(false);setAttempt(n=>n+1)}} onSaved={()=>{setEditing(false);setAttempt(n=>n+1)}}/>}
  </section>
}
