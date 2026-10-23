import tr from "./translate"
import { App as AntdApp } from "antd"
// 页面 SEO 元数据管理：列表 + 新建/编辑 + 删除
// 支持 hreflang JSON / structured_data JSON-LD 数组
import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, Modal, Popconfirm, Space, Tag, Typography } from 'antd'
import { Table } from './ResizableTable'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import FilePicker from './FilePicker'
import {
  createPageMeta,
  deletePageMeta,
  listPageMetas,
  updatePageMeta,
  type SEOPageMeta,
} from './api'

interface FormValues extends Partial<SEOPageMeta> {}

export default function PageMetasPage() {
  const { message } = AntdApp.useApp()
  const [rows, setRows] = useState<SEOPageMeta[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [size, setSize] = useState(20)
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState<{ q?: string }>({})
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<SEOPageMeta | null>(null)
  const [form] = Form.useForm<FormValues>()

  async function load() {
    setLoading(true)
    try {
      const r = await listPageMetas({ page, size, ...filter })
      setRows(r.list)
      setTotal(r.total)
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page, size, filter])

  function openCreate() {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ robots: 'index,follow' })
    setOpen(true)
  }

  function openEdit(row: SEOPageMeta) {
    setEditing(row)
    form.setFieldsValue(row)
    setOpen(true)
  }

  async function submit() {
    const v = await form.validateFields()
    try {
      if (editing) {
        await updatePageMeta(editing.id, v)
        message.success(tr("已保存"))
      } else {
        await createPageMeta(v)
        message.success(tr("已创建"))
      }
      setOpen(false)
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  async function remove(id: string) {
    try {
      await deletePageMeta(id)
      message.success(tr("已删除"))
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const columns: ColumnsType<SEOPageMeta> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: 'Path', dataIndex: 'path', width: 220 },
    { title: 'Title', dataIndex: 'title' },
    {
      title: 'Canonical',
      dataIndex: 'canonical',
      width: 240,
      render: (c: string) => (c ? <Typography.Text code>{c}</Typography.Text> : '-'),
    },
    {
      title: 'Robots',
      dataIndex: 'robots',
      width: 130,
      render: (r: string) => <Tag>{r}</Tag>,
    },
    {
      title: tr("操作"),
      fixed: 'right',
      width: 160,
      render: (_, row) => (
        <Space>
          <Button size="small" onClick={() => openEdit(row)}>{tr("编辑")}</Button>
          <Popconfirm title={tr(`删除 ${row.path}?`)} onConfirm={() => remove(row.id)}>
            <Button size="small" danger>{tr("删除")}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Typography.Title level={4} style={{ margin: 0 }}>{tr("页面 SEO 元数据")}</Typography.Title>
      <Card>
        <Space style={{ marginBottom: 16, width: '100%', justifyContent: 'space-between' }} wrap>
          <Input.Search
            placeholder={tr("搜索 path / title / description")}
            allowClear
            style={{ width: 280 }}
            onSearch={(v) => setFilter({ q: v })}
          />
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{tr("新增页面元数据")}</Button>
        </Space>
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={rows}
          scroll={{ x: 1000 }}
          pagination={{
            current: page,
            pageSize: size,
            total,
            onChange: (p, s) => {
              setPage(p)
              setSize(s)
            },
            showSizeChanger: true,
          }}
        />
      </Card>
      <Modal rootClassName="seo-reference"
        title={editing ? tr(`编辑 ${editing.path}`) : tr("新增页面元数据")}
        open={open}
        onOk={submit}
        onCancel={() => setOpen(false)}
        okText={tr("保存")}
        cancelText={tr("取消")}
        width={760}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="path" label="Path" rules={[{ required: true }]}>
            <Input placeholder="/dk-en/products/iphone-16-pro-max" disabled={!!editing} />
          </Form.Item>
          <Form.Item name="title" label="Title">
            <Input placeholder="iPhone 16 Pro Max Wholesale - Refurbished" />
          </Form.Item>
          <Form.Item name="description" label="Description">
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item name="canonical" label="Canonical">
            <Input placeholder="https://example.com/dk-en/products/iphone-16-pro-max" />
          </Form.Item>
          <Form.Item name="og_image" label="OG Image">
            <FilePicker accept="image/*" maxSize={5} />
          </Form.Item>
          <Form.Item name="robots" label="Robots">
            <Input placeholder="index,follow" />
          </Form.Item>
          <Form.Item name="hreflang" label="Hreflang JSON" tooltip='{"en":"https://example.com/dk-en/about","zh-CN":"https://example.com/dk-zh/about"}'>
            <Input.TextArea rows={3} placeholder='{"en":"https://example.com/dk-en/about","zh-CN":"https://example.com/dk-zh/about"}' />
          </Form.Item>
          <Form.Item name="structured_data" label="Structured Data (JSON-LD array)">
            <Input.TextArea rows={5} placeholder='[{"@context":"https://schema.org","@type":"Product",...}]' />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  )
}
