import tr from "./translate"
import { App as AntdApp } from "antd"
// SEO/GEO 设置：站点 URL / GA4 / GSC / IndexNow 凭据
import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, Modal, Popconfirm, Space, Switch, Tag, Typography } from 'antd'
import { Table } from './ResizableTable'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { deleteSetting, listSettings, upsertSetting, type SEOSetting } from './api'

interface FormValues {
  key: string
  value: string
  group: string
  is_secret: boolean
}

const groupLabel: Record<string, string> = {
  ga4: 'GA4',
  gsc: 'Search Console',
  indexnow: 'IndexNow',
  bing: 'Bing Webmaster Tools',
}

export default function SEOSettingsPage() {
  const { message } = AntdApp.useApp()
  const [rows, setRows] = useState<SEOSetting[]>([])
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<SEOSetting | null>(null)
  const [form] = Form.useForm<FormValues>()

  async function load() {
    setLoading(true)
    try {
      const list = await listSettings()
      setRows(list.filter((item) => item.group !== 'site'))
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
    form.setFieldsValue({ group: 'ga4', is_secret: false })
    setOpen(true)
  }

  function openEdit(row: SEOSetting) {
    setEditing(row)
    form.setFieldsValue({
      key: row.key,
      value: row.value === '********' ? '' : row.value,
      group: row.group,
      is_secret: row.is_secret,
    })
    setOpen(true)
  }

  async function submit() {
    const v = await form.validateFields()
    try {
      await upsertSetting(v.key, {
        value: v.value,
        is_secret: v.is_secret,
        group: v.group,
      })
      message.success(tr("已保存"))
      setOpen(false)
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  async function remove(key: string) {
    try {
      await deleteSetting(key)
      message.success(tr("已删除"))
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const columns: ColumnsType<SEOSetting> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: 'Key', dataIndex: 'key', width: 240 },
    {
      title: tr("分组"),
      dataIndex: 'group',
      width: 130,
      render: (g: string) => <Tag color="blue">{groupLabel[g] || g}</Tag>,
    },
    {
      title: tr("值"),
      dataIndex: 'value',
      render: (v: string) => <Typography.Text code>{v || '(secret)'}</Typography.Text>,
    },
    {
      title: tr("敏感"),
      dataIndex: 'is_secret',
      width: 80,
      render: (b: boolean) => (b ? <Tag color="orange">{tr("是")}</Tag> : '-'),
    },
    {
      title: tr("操作"),
      fixed: 'right',
      width: 160,
      render: (_, row) => (
        <Space>
          <Button size="small" onClick={() => openEdit(row)}>{tr("编辑")}</Button>
          <Popconfirm
            title={tr(`删除 ${row.key}?`)}
            onConfirm={() => remove(row.key)}
            okText={tr("删除")}
            cancelText={tr("取消")}
          >
            <Button size="small" danger>{tr("删除")}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Typography.Title level={4} style={{ margin: 0 }}>{tr("SEO/GEO 配置")}</Typography.Title>
      <Card
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{tr("新增配置")}</Button>
        }
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={rows}
          pagination={false}
          scroll={{ x: 900 }}
        />
      </Card>
      <Modal rootClassName="seo-reference"
        title={editing ? tr(`编辑 ${editing.key}`) : tr("新增配置")}
        open={open}
        onOk={submit}
        onCancel={() => setOpen(false)}
        okText={tr("保存")}
        cancelText={tr("取消")}
      >
        <Form form={form} layout="vertical" autoComplete="off">
          <Form.Item name="key" label="Key" rules={[{ required: true }]}>
            <Input placeholder="seo.default_title" disabled={!!editing} />
          </Form.Item>
          <Form.Item name="value" label="Value">
            <Input.TextArea rows={3} placeholder={editing?.is_secret ? tr("留空表示不变") : tr("配置值")} />
          </Form.Item>
          <Form.Item name="group" label={tr("分组")}>
            <Input placeholder="ga4 / gsc / indexnow / bing" />
          </Form.Item>
          <Form.Item name="is_secret" label={tr("是否敏感（AES 加密）")} valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  )
}
