import {useAccess} from './Permissions'
import { useEffect, useState } from 'react'
import { Alert, Button, Collapse, Descriptions, Empty, Space, Spin, Table, Tag, Typography } from 'antd'
import { Drawer } from 'antd'
import { APIError, variantInventory } from './api'
import type { InventoryLevel, VariantDetail, VariantInventory as Inventory } from './api'
import { amountText } from './VariantPrices'
import type { Copy } from './i18n'
import StockEdit from './StockEdit'

export default function VariantInventory({productID,variant,copy,onClose,onExpired}:{productID:string;variant:VariantDetail;copy:Copy;onClose():void;onExpired():void}){
  const access=useAccess()
  const [attempt,setAttempt]=useState(0)
  const [editing,setEditing]=useState<InventoryLevel>()
  const refreshed=()=>{setEditing(undefined);setAttempt(n=>n+1)}
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
  const d=state.data
  return <Drawer open title={copy.inventoryDetails} width="min(1100px, 100vw)" onClose={()=>{if(!editing)onClose()}} maskClosable={!editing} keyboard={!editing} closable={!editing} destroyOnHidden extra={<Button aria-label={copy.cancel} disabled={!!editing} onClick={onClose}>{copy.cancel}</Button>}>
    <Space direction="vertical" size={24} className="full-width">
      <Typography.Text strong>{variant.title}</Typography.Text>
      <Alert type="info" showIcon message={copy.inventoryReadOnly}/>
      {state.loading?<Spin aria-label={copy.loadingInventory}/>:state.error!==undefined?<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError&&state.error.status===404?copy.variantNotFound:copy.unavailable}/><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>:d&&<>
        <Descriptions column={1} items={[{key:'manage',label:copy.manageInventory,children:d.manage_inventory?copy.yes:copy.no},{key:'backorder',label:copy.allowBackorder,children:d.allow_backorder?copy.yes:copy.no}]}/>
        {!d.items.length?<Empty description={copy.noInventoryItems}/>:<Collapse ghost items={d.items.map(i=>({key:i.inventory_item_id,label:<Space wrap><Typography.Text>{i.title??i.sku??i.inventory_item_id}</Typography.Text>{(!i.item_available||i.migration_holds.length>0||i.unmatched_reservations.length>0||i.levels.some(l=>!l.reconciled||l.location_name===null))&&<Tag color="warning">{copy.needsReconciliation}</Tag>}</Space>,children:<Space direction="vertical" size={24} className="full-width">
          <Descriptions column={1} items={[{key:'id',label:copy.inventoryItemID,children:i.inventory_item_id},{key:'sku',label:'SKU',children:i.sku??'—'},{key:'required',label:copy.requiredQuantity,children:amountText(i.required_quantity)}]}/>
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
    {access.can('inventory.stock.write')&&editing&&<StockEdit level={editing} copy={copy} onClose={refreshed} onSaved={refreshed} onExpired={onExpired}/>}
  </Drawer>
}
