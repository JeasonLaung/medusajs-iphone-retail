import tr from "./translate"
import { App as AntdApp } from "antd"
import { useEffect, useState } from 'react'
import { ArrowLeftOutlined, PlusOutlined } from '@ant-design/icons'
import { Button, Form, Input, InputNumber, Modal, Popconfirm, Space, Switch, Tag, Tooltip, Typography } from 'antd'
import { Table } from './ResizableTable'
import type { ColumnsType } from 'antd/es/table'
import { deleteCrawlerSource, listCrawlerSources, upsertCrawlerSource, type SEOCrawlerSource } from './api'

interface Props {
  open: boolean
  onClose: () => void
  onSaved?: () => void
}

type SourceForm = Pick<SEOCrawlerSource, 'source_name' | 'file_path' | 'interval_minutes' | 'enabled' | 'collect_ai_crawlers' | 'collect_traffic'>

export default function NginxLogSourceDialog({ open, onClose, onSaved }: Props) {
  const { message } = AntdApp.useApp()
  const [sources, setSources] = useState<SEOCrawlerSource[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState<SEOCrawlerSource | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form] = Form.useForm<SourceForm>()

  async function load() {
    setLoading(true)
    try {
      setSources(await listCrawlerSources())
    } catch (error) {
      message.error((error as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!open) return
    setShowForm(false)
    setEditing(null)
    void load()
  }, [open])

  function edit(source?: SEOCrawlerSource) {
    setEditing(source || null)
    form.resetFields()
    form.setFieldsValue(source || {
      source_name: '', file_path: '/var/log/nginx/access.log', interval_minutes: 10,
      enabled: true, collect_ai_crawlers: true, collect_traffic: true,
    })
    setShowForm(true)
  }

  async function save() {
    const values = await form.validateFields()
    if (!values.collect_ai_crawlers && !values.collect_traffic) {
      message.error(tr("至少启用一种识别类型"))
      return
    }
    setSaving(true)
    try {
      await upsertCrawlerSource(editing?.id || null, { ...values, source_type: 'nginx_file' })
      message.success(tr("Nginx 日志源已保存"))
      setShowForm(false)
      await load()
      onSaved?.()
    } catch (error) {
      message.error((error as Error).message)
    } finally {
      setSaving(false)
    }
  }

  async function remove(source: SEOCrawlerSource) {
    try {
      await deleteCrawlerSource(source.id)
      message.success(tr("Nginx 日志源已删除"))
      await load()
      onSaved?.()
    } catch (error) {
      message.error((error as Error).message)
    }
  }

  const columns: ColumnsType<SEOCrawlerSource> = [
    { title: tr("名称"), dataIndex: 'source_name', width: 150 },
    { title: tr("日志路径"), dataIndex: 'file_path', width: 260, ellipsis: true },
    { title: tr("间隔"), dataIndex: 'interval_minutes', width: 76, align: 'right', render: (value: number) => tr(`${value} 分`) },
    { title: tr("识别范围"), width: 170, render: (_, row) => <Space size={[4, 4]} wrap>{row.collect_ai_crawlers && <Tag color="blue">{tr("AI 爬虫")}</Tag>}{row.collect_traffic && <Tag color="purple">{tr("渠道归因")}</Tag>}</Space> },
    { title: tr("已读行"), dataIndex: 'read_lines', width: 90, align: 'right' },
    { title: tr("AI 命中"), dataIndex: 'matched_lines', width: 90, align: 'right' },
    { title: tr("渠道命中"), dataIndex: 'traffic_matched_lines', width: 90, align: 'right' },
    { title: tr("最近扫描"), dataIndex: 'last_scanned_at', width: 170, render: (value: number) => value ? new Date(value * 1000).toLocaleString() : tr("尚未扫描") },
    {
      title: tr("状态"), width: 100, render: (_, row) => !row.enabled
        ? <Tag>{tr("已停用")}</Tag>
        : row.last_error
          ? <Tooltip title={row.last_error}><Tag color="red">{tr("读取失败")}</Tag></Tooltip>
          : <Tag color="green">{tr("正常")}</Tag>,
    },
    {
      title: tr("操作"), fixed: 'right', width: 136, render: (_, row) => <Space size="small">
        <Button size="small" onClick={() => edit(row)}>{tr("编辑")}</Button>
        <Popconfirm title={tr(`删除 ${row.source_name}？`)} description={tr("统计记录不会随数据源一起删除。")} onConfirm={() => void remove(row)}>
          <Button size="small" danger>{tr("删除")}</Button>
        </Popconfirm>
      </Space>,
    },
  ]

  return <Modal rootClassName="seo-reference"
    open={open}
    width={1100}
    centered
    destroyOnClose
    onCancel={onClose}
    footer={null}
    title={<div><Typography.Title level={4} style={{ margin: 0 }}>{tr("Nginx 日志源")}</Typography.Title><Typography.Text type="secondary">{tr("每个日志文件只增量读取一次，同时产出 AI 爬虫和渠道归因数据。")}</Typography.Text></div>}
  >
    {showForm ? <>
      <Button type="link" icon={<ArrowLeftOutlined />} style={{ paddingInline: 0, marginBottom: 12 }} onClick={() => setShowForm(false)}>{tr("返回数据源列表")}</Button>
      <Form form={form} layout="vertical">
        <Form.Item name="source_name" label={tr("名称")} rules={[{ required: true, message: tr("请输入数据源名称") }]}><Input placeholder="www.eurorefurb.com" /></Form.Item>
        <Form.Item name="file_path" label={tr("服务器日志文件路径")} rules={[{ required: true, message: tr("请输入日志文件路径") }]}><Input placeholder="/var/log/nginx/access.log" /></Form.Item>
        <Form.Item name="interval_minutes" label={tr("扫描间隔（分钟）")} rules={[{ required: true }]}><InputNumber min={1} max={1440} style={{ width: '100%' }} /></Form.Item>
        <Space size="large" wrap>
          <Form.Item name="collect_ai_crawlers" label={tr("AI 爬虫识别")} valuePropName="checked"><Switch /></Form.Item>
          <Form.Item name="collect_traffic" label={tr("渠道归因识别")} valuePropName="checked"><Switch /></Form.Item>
          <Form.Item name="enabled" label={tr("启用日志源")} valuePropName="checked"><Switch /></Form.Item>
        </Space>
        <div><Button onClick={() => setShowForm(false)}>{tr("取消")}</Button> <Button type="primary" loading={saving} onClick={() => void save()}>{tr("保存")}</Button></div>
      </Form>
    </> : <>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}><Button type="primary" icon={<PlusOutlined />} onClick={() => edit()}>{tr("新增日志源")}</Button></div>
      <Table rowKey="id" loading={loading} columns={columns} dataSource={sources} pagination={false} size="small" scroll={{ x: 1350 }} />
    </>}
  </Modal>
}
