import tr from "./translate"
import { App as AntdApp } from "antd"
// Provider 配置：ChatGPT / DeepSeek API key
import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Tag, Typography } from 'antd'
import { Table } from './ResizableTable'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  deleteProvider,
  listProviders,
  upsertProvider,
  type SEOProviderConfig,
} from './api'

interface FormValues {
  provider?: string
  api_key?: string
  base_url?: string
  model?: string
  wire_api?: string
}

export default function ProvidersPage() {
  const { message } = AntdApp.useApp()
  const [rows, setRows] = useState<SEOProviderConfig[]>([])
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<SEOProviderConfig | null>(null)
  const [form] = Form.useForm<FormValues>()

  async function load() {
    setLoading(true)
    try {
      setRows(await listProviders())
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  function openCreate() {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ provider: 'chatgpt', base_url: 'https://api.openai.com/v1', model: 'gpt-4o-mini', wire_api: 'chat' })
    setOpen(true)
  }

  function openEdit(row: SEOProviderConfig) {
    setEditing(row)
    form.setFieldsValue({
      ...row,
      wire_api: row.wire_api,
      api_key: '', // 留空表示不修改
    })
    setOpen(true)
  }

  async function submit() {
    const v = await form.validateFields()
    try {
      await upsertProvider(editing?.id || null, v)
      message.success(tr("已保存"))
      setOpen(false)
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  async function remove(id: string) {
    try {
      await deleteProvider(id)
      message.success(tr("已删除"))
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const columns: ColumnsType<SEOProviderConfig> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: 'Provider',
      dataIndex: 'provider',
      width: 140,
      render: (p: string) => <Tag color="blue">{p}</Tag>,
    },
    { title: 'Base URL', dataIndex: 'base_url' },
    { title: 'Model', dataIndex: 'model', width: 160 },
    {
      title: 'Wire API',
      dataIndex: 'wire_api',
      width: 120,
      render: (w: string) => <Tag>{w}</Tag>,
    },
    {
      title: 'API Key',
      dataIndex: 'api_key_configured',
      width: 130,
      render: (configured: boolean) => (
        <Tag color={configured ? 'green' : 'default'}>{configured ? tr("已配置") : tr("未设置")}</Tag>
      ),
    },
    {
      title: tr("操作"),
      fixed: 'right',
      width: 160,
      render: (_, row) => (
        <Space>
          <Button size="small" onClick={() => openEdit(row)}>{tr("编辑")}</Button>
          <Popconfirm title={tr(`删除 ${row.provider}?`)} onConfirm={() => remove(row.id)}>
            <Button size="small" danger>{tr("删除")}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Typography.Title level={4} style={{ margin: 0 }}>{tr("AI Provider 配置")}</Typography.Title>
      <Card
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{tr("新增 Provider")}</Button>
        }
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={rows}
          pagination={false}
          scroll={{ x: 950 }}
        />
      </Card>
      <Modal rootClassName="seo-reference"
        title={editing ? tr(`编辑 ${editing.provider}`) : tr("新增 Provider")}
        open={open}
        onOk={submit}
        onCancel={() => setOpen(false)}
        okText={tr("保存")}
        cancelText={tr("取消")}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="provider" label="Provider" rules={[{ required: true }]}>
            <Select
              disabled={!!editing}
              onChange={(provider) => form.setFieldsValue(provider === 'deepseek'
                ? { base_url: 'https://api.deepseek.com', model: 'deepseek-chat', wire_api: 'chat' }
                : { base_url: 'https://api.openai.com/v1', model: 'gpt-4o-mini', wire_api: 'chat' })}
              options={[
                { label: 'ChatGPT', value: 'chatgpt' },
                { label: 'DeepSeek', value: 'deepseek' },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="api_key"
            label="API Key"
            extra={editing?.api_key_configured ? tr("已加密保存；留空表示保持当前密钥") : tr("将加密保存")}
            rules={[{ validator: (_, value) => value || editing?.api_key_configured ? Promise.resolve() : Promise.reject(new Error(tr("请输入 API Key"))) }]}
          >
            <Input.Password placeholder="sk-..." />
          </Form.Item>
          <Form.Item name="base_url" label="Base URL" rules={[{ required: true }]}>
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item name="model" label="Model" rules={[{ required: true }]}>
            <Input placeholder="gpt-4o-mini" />
          </Form.Item>
          <Form.Item name="wire_api" label="Wire API">
            <Select
              options={[
                { label: 'Responses', value: 'responses' },
                { label: 'Chat Completions', value: 'chat' },
                { label: 'Chat input (compatible gateway)', value: 'chat_input' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  )
}
