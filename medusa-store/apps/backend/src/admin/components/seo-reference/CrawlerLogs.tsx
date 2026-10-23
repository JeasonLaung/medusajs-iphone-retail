import tr from "./translate"
import { App as AntdApp } from "antd"
// AI crawler analytics. Shared Nginx sources are configured from the SEO/GEO overview dialog.
import { useEffect, useState } from 'react'
import { Button, Card, Col, Input, Row, Space, Statistic, Tag, Typography } from 'antd'
import { Table } from './ResizableTable'
import { SyncOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getCrawlerStats, listCrawlerVisits, scanCrawlerSources, type CrawlerStats, type SEOCrawlerVisit } from './api'

export default function CrawlerLogsPage() {
  const { message } = AntdApp.useApp()
  const [visits, setVisits] = useState<SEOCrawlerVisit[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [size, setSize] = useState(20)
  const [loading, setLoading] = useState(false)
  const [scanning, setScanning] = useState(false)
  const [filters, setFilters] = useState<{ source_id?: string; crawler?: string; path?: string }>({})
  const [stats, setStats] = useState<CrawlerStats | null>(null)

  async function loadVisits() {
    setLoading(true)
    try {
      const result = await listCrawlerVisits({ page, size, ...filters })
      setVisits(result.list)
      setTotal(result.total)
    } catch (error) {
      message.error((error as Error).message)
    } finally {
      setLoading(false)
    }
  }

  async function loadStats() {
    try {
      setStats(await getCrawlerStats())
    } catch (error) {
      message.error((error as Error).message)
    }
  }

  useEffect(() => { void loadStats() }, [])
  useEffect(() => { void loadVisits() }, [page, size, filters])

  async function scanNow() {
    setScanning(true)
    try {
      const result = await scanCrawlerSources()
      await Promise.all([loadStats(), loadVisits()])
      if (result.failed > 0) message.warning(tr(`抓取完成，${result.succeeded} 个数据源成功，${result.failed} 个失败`))
    } catch (error) {
      message.error((error as Error).message)
    } finally {
      setScanning(false)
    }
  }

  const columns: ColumnsType<SEOCrawlerVisit> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: tr("时间"), dataIndex: 'occurred_at', width: 170, render: (value: number) => new Date(value * 1000).toLocaleString() },
    { title: tr("来源 ID"), dataIndex: 'source_id', width: 90 },
    { title: tr("爬虫"), dataIndex: 'crawler_name', width: 160 },
    { title: tr("路径"), dataIndex: 'request_path' },
    { title: tr("状态"), dataIndex: 'status_code', width: 90, render: (value: number) => <Tag color={value < 400 ? 'green' : 'red'}>{value}</Tag> },
    { title: 'IP Hash', dataIndex: 'client_ip_hash', width: 220 },
  ]

  return <Space className="crawler-logs-page" direction="vertical" size="middle" style={{ width: '100%' }}>
    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
      <div>
        <Typography.Title level={4} style={{ margin: 0 }}>{tr("AI 爬虫日志")}</Typography.Title>
        <Typography.Text type="secondary">{tr("展示共享 Nginx 日志源识别出的 AI Bot 访问。")}</Typography.Text>
      </div>
      <Button type="primary" icon={<SyncOutlined spin={scanning} />} loading={scanning} onClick={() => void scanNow()}>{tr("立即抓取")}</Button>
    </div>

    <Row className="crawler-summary-row" gutter={[16, 16]}>
      <Col xs={24} lg={6}><Card className="crawler-summary-card crawler-summary-stat"><Statistic title={tr("命中爬虫总数")} value={stats?.total_count ?? 0} /></Card></Col>
      <Col xs={24} lg={9}><Card className="crawler-summary-card" title={tr("Top 爬虫")}><div className="crawler-ranking-list">{stats?.top_crawlers?.length ? stats.top_crawlers.map((item) => <div className="crawler-ranking-row" key={item.name}><Tag color="blue">{item.name}</Tag><Typography.Text>{item.count}</Typography.Text></div>) : <Typography.Text type="secondary">{tr("暂无数据")}</Typography.Text>}</div></Card></Col>
      <Col xs={24} lg={9}><Card className="crawler-summary-card" title={tr("Top 路径")}><div className="crawler-ranking-list">{stats?.top_paths?.length ? stats.top_paths.map((item) => <div className="crawler-ranking-row" key={item.name}><Typography.Text code ellipsis={{ tooltip: item.name }}>{item.name}</Typography.Text><Typography.Text>{item.count}</Typography.Text></div>) : <Typography.Text type="secondary">{tr("暂无数据")}</Typography.Text>}</div></Card></Col>
    </Row>

    <Card title={tr("访问记录")}>
      <Space style={{ marginBottom: 16 }} wrap>
        <Input.Search allowClear placeholder={tr("爬虫名")} style={{ width: 180 }} onSearch={(crawler) => { setPage(1); setFilters((value) => ({ ...value, crawler })) }} />
        <Input.Search allowClear placeholder={tr("路径")} style={{ width: 220 }} onSearch={(path) => { setPage(1); setFilters((value) => ({ ...value, path })) }} />
        <Input.Search allowClear placeholder={tr("来源 ID")} style={{ width: 140 }} onSearch={(source_id) => { setPage(1); setFilters((value) => ({ ...value, source_id })) }} />
        <Button onClick={() => { void loadStats(); void loadVisits() }}>{tr("刷新")}</Button>
      </Space>
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={visits}
        size="small"
        scroll={{ x: 1000 }}
        pagination={{ current: page, pageSize: size, total, showSizeChanger: true, onChange: (nextPage, nextSize) => { setPage(nextPage); setSize(nextSize) } }}
      />
    </Card>
  </Space>
}
