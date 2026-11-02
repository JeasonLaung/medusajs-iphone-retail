import {useAccess} from './Permissions'
import { useEffect, useState } from 'react'
import { Alert, Button, Collapse, Descriptions, Empty, Space, Spin, Table, Tag, Typography } from 'antd'
import { Drawer } from 'antd'
import { APIError, variantInventory, readInventoryLinks } from './api'
import type { InventoryLevel, VariantDetail, VariantInventory as Inventory } from './api'
import { amountText } from './VariantPrices'
import type { Copy } from './i18n'
import StockEdit from './StockEdit'
import StockCreate from './StockCreate'
import VariantInventoryItems from './VariantInventoryItems'

export default function VariantInventory({productID,variant,copy,onClose,onExpired}:{productID:string;variant:VariantDetail;copy:Copy;onClose():void;onExpired():void}){
  const access=useAccess()
  const [managing,setManaging]=useState(false)
  const [linksEnabled,setLinksEnabled]=useState(false)
  const canManage=access.can('product.read','product.write','inventory.read')
  const [attempt,setAttempt]=useState(0)
  const [editing,setEditing]=useState<InventoryLevel>()
  const [creating,setCreating]=useState<string>()
  const refreshed=()=>{setEditing(undefined);setCreating(undefined);setManaging(false);setAttempt(n=>n+1)}
  const [state,setState]=useState<{loading:boolean;data?:Inventory;error?:unknown}>({loading:true})
  useEffect(()=>{
    const controller=new AbortController();setState({loading:true})
    variantInventory(productID,variant.id,controller.signal).then(data=>{if(!controller.signal.aborted)setState({loading:false,data})}).catch(error=>{
      if(controller.signal.aborted)return
      if(error instanceof APIError&&error.status===401)onExpired()
      else setState({loading:false,error})
    })
    return()=>controller.abort()
  },[productID,variant.id,attempt,onExpired])
  useEffect(()=>{const c=new AbortController();setLinksEnabled(false);if(canManage)readInventoryLinks(productID,variant.id,c.signal).then(d=>{if(!c.signal.aborted)setLinksEnabled(d.enabled)}).catch(e=>{if(!c.signal.aborted&&e instanceof APIError&&e.status===401)onExpired()});return()=>c.abort()},[productID,variant.id,attempt,onExpired,canManage])
  const d=state.data
  return <Drawer open title={copy.inventoryDetails} width="min(1100px, 100vw)" onClose={()=>{if(!editing&&!creating&&!managing)onClose()}} maskClosable={!editing&&!creating&&!managing} keyboard={!editing&&!creating&&!managing} closable={!editing&&!creating&&!managing} destroyOnHidden extra={<Button aria-label={copy.cancel} disabled={!!editing||!!creating||managing} onClick={onClose}>{copy.cancel}</Button>}>
    <Space direction="vertical" size={24} className="full-width">
      <Typography.Text strong>{variant.title}</Typography.Text>
      {linksEnabled?<Button onClick={()=>setManaging(true)}>{copy.language==='语言'?'管理库存关联':'Manage inventory links'}</Button>:<Alert type="info" showIcon message={copy.inventoryReadOnly}/>}
      {state.loading?<Spin aria-label={copy.loadingInventory}/>:state.error!==undefined?<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError&&state.error.status===404?copy.variantNotFound:copy.unavailable}/><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>:d&&<>
        <Descriptions column={1} items={[{key:'manage',label:copy.manageInventory,children:d.manage_inventory?copy.yes:copy.no},{key:'backorder',label:copy.allowBackorder,children:d.allow_backorder?copy.yes:copy.no}]}/>
        {!d.items.length?<Empty description={copy.noInventoryItems}/>:<Collapse ghost items={d.items.map(i=>({key:i.inventory_item_id,label:<Space wrap><Typography.Text>{i.title??i.sku??i.inventory_item_id}</Typography.Text>{(!i.item_available||i.migration_holds.length>0||i.unmatched_reservations.length>0||i.levels.some(l=>!l.reconciled||l.location_name===null))&&<Tag color="warning">{copy.needsReconciliation}</Tag>}</Space>,children:<Space direction="vertical" size={24} className="full-width">
          <Descriptions column={1} items={[{key:'id',label:copy.inventoryItemID,children:i.inventory_item_id},{key:'sku',label:'SKU',children:i.sku??'—'},{key:'required',label:copy.requiredQuantity,children:amountText(i.required_quantity)}]}/>
          {i.item_available&&access.can('inventory.stock.write','warehouse.read')&&<Button onClick={()=>setCreating(i.inventory_item_id)}>{copy.createStockLevel}</Button>}
          {!i.item_available&&<Alert type="warning" showIcon message={copy.inventoryItemUnavailable}/>}
          {i.migration_holds.length>0&&<><Alert type="warning" showIcon message={copy.migrationHolds}/><Table rowKey={h=>`${h.location_id}:${h.reason}`} dataSource={i.migration_holds} pagination={false} scroll={{x:550}} columns={[{title:copy.stockLocation,dataIndex:"location_id"},{title:copy.reconciliation,dataIndex:"reason",render:(reason:Inventory['items'][number]['migration_holds'][number]['reason'])=>copy[reason]}]}/></>}
          <Table<InventoryLevel> rowKey="id" dataSource={i.levels} pagination={false} scroll={{x:1850}} locale={{emptyText:<Empty description={copy.noInventoryLevels}/>}} columns={[
            {title:copy.stockLocation,dataIndex:'location_name',width:220,render:(name:string|null,l)=><Space direction="vertical"><Typography.Text>{name??copy.locationUnavailable}</Typography.Text><Typography.Text type="secondary">{l.location_id}</Typography.Text></Space>},
            {title:copy.stockedQuantity,dataIndex:'stocked_quantity',render:amountText},
            {title:copy.reservedQuantity,dataIndex:'reserved_quantity',render:amountText},
            {title:copy.levelBalance,dataIndex:'available_quantity',render:amountText},
            {title:copy.incomingQuantity,dataIndex:'incoming_quantity',render:amountText},
            {title:copy.reservationTotal,dataIndex:'reservation_quantity',render:amountText},
            {title:copy.reservationCount,dataIndex:'reservation_count'},
            {title:copy.editStock,key:'edit',render:(_,l)=>access.can('inventory.stock.write')&&d.stock_edit_enabled&&i.item_available&&l.location_name!==null?<Button aria-label={copy.editStock+' '+l.location_id} onClick={()=>setEditing(l)}>{copy.editStock}</Button>:null},
            {title:copy.reconciliation,dataIndex:'reconciled',render:(ok:boolean)=>ok?<Tag color="success">{copy.reconciled}</Tag>:<Tag color="warning">{copy.needsReconciliation}</Tag>},
          ]}/>
          {i.unmatched_reservations.length>0&&<><Alert type="warning" showIcon message={copy.unmatchedReservations}/><Table rowKey="location_id" dataSource={i.unmatched_reservations} pagination={false} scroll={{x:550}} columns={[{title:copy.stockLocation,dataIndex:'location_id'},{title:copy.reservationTotal,dataIndex:'quantity',render:amountText},{title:copy.reservationCount,dataIndex:'count'}]}/></>}
        </Space>}))}/>}
      </>}
    </Space>
    {canManage&&managing&&<VariantInventoryItems productID={productID} variantID={variant.id} copy={copy} onClose={()=>setManaging(false)} onSaved={refreshed} onExpired={onExpired}/>}
    {access.can('inventory.stock.write','warehouse.read')&&creating&&<StockCreate productID={productID} variantID={variant.id} itemID={creating} copy={copy} onClose={refreshed} onExpired={onExpired}/>}
    {access.can('inventory.stock.write')&&editing&&<StockEdit level={editing} copy={copy} onClose={refreshed} onSaved={refreshed} onExpired={onExpired}/>}
  </Drawer>
}
