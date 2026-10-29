import {useEffect,useState} from 'react'
import {Alert,Button,Radio,Result,Space,Spin,Table,Typography} from 'antd'
import {APIError,getWarehouseAccess,getWarehouseInventory,editWarehouseStock} from './api'
import type {WarehouseAccess,WarehouseInventoryPage} from './api'
import type {Copy} from './i18n'
import StockEdit from './StockEdit'
import type {StockEditLevel} from './StockEdit'
import {amountText} from './VariantPrices'

export default function WarehouseInventory({copy,onExpired}:{copy:Copy;onExpired():void}){
 const [attempt,setAttempt]=useState(0),[access,setAccess]=useState<WarehouseAccess>(),[location,setLocation]=useState(''),[offset,setOffset]=useState(0),[data,setData]=useState<WarehouseInventoryPage>(),[loading,setLoading]=useState(true),[error,setError]=useState<unknown>(),[editing,setEditing]=useState<StockEditLevel>()
 useEffect(()=>{
  const abort=new AbortController();setLoading(true);setError(undefined);setData(undefined);setEditing(undefined)
  const run=async()=>{
   try{
    const current=await getWarehouseAccess(abort.signal);if(abort.signal.aborted)return;setAccess(current)
    if(!current.actions.includes('inventory.read'))return
    const id=current.locations.some(l=>l.id===location)?location:current.locations[0]?.id??''
    if(id!==location){setLocation(id);setOffset(0);return}
    if(id){const page=await getWarehouseInventory(id,offset,abort.signal);if(!abort.signal.aborted)setData(page)}
   }catch(error){if(abort.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setError(error)}
   finally{if(!abort.signal.aborted)setLoading(false)}
  };void run();return()=>abort.abort()
 },[attempt,location,offset,onExpired])
 const reload=()=>setAttempt(n=>n+1)
 if(loading)return <Spin/>
 if(error)return <Space direction="vertical">{error instanceof APIError&&error.status===403?<Result status="403" title={copy.accessDenied}/>:<Alert type="error" showIcon message={copy.unavailable}/>}<Button onClick={reload}>{copy.retry}</Button></Space>
 if(!access?.actions.includes('inventory.read'))return <Result status="403" title={copy.accessDenied}/>
 const name=access.locations.find(l=>l.id===location)?.name
 return <section className="account-panel"><Space direction="vertical" size={24} className="full-width">
  <Typography.Paragraph type="secondary">{copy.scopedInventoryInfo}</Typography.Paragraph>
  <Space wrap><Button onClick={reload}>{copy.reload}</Button><Radio.Group aria-label={copy.stockLocation} value={location} onChange={e=>{setLocation(e.target.value);setOffset(0)}} options={access.locations.map(l=>({value:l.id,label:l.name}))}/></Space>
  {!access.locations.length?<Alert type="info" showIcon message={copy.noWarehouseScope}/>:data&&<>
   <Table rowKey="id" dataSource={data.levels} pagination={false} scroll={{x:900}} locale={{emptyText:copy.noInventoryLevels}} columns={[
    {title:'SKU',dataIndex:'sku',render:(v:string|null)=>v??'—'},
    {title:copy.name,dataIndex:'title',render:(v:string|null)=>v??'—'},
    {title:copy.inventoryItemID,dataIndex:'inventory_item_id'},
    {title:copy.stockedQuantity,dataIndex:'stocked_quantity',render:amountText},
    {title:copy.reservedQuantity,dataIndex:'reserved_quantity',render:amountText},
    {title:copy.incomingQuantity,dataIndex:'incoming_quantity',render:amountText},
    {title:copy.levelBalance,dataIndex:'available_quantity',render:amountText},
    {title:copy.editStock,render:(_,row)=>data.writes_enabled&&access.actions.includes('inventory.stock.write')?<Button onClick={()=>setEditing({...row,location_name:name??null})}>{copy.editStock}</Button>:'—'},
   ]}/>
   <Space><Button disabled={offset===0} onClick={()=>setOffset(n=>Math.max(0,n-50))}>{copy.previousWarehousePage}</Button><Typography.Text>{offset+1}–{offset+data.levels.length}</Typography.Text><Button disabled={!data.has_more} onClick={()=>setOffset(n=>n+50)}>{copy.nextWarehousePage}</Button></Space>
  </>}
 </Space>{editing&&<StockEdit level={editing} copy={copy} save={(id,body)=>editWarehouseStock(location,id,body)} onExpired={onExpired} onClose={()=>{setEditing(undefined);reload()}} onSaved={()=>{setEditing(undefined);reload()}}/>}</section>
}
