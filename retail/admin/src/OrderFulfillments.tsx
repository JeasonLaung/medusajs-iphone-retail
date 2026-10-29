import {useAccess} from './Permissions'
import OrderParcel from './OrderParcel'
import {useEffect,useState} from 'react'
import {Alert,Button,Descriptions,Empty,Space,Spin,Table,Tag,Typography} from 'antd'
import {APIError,getOrderFulfillments} from './api'
import type {OrderFulfillments as Evidence} from './api'
import type {Copy} from './i18n'
import {amountText} from './VariantPrices'

// Original URLs remain text unless they are absolute HTTP(S) links. Rendering
// never fetches labels or embeds a historical provider document automatically.
function evidenceLink(value:string,label:string) {
 try {const url=new URL(value);if(['https:','http:'].includes(url.protocol)&&!url.username&&!url.password)return <Typography.Link href={value} target="_blank" rel="noopener noreferrer">{label}</Typography.Link>} catch {/* Retain unsupported original values as text. */}
 return <Typography.Text className="price-rule-value">{value||'—'}</Typography.Text>
}
export default function OrderFulfillments({id,version,copy,onExpired,onChanged}:{id:string;version:number;copy:Copy;onExpired():void;onChanged?():void}) {
 const access=useAccess()
 const [operating,setOperating]=useState<string>()
 const [attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{loading:boolean;data?:Evidence;error?:unknown}>({loading:true})
 useEffect(()=>{
  const controller=new AbortController();setState({loading:true})
  getOrderFulfillments(id,controller.signal).then(data=>{
   if(controller.signal.aborted)return
   if(data.version!==version)throw new APIError(0,'orders.version_changed')
   setState({loading:false,data})
  }).catch(error=>{if(controller.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setState({loading:false,error})})
  return()=>controller.abort()
 },[id,version,attempt,onExpired])
 const text=(v:string|null)=><span className="price-rule-value">{v===null?'—':v}</span>
 const time=(v:string|null)=>v?<time dateTime={v}>{new Date(v).toLocaleString(document.documentElement.lang)}</time>:'—'
 return <section aria-label={copy.orderFulfillments}><Space direction="vertical" size={16} className="full-width">
  <Typography.Title level={4}>{copy.orderFulfillments}</Typography.Title>
  {state.loading?<Spin/>:state.error!==undefined?<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError&&state.error.code==='orders.version_changed'?copy.orderVersionChanged:copy.unavailable}/><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>:state.data&&<>
   <Alert type="info" showIcon message={copy.fulfillmentEvidenceInfo}/>
   {state.data.fulfillments.length===0?<Empty description={copy.noFulfillments}/>:state.data.fulfillments.map(link=>{
    const r=link.record,a=r?.address
    return <Space direction="vertical" size={16} key={link.id} data-fulfillment-id={link.id} className="full-width">
     <Typography.Text strong className="price-rule-value">{link.id}</Typography.Text>
     {r&&access.can('order.allocation.write','order.payment.read','device.read')&&<Button onClick={()=>setOperating(link.id)}>{copy.parcelOperations}</Button>}
     {!r?<Alert type="warning" showIcon message={link.availability==='deleted'?copy.fulfillmentDeleted:copy.fulfillmentMissing}/>:<>
      <Descriptions column={{xs:1,sm:2,lg:3}} items={[
       {key:'location',label:copy.fulfillmentLocation,children:text(r.location_id)},
       {key:'provider',label:copy.fulfillmentProvider,children:text(r.provider_id)},
       {key:'option',label:copy.fulfillmentShippingOption,children:text(r.shipping_option_id)},
       {key:'shipping',label:copy.requiresShipment,children:<Tag>{r.requires_shipping?copy.yes:copy.no}</Tag>},
       {key:'created',label:copy.createdAt,children:time(r.created_at)},
       {key:'packed',label:copy.packedAt,children:time(r.packed_at)},
       {key:'shipped',label:copy.shippedAt,children:time(r.shipped_at)},
       {key:'delivered',label:copy.deliveredAt,children:time(r.delivered_at)},
       {key:'canceled',label:copy.paymentCanceledAt,children:time(r.canceled_at)},
      ]}/>
      <Table rowKey="id" size="small" pagination={false} dataSource={r.items} scroll={{x:900}} columns={[
       {title:copy.productTitle,dataIndex:'title'},
       {title:'SKU',dataIndex:'sku',render:text},
       {title:copy.barcode,dataIndex:'barcode',render:text},
       {title:copy.fulfilledQuantity,dataIndex:'quantity',render:(v:string)=><span className="price-rule-value">{amountText(v)}</span>},
       {title:copy.fulfillmentOrderLine,dataIndex:'line_item_id',render:text},
       {title:copy.inventoryItemID,dataIndex:'inventory_item_id',render:text},
      ]}/>
      <Typography.Text strong>{copy.fulfillmentLabels}</Typography.Text>
      <Table rowKey="id" size="small" pagination={false} dataSource={r.labels} scroll={{x:600}} columns={[
       {title:copy.trackingNumber,dataIndex:'tracking_number',render:text},
       {title:copy.trackingURL,dataIndex:'tracking_url',render:(v:string)=>evidenceLink(v,copy.openTracking)},
       {title:copy.labelURL,dataIndex:'label_url',render:(v:string)=>evidenceLink(v,copy.openLabel)},
      ]}/>
      <Typography.Text strong>{copy.deliveryAddress}</Typography.Text>
      {a?<Descriptions column={{xs:1,sm:2,lg:3}} items={[
       {key:'name',label:copy.recipient,children:a.first_name===null&&a.last_name===null?'—':[a.first_name,a.last_name].filter(v=>v!==null).join(' ')},
       {key:'company',label:copy.company,children:text(a.company)},
       {key:'address1',label:copy.addressLine1,children:text(a.address_1)},
       {key:'address2',label:copy.addressLine2,children:text(a.address_2)},
       {key:'city',label:copy.city,children:text(a.city)},
       {key:'province',label:copy.province,children:text(a.province)},
       {key:'postal',label:copy.postalCode,children:text(a.postal_code)},
       {key:'country',label:copy.country,children:text(a.country_code)},
       {key:'phone',label:copy.phone,children:text(a.phone)},
      ]}/>:r.delivery_address_id!==null?<Alert type="warning" showIcon message={copy.fulfillmentAddressMissing} description={text(r.delivery_address_id)}/>:<Typography.Text type="secondary">{copy.noDeliveryAddress}</Typography.Text>}
     </>}
    </Space>
   })}
  </>}
 </Space>
 {operating&&access.can('order.allocation.write','order.payment.read','device.read')&&<OrderParcel orderID={id} fulfillmentID={operating} copy={copy} onExpired={onExpired} onClose={()=>{setOperating(undefined);setAttempt(n=>n+1);onChanged?.()}}/>}
 </section>
}
