import tr from "./translate"
import { App as AntdApp } from "antd"
// AI 引用监测：任务管理 + 采样运行 + 证据
import { useEffect, useState } from 'react'
import { Alert, Button, Card, Drawer, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Tag, Typography } from 'antd'
import { Table } from './ResizableTable'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  createCitationRun,
  deleteCitationMonitor,
  getCitationRun,
  listCitationMonitors,
  listCitationRuns,
  upsertCitationMonitor,
  type SEOCitationEvidence,
  type SEOCitationMonitor,
  type SEOCitationRun,
} from './api'

export default function CitationMonitorPage() {
  const { message } = AntdApp.useApp()
  // monitors
  const [monitors, setMonitors] = useState<SEOCitationMonitor[]>([])
  const [mLoading, setMLoading] = useState(false)
  const [openM, setOpenM] = useState(false)
  const [editingM, setEditingM] = useState<SEOCitationMonitor | null>(null)
  const [mForm] = Form.useForm<SEOCitationMonitor>()

  // runs
  const [runs, setRuns] = useState<SEOCitationRun[]>([])
  const [rTotal, setRTotal] = useState(0)
  const [rPage, setRPage] = useState(1)
  const [rSize, setRSize] = useState(20)
  const [rLoading, setRLoading] = useState(false)
  const [rFilter, setRFilter] = useState<{ monitor_id?: string }>({})

  // 创建 run modal
  const [openRun, setOpenRun] = useState(false)
  const [runSubmitting, setRunSubmitting] = useState(false)
  const [runForm] = Form.useForm<{
    monitor_id: string
  }>()

  // 证据 drawer
  const [openEvidence, setOpenEvidence] = useState(false)
  const [evidence, setEvidence] = useState<SEOCitationEvidence[]>([])
  const [evidenceRun, setEvidenceRun] = useState<SEOCitationRun | null>(null)

  async function loadMonitors() {
    setMLoading(true)
    try {
      setMonitors(await listCitationMonitors())
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setMLoading(false)
    }
  }

  async function loadRuns() {
    setRLoading(true)
    try {
      const r = await listCitationRuns({ page: rPage, size: rSize, ...rFilter })
      setRuns(r.list)
      setRTotal(r.total)
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setRLoading(false)
    }
  }

  useEffect(() => {
    loadMonitors()
  }, [])

  useEffect(() => {
    loadRuns()
  }, [rPage, rSize, rFilter])

  async function submitMonitor() {
    const v = await mForm.validateFields()
    try {
      await upsertCitationMonitor(editingM?.id || null, v)
      message.success(tr("已保存"))
      setOpenM(false)
      loadMonitors()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  async function removeMonitor(id: string) {
    try {
      await deleteCitationMonitor(id)
      message.success(tr("已删除"))
      loadMonitors()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  async function submitRun() {
    const v = await runForm.validateFields()
    setRunSubmitting(true)
    try {
      const result = await createCitationRun(v)
      if (result.status === 'completed') message.success(tr("采样完成"))
      else message.error(result.error_message || tr("采样失败"))
      setOpenRun(false)
      loadRuns()
      loadMonitors()
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setRunSubmitting(false)
    }
  }

  async function showEvidence(id: string) {
    try {
      const r = await getCitationRun(id)
      setEvidenceRun(r.run)
      setEvidence(r.evidence || [])
      setOpenEvidence(true)
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const mCols: ColumnsType<SEOCitationMonitor> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: tr("名称"), dataIndex: 'monitor_name', width: 160 },
    { title: tr("品牌"), dataIndex: 'brand_name', width: 140 },
    {
      title: 'Provider',
      dataIndex: 'provider',
      width: 110,
      render: (p: string) => <Tag color="blue">{p}</Tag>,
    },
    { title: tr("查询文本"), dataIndex: 'query_text' },
    {
      title: tr("间隔(分)"),
      dataIndex: 'interval_minutes',
      width: 100,
      align: 'right' as const,
    },
    {
      title: tr("启用"),
      dataIndex: 'enabled',
      width: 80,
      render: (b: boolean) => (b ? <Tag color="green">{tr("是")}</Tag> : <Tag>{tr("否")}</Tag>),
    },
    {
      title: tr("操作"),
      fixed: 'right',
      width: 220,
      render: (_, row) => (
        <Space>
          <Button
            size="small"
            type="primary"
            onClick={() => {
              runForm.resetFields()
              runForm.setFieldsValue({
                monitor_id: row.id,
              })
              setOpenRun(true)
            }}
          >{tr("采样")}</Button>
          <Button
            size="small"
            onClick={() => {
              setEditingM(row)
              mForm.setFieldsValue(row)
              setOpenM(true)
            }}
          >{tr("编辑")}</Button>
          <Popconfirm title={tr(`删除 ${row.monitor_name}?`)} onConfirm={() => removeMonitor(row.id)}>
            <Button size="small" danger>{tr("删除")}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const rCols: ColumnsType<SEOCitationRun> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: 'Monitor', dataIndex: 'monitor_id', width: 100 },
    { title: 'Provider', dataIndex: 'provider', width: 110 },
    { title: 'Model', dataIndex: 'model', width: 160 },
    {
      title: tr("提及品牌"),
      dataIndex: 'brand_mentioned',
      width: 100,
      render: (b: boolean) => (b ? <Tag color="green">{tr("是")}</Tag> : <Tag>{tr("否")}</Tag>),
    },
    {
      title: tr("引用数"),
      dataIndex: 'citation_count',
      width: 90,
      align: 'right' as const,
    },
    {
      title: tr("状态"),
      dataIndex: 'status',
      width: 100,
      render: (s: string) => <Tag color={s === 'completed' ? 'green' : s === 'failed' ? 'red' : 'blue'}>{s}</Tag>,
    },
    {
      title: tr("时间"),
      dataIndex: 'started_at',
      width: 170,
      render: (t: number) => new Date(t * 1000).toLocaleString(),
    },
    {
      title: tr("操作"),
      fixed: 'right',
      width: 110,
      render: (_, row) => (
        <Button size="small" onClick={() => showEvidence(row.id)}>{tr("查看证据")}</Button>
      ),
    },
  ]

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Typography.Title level={4} style={{ margin: 0 }}>{tr("AI 引用监测")}</Typography.Title>
      <Card
        title={tr("监测任务")}
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              setEditingM(null)
              mForm.resetFields()
              mForm.setFieldsValue({ provider: 'chatgpt', interval_minutes: 1440, enabled: false })
              setOpenM(true)
            }}
          >{tr("新建任务")}</Button>
        }
      >
        <Table
          rowKey="id"
          loading={mLoading}
          columns={mCols}
          dataSource={monitors}
          pagination={false}
          size="small"
          scroll={{ x: 1100 }}
        />
      </Card>

      <Card title={tr("采样运行")}>
        <Space style={{ marginBottom: 16 }} wrap>
          <Input
            placeholder="Monitor ID"
            allowClear
            style={{ width: 140 }}
            onPressEnter={(e) => setRFilter((f) => ({ ...f, monitor_id: (e.target as HTMLInputElement).value }))}
          />
        </Space>
        <Table
          rowKey="id"
          loading={rLoading}
          columns={rCols}
          dataSource={runs}
          size="small"
          scroll={{ x: 1100 }}
          pagination={{
            current: rPage,
            pageSize: rSize,
            total: rTotal,
            onChange: (p, s) => {
              setRPage(p)
              setRSize(s)
            },
            showSizeChanger: true,
          }}
        />
      </Card>

      <Modal rootClassName="seo-reference"
        title={editingM ? tr("编辑任务") : tr("新建任务")}
        open={openM}
        onOk={submitMonitor}
        onCancel={() => setOpenM(false)}
        okText={tr("保存")}
        cancelText={tr("取消")}
        width={640}
      >
        <Form form={mForm} layout="vertical">
          <Form.Item name="monitor_name" label={tr("任务名")} rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="brand_name" label={tr("品牌名")} rules={[{ required: true }]}>
            <Input placeholder="iPhone Wholesale" />
          </Form.Item>
          <Form.Item name="provider" label="Provider">
            <Select
              options={[
                { label: 'ChatGPT', value: 'chatgpt' },
                { label: 'DeepSeek', value: 'deepseek' },
              ]}
            />
          </Form.Item>
          <Form.Item name="query_text" label={tr("查询文本")} rules={[{ required: true }]}>
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item name="interval_minutes" label={tr("间隔（分钟）")}>
            <InputNumber style={{ width: '100%' }} min={1} />
          </Form.Item>
          <Form.Item name="enabled" label={tr("启用")} valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Modal rootClassName="seo-reference"
        title={tr("运行真实采样")}
        open={openRun}
        onOk={submitRun}
        confirmLoading={runSubmitting}
        onCancel={() => setOpenRun(false)}
        okText={tr("开始采样")}
        cancelText={tr("取消")}
        width={720}
      >
        <Form form={runForm} layout="vertical">
          <Form.Item name="monitor_id" label="Monitor ID" rules={[{ required: true }]}>
            <Select showSearch optionFilterProp="label" options={monitors.map(monitor => ({ value: monitor.id, label: `${monitor.monitor_name} · ${monitor.id}` }))} />
          </Form.Item>
          <Typography.Paragraph type="secondary">{tr("系统将使用任务中的 Provider、查询文本和已配置模型发起请求，并自动记录品牌提及与引用链接。")}</Typography.Paragraph>
        </Form>
      </Modal>

      <Drawer rootClassName="seo-reference"
        title={tr(`证据 #${evidenceRun?.id || ''}`)}
        open={openEvidence}
        onClose={() => setOpenEvidence(false)}
        width={640}
      >
        {evidenceRun && (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Typography.Paragraph>
              <strong>{tr("品牌提及：")}</strong>
              {evidenceRun.brand_mentioned ? tr("是") : tr("否")}{tr("；引用数")}{evidenceRun.citation_count}
            </Typography.Paragraph>
            {evidenceRun.error_message && (
              <Alert type="error" showIcon message={tr("采样失败")} description={evidenceRun.error_message} />
            )}
            <div>
              <Typography.Title level={5}>Prompt</Typography.Title>
              <pre style={{ background: '#f5f5f5', padding: 12, borderRadius: 8, whiteSpace: 'pre-wrap' }}>
                {evidenceRun.prompt_text || '(empty)'}
              </pre>
            </div>
            <div>
              <Typography.Title level={5}>Answer</Typography.Title>
              <pre style={{ background: '#f5f5f5', padding: 12, borderRadius: 8, whiteSpace: 'pre-wrap', maxHeight: 320, overflow: 'auto' }}>
                {evidenceRun.answer_text || '(empty)'}
              </pre>
            </div>
            <div>
              <Typography.Title level={5}>{tr("证据链接")}</Typography.Title>
              {evidence.length === 0 ? (
                <Typography.Text type="secondary">{tr("无")}</Typography.Text>
              ) : (
                evidence.map((e) => (
                  <div key={e.id}>
                    <Tag>{e.evidence_type === 'provider_citation' ? tr("平台引用") : tr("正文链接")}</Tag>
                    <a href={e.source_url} target="_blank" rel="noreferrer">
                      {e.source_title || e.source_url}
                    </a>
                  </div>
                ))
              )}
            </div>
          </Space>
        )}
      </Drawer>
    </Space>
  )
}
