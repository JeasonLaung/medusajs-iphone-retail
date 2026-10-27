import { useEffect, useState } from 'react'
import { Alert, Button, Empty, Form, Input, Select, Space, Table, Tag, Typography } from 'antd'
import { useSearchParams } from 'react-router-dom'
import { APIError, listProducts } from './api'
import type { ProductList, ProductSummary } from './api'
import type { Copy } from './i18n'

type Filters = { q?: string; status?: string; sort?: string }
export default function Products({ copy, onExpired }: { copy:Copy; onExpired():void }) {
  const [params,setParams]=useSearchParams()
  const key=params.toString()
  const [form]=Form.useForm<Filters>()
  const [state,setState]=useState<{loading:boolean;data?:ProductList;error?:unknown}>({loading:true})
  const [attempt,setAttempt]=useState(0)
  const statuses: Record<string,string>={draft:copy.draft,proposed:copy.proposed,published:copy.published,rejected:copy.rejected}
  useEffect(()=>{
    form.setFieldsValue({q:params.get('q')??'',status:params.get('status')??undefined,sort:params.get('sort')??'created_at_desc'})
  },[key,form,params])
  useEffect(()=>{
    const controller=new AbortController()
    setState({loading:true})
    listProducts(new URLSearchParams(key),controller.signal).then(data=>{
      if(!controller.signal.aborted)setState({loading:false,data})
    }).catch(error=>{
      if(controller.signal.aborted)return
      if(error instanceof APIError && error.status===401)onExpired()
      else setState({loading:false,error})
    })
    return()=>controller.abort()
  },[key,attempt,onExpired])
  const apply=(values:Filters)=>{
    const next=new URLSearchParams()
    if(values.q?.trim())next.set('q',values.q.trim())
    if(values.status)next.set('status',values.status)
    if(values.sort && values.sort!=='created_at_desc')next.set('sort',values.sort)
    next.set('limit',params.get('limit')??'20')
    setParams(next)
  }
  return <section className="account-panel" aria-label={copy.products}>
    <Form form={form} layout="inline" onFinish={apply} className="product-filters">
      <Form.Item name="q" label={copy.search}><Input maxLength={200} allowClear /></Form.Item>
      <Form.Item name="status" label={copy.status}><Select allowClear style={{minWidth:120}} options={Object.entries(statuses).map(([value,label])=>({value,label}))} /></Form.Item>
      <Form.Item name="sort" label={copy.sort}><Select style={{minWidth:170}} options={[{value:'created_at_desc',label:copy.newest},{value:'created_at_asc',label:copy.oldest},{value:'title_asc',label:copy.titleAsc},{value:'title_desc',label:copy.titleDesc}]} /></Form.Item>
      <Form.Item><Space><Button aria-label={copy.search} htmlType="submit" type="primary">{copy.search}</Button><Button aria-label={copy.reset} onClick={()=>setParams(new URLSearchParams())}>{copy.reset}</Button></Space></Form.Item>
    </Form>
    {state.error!==undefined ? <Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError && state.error.status===400?copy.invalidFilters:copy.unavailable} /><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space> :
      <Table<ProductSummary> rowKey="id" loading={state.loading} dataSource={state.data?.products??[]} scroll={{x:720}} locale={{emptyText:<Empty description={copy.noProducts} />}} columns={[
        {title:copy.productTitle,dataIndex:'title',render:(title:string,row)=><Space direction="vertical" size={0}><Typography.Text strong>{title}</Typography.Text><Typography.Text type="secondary">{row.handle}</Typography.Text></Space>},
        {title:copy.status,dataIndex:'status',render:(status:string)=><Tag color={status==='published'?'success':status==='rejected'?'error':undefined}>{statuses[status]}</Tag>},
        {title:copy.variants,dataIndex:'variant_count'},
        {title:copy.createdAt,dataIndex:'created_at',render:(value:string)=><time dateTime={value}>{new Date(value).toLocaleString(document.documentElement.lang)}</time>},
      ]} pagination={{current:state.data?Math.floor(state.data.offset/state.data.limit)+1:1,pageSize:state.data?.limit??20,total:state.data?.count??0,showSizeChanger:true,pageSizeOptions:[20,50,100],onChange:(page,size)=>{
        const next=new URLSearchParams(key);next.set('limit',String(size));next.set('offset',String((page-1)*size));setParams(next)
      }}} />}
  </section>
}
