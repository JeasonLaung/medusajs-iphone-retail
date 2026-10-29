import {useEffect,useState} from 'react'
import {Alert,Button,Drawer,Input,Space,Spin,Table,Typography} from 'antd'
import {APIError,warehouseIntakeVariants} from './api'
import type {WarehouseIntakeScope,WarehouseIntakeVariant} from './api'
import type {Copy} from './i18n'
import DeviceIntake from './DeviceIntake'

export default function WarehouseIntakePicker({scope,copy,onExpired,onClose}:{scope:WarehouseIntakeScope;copy:Copy;onExpired():void;onClose():void}){
 const [q,setQ]=useState(''),[offset,setOffset]=useState(0),[attempt,setAttempt]=useState(0),[loading,setLoading]=useState(true),[error,setError]=useState<unknown>(),[data,setData]=useState<Awaited<ReturnType<typeof warehouseIntakeVariants>>>(),[selected,setSelected]=useState<WarehouseIntakeVariant>()
 useEffect(()=>{const abort=new AbortController();setLoading(true);setError(undefined);setData(undefined);warehouseIntakeVariants(scope,q,offset,abort.signal).then(data=>{if(!abort.signal.aborted)setData(data)}).catch(error=>{if(abort.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setError(error)}).finally(()=>{if(!abort.signal.aborted)setLoading(false)});return()=>abort.abort()},[q,offset,attempt,scope.locationID,scope.category,onExpired])
 return <Drawer open title={copy.deviceIntake} width={760} onClose={()=>{if(!selected)onClose()}} closable={!selected} maskClosable={!selected} keyboard={!selected}>
  <Space direction="vertical" size={24} className="full-width">
   <Typography.Paragraph>{copy.warehouseIntakeInfo}</Typography.Paragraph>
   <Input.Search aria-label={copy.warehouseIntakeSearch} placeholder={copy.warehouseIntakeSearch} maxLength={200} allowClear onSearch={value=>{setQ(value.trim());setOffset(0)}}/>
   {loading?<Spin/>:error?<><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></>:data&&<>
    <Table<WarehouseIntakeVariant> rowKey="id" dataSource={data.variants} pagination={false} scroll={{x:600}} columns={[{title:copy.products,dataIndex:'product_title'},{title:'SKU',dataIndex:'sku'},{title:copy.name,dataIndex:'title'},{title:copy.deviceIntake,render:(_,row)=><Button onClick={()=>setSelected(row)}>{copy.deviceIntake}</Button>}]}/>
    <Space><Button disabled={!offset} onClick={()=>setOffset(n=>Math.max(0,n-20))}>{copy.previousWarehousePage}</Button><Typography.Text>{offset+1}–{offset+data.variants.length}</Typography.Text><Button disabled={!data.has_more} onClick={()=>setOffset(n=>n+20)}>{copy.nextWarehousePage}</Button></Space>
   </>}
  </Space>
  {selected&&<DeviceIntake productID={selected.product_id} variantID={selected.id} scope={scope} copy={copy} onExpired={onExpired} onClose={()=>{setSelected(undefined);onClose()}}/>}
 </Drawer>
}
