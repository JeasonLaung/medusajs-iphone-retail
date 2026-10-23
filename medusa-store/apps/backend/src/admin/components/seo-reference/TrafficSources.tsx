import tr from "./translate"
import { App as AntdApp } from "antd"
import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Card, Cascader, Col, DatePicker, Empty, Input, Popover, Row, Segmented, Space, Statistic, Tag, Typography, theme as antdTheme } from 'antd'
import { Table } from './ResizableTable'
import { InfoCircleOutlined, SyncOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as echarts from 'echarts/core'
import type { ComposeOption } from 'echarts/core'
import { LineChart, PieChart } from 'echarts/charts'
import type { LineSeriesOption, PieSeriesOption } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import type { GridComponentOption, LegendComponentOption, TooltipComponentOption } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import dayjs from 'dayjs'
import { getSitemapPages, getTrafficStats, listTrafficVisits, scanCrawlerSources, type CrawlerStatsRow, type SitemapPage, type TrafficAcquisitionVisit, type TrafficPathStatsRow, type TrafficStats } from './api'

echarts.use([LineChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

const { RangePicker } = DatePicker
type ChartOption = ComposeOption<LineSeriesOption | PieSeriesOption | GridComponentOption | LegendComponentOption | TooltipComponentOption>
type DateFilters = { start: string; end: string }
type PageCascadeOption = { label: string; value: string; children?: PageCascadeOption[] }

function datesForRange(days: number): DateFilters {
  return {
    start: dayjs().subtract(days - 1, 'day').startOf('day').unix().toString(),
    end: dayjs().endOf('day').unix().toString(),
  }
}

function pageGroup(path: string) {
  const segment = path.split('/').filter(Boolean)[0]
  const locale = segment?.match(/^[a-z]{2}-(en|zh)$/)?.[1] || segment
  return locale && /^[a-z]{2}$/i.test(locale) ? `locale:${locale.toLowerCase()}` : 'locale:other'
}

function buildPageOptions(pages: SitemapPage[]): PageCascadeOption[] {
  const groups = new Map<string, PageCascadeOption[]>()
  for (const page of pages) {
    const group = pageGroup(page.path)
    const children = groups.get(group) || []
    children.push({ label: page.path, value: page.path })
    groups.set(group, children)
  }
  return [...groups.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([group, children]) => ({
      label: group === 'locale:other' ? tr("其他页面") : group.slice('locale:'.length).toUpperCase(),
      value: group,
      children: children.sort((left, right) => left.value.localeCompare(right.value)),
    }))
}

function fillDailyTrend(points: TrafficStats['trend'], filters: DateFilters) {
  const values = new Map(points.map((point) => [point.date, point.count]))
  const result: Array<{ date: string; count: number }> = []
  let current = dayjs.unix(Number(filters.start)).startOf('day')
  const end = dayjs.unix(Number(filters.end)).startOf('day')
  while (!current.isAfter(end) && result.length < 366) {
    const date = current.format('YYYY-MM-DD')
    result.push({ date, count: values.get(date) || 0 })
    current = current.add(1, 'day')
  }
  return result
}

function Chart({ option, empty, height = 300, label }: { option: ChartOption; empty: boolean; height?: number; label?: string }) {
  const elementRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!elementRef.current || empty) return
    const chart = echarts.init(elementRef.current)
    chart.setOption(option)
    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(elementRef.current)
    return () => {
      observer.disconnect()
      chart.dispose()
    }
  }, [empty, option])

  return empty ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={tr("当前范围暂无来源访问")} /> : <div ref={elementRef} role="img" aria-label={label} style={{ width: '100%', height }} />
}

function DailyTrendChart({ stats, filters }: { stats: TrafficStats; filters: DateFilters }) {
  const { token } = antdTheme.useToken()
  const points = useMemo(() => fillDailyTrend(stats.trend, filters), [filters, stats.trend])
  const option = useMemo<ChartOption>(() => ({
    tooltip: { trigger: 'axis', backgroundColor: token.colorBgElevated, borderColor: token.colorBorder, textStyle: { color: token.colorText } },
    grid: { left: 20, right: 20, top: 24, bottom: 12, containLabel: true },
    xAxis: { type: 'category', boundaryGap: false, data: points.map((point) => point.date), axisLabel: { color: token.colorTextSecondary } },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: token.colorTextSecondary }, splitLine: { lineStyle: { color: token.colorBorderSecondary } } },
    series: [{ type: 'line', name: tr("来源访问"), data: points.map((point) => point.count), smooth: true, symbolSize: 6, lineStyle: { width: 3, color: token.colorPrimary }, itemStyle: { color: token.colorPrimary }, areaStyle: { color: token.colorPrimaryBg } }],
  }), [points, token])

  return <Chart option={option} empty={!points.some((point) => point.count > 0)} height={320} label={tr("渠道每日趋势折线图")} />
}

function DistributionChart({ items, label }: { items: CrawlerStatsRow[]; label: string }) {
  const { token } = antdTheme.useToken()
  const option = useMemo<ChartOption>(() => {
    const total = items.reduce((sum, item) => sum + item.count, 0)
    const percentageByName = new Map(items.map((item) => [item.name, total ? (item.count / total) * 100 : 0]))
    const formatPercentage = (value: number) => `${value.toLocaleString('zh-CN', { maximumFractionDigits: 1 })}%`

    return {
      tooltip: { trigger: 'item', backgroundColor: token.colorBgElevated, borderColor: token.colorBorder, textStyle: { color: token.colorText }, renderMode: 'richText', formatter: '{b}\n{c} ({d}%)' },
      legend: {
        type: 'scroll',
        bottom: 0,
        left: 'center',
        textStyle: { color: token.colorTextSecondary },
        itemWidth: 10,
        itemHeight: 10,
        formatter: (name: string) => `${name} ${formatPercentage(percentageByName.get(name) || 0)}`,
      },
      series: [{
        type: 'pie',
        radius: ['40%', '66%'],
        center: ['50%', '40%'],
        avoidLabelOverlap: true,
        itemStyle: { borderColor: token.colorBgContainer, borderWidth: 2 },
        label: { show: false },
        emphasis: { label: { show: true, formatter: '{b}\n{d}%' } },
        data: items.map((item) => ({ name: item.name, value: item.count })),
      }],
    }
  }, [items, token])

  return <Chart option={option} empty={!items.length} height={280} label={label} />
}

export default function TrafficSourcesPage() {
  const { message } = AntdApp.useApp()
  const [rows, setRows] = useState<TrafficAcquisitionVisit[]>([])
  const [stats, setStats] = useState<TrafficStats | null>(null)
  const [pageOptions, setPageOptions] = useState<PageCascadeOption[]>([])
  const [selectedLocale, setSelectedLocale] = useState<string>()
  const [selectedPath, setSelectedPath] = useState<string>()
  const [loading, setLoading] = useState(false)
  const [scanning, setScanning] = useState(false)
  const [page, setPage] = useState(1)
  const [size, setSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [rangeDays, setRangeDays] = useState(30)
  const [dateFilters, setDateFilters] = useState<DateFilters>(() => datesForRange(30))
  const [detailFilters, setDetailFilters] = useState<{ source?: string; platform?: string }>({})
  const [pathQuery, setPathQuery] = useState('')
  const [appliedPathQuery, setAppliedPathQuery] = useState('')
  const [pathSource, setPathSource] = useState('')
  const [appliedPathSource, setAppliedPathSource] = useState('')
  const [pathPage, setPathPage] = useState(1)

  async function load() {
    setLoading(true)
    try {
      const scope = { ...dateFilters, ...(selectedLocale ? { locale: selectedLocale } : {}), ...(selectedPath ? { path: selectedPath } : {}) }
      const [visits, summary] = await Promise.all([
        selectedPath ? listTrafficVisits({ page, size, ...scope, ...detailFilters }) : Promise.resolve(null),
        getTrafficStats({
          ...scope,
          path_page: pathPage,
          ...(appliedPathQuery ? { path_query: appliedPathQuery } : {}),
          ...(appliedPathSource ? { path_source: appliedPathSource } : {}),
        }),
      ])
      setRows(visits?.list || [])
      setTotal(visits?.total || 0)
      setStats(summary)
    } catch (error) {
      message.error((error as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [page, size, dateFilters, detailFilters, selectedLocale, selectedPath, pathPage, appliedPathQuery, appliedPathSource])
  useEffect(() => {
    void getSitemapPages()
      .then((pages) => setPageOptions(buildPageOptions(pages)))
      .catch((error) => message.error(tr(`读取 Sitemap 页面失败：${(error as Error).message}`)))
  }, [])

  async function scanNow() {
    setScanning(true)
    try {
      const result = await scanCrawlerSources()
      await load()
      if (result.failed > 0) message.warning(tr(`抓取完成，${result.succeeded} 个数据源成功，${result.failed} 个失败`))
    } catch (error) {
      message.error((error as Error).message)
    } finally {
      setScanning(false)
    }
  }

  const columns: ColumnsType<TrafficAcquisitionVisit> = [
    { title: tr("时间"), dataIndex: 'occurred_at', width: 170, render: (value: number) => new Date(value * 1000).toLocaleString() },
    { title: tr("平台"), dataIndex: 'platform', width: 130, render: (value: string) => <Tag color="blue">{value}</Tag> },
    { title: tr("来源"), dataIndex: 'source', width: 130 },
    { title: tr("媒介"), dataIndex: 'medium', width: 120, render: (value: string) => value || '—' },
    { title: 'Campaign', dataIndex: 'campaign', width: 180, ellipsis: true, render: (value: string) => value || '—' },
    { title: tr("识别依据"), dataIndex: 'matched_by', width: 130 },
    { title: tr("点击参数"), dataIndex: 'click_id_type', width: 120, render: (value: string) => value || '—' },
    { title: tr("页面"), dataIndex: 'landing_path', width: 220, ellipsis: true },
    { title: 'Referer', dataIndex: 'referrer_host', width: 190, ellipsis: true, render: (value: string) => value || '—' },
    { title: 'IP', dataIndex: 'client_ip', width: 150 },
    { title: tr("地区"), dataIndex: 'location', width: 240, ellipsis: true, render: (value: string) => value || '—' },
    { title: tr("状态"), dataIndex: 'status_code', width: 80, render: (value: number) => <Tag color={value < 400 ? 'green' : 'red'}>{value}</Tag> },
  ]

  const pathColumns: ColumnsType<TrafficPathStatsRow> = [
    { title: tr("排名"), width: 72, align: 'center', render: (_value, _row, index) => (pathPage - 1) * 10 + index + 1 },
    { title: 'Path', dataIndex: 'name', ellipsis: true },
    {
      title: tr("来源"),
      dataIndex: 'sources',
      width: 320,
      render: (sources: CrawlerStatsRow[]) => (
        <Space size={[4, 4]} wrap>
          {sources.map((source) => <Tag key={source.name}>{source.name} · {source.count}</Tag>)}
        </Space>
      ),
    },
    { title: tr("点击次数"), dataIndex: 'count', width: 120, align: 'right' },
    { title: tr("当日去重 IP 次数"), dataIndex: 'daily_unique_ip_count', width: 160, align: 'right', sorter: (left, right) => left.daily_unique_ip_count - right.daily_unique_ip_count },
  ]

  const scopeTitle = selectedPath ? tr(`页面统计 · ${selectedPath}`) : selectedLocale ? tr(`${selectedLocale.toUpperCase()} 语言页面统计`) : tr("全站来源统计")

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <Space align="center" size={4}>
            <Typography.Title level={4} style={{ margin: 0 }}>{tr("渠道来源统计")}</Typography.Title>
            <Popover
            trigger="click"
            placement="bottomLeft"
            title={tr("来源统计说明")}
            content={(
              <Space direction="vertical" size={8} style={{ maxWidth: 380 }}>
                <Typography.Text>{tr("统计 Nginx 日志中能够识别来源的页面访问，识别优先级为：平台点击参数、UTM、外部 Referer。")}</Typography.Text>
                <Typography.Text><Typography.Text strong>{tr("纳入统计：")}</Typography.Text>{tr("AI 引荐、广告点击、UTM、搜索引荐、社交媒体及其他外部网站引荐。")}</Typography.Text>
                <Typography.Text><Typography.Text strong>{tr("未纳入统计：")}</Typography.Text>{tr("直接访问、站内跳转，以及没有来源参数且未提供 Referer 的访问。")}</Typography.Text>
                <Typography.Text type="secondary">{tr("这里展示可归因渠道数据；包含直接访问在内的全站流量请以 GA4 为准。")}</Typography.Text>
              </Space>
            )}
            >
              <Button type="text" size="small" icon={<InfoCircleOutlined />} aria-label={tr("查看渠道来源统计说明")} />
            </Popover>
          </Space>
          <div><Typography.Text type="secondary">{tr("基于 Nginx 落地请求中的平台点击参数、UTM 和外部 Referer 识别。")}</Typography.Text></div>
        </div>
        <Button type="primary" icon={<SyncOutlined spin={scanning} />} loading={scanning} onClick={() => void scanNow()}>{tr("立即抓取")}</Button>
      </div>

      <Card>
        <Space wrap size="middle">
          <Segmented
            value={rangeDays}
            options={[{ label: tr("近 7 天"), value: 7 }, { label: tr("近 30 天"), value: 30 }, { label: tr("近 90 天"), value: 90 }]}
            onChange={(value) => { const days = Number(value); setRangeDays(days); setPage(1); setPathPage(1); setDateFilters(datesForRange(days)) }}
          />
          <RangePicker
            value={[dayjs.unix(Number(dateFilters.start)), dayjs.unix(Number(dateFilters.end))]}
            onChange={(dates) => {
              if (!dates?.[0] || !dates[1]) return
              setRangeDays(0)
              setPage(1)
              setPathPage(1)
              setDateFilters({ start: dates[0].startOf('day').unix().toString(), end: dates[1].endOf('day').unix().toString() })
            }}
          />
          <Cascader
            allowClear
            showSearch
            value={selectedPath ? [pageGroup(selectedPath), selectedPath] : selectedLocale ? [`locale:${selectedLocale}`] : []}
            placeholder={tr("全部语言 · 全部页面")}
            style={{ width: 340, maxWidth: '100%' }}
            options={pageOptions}
            changeOnSelect
            displayRender={(labels) => labels.at(-1) || ''}
            onClear={() => {
              setSelectedLocale(undefined)
              setSelectedPath(undefined)
              setPage(1)
              setPathPage(1)
            }}
            onChange={(value) => {
              const group = value.length ? String(value[0]) : ''
              setSelectedLocale(group.startsWith('locale:') && group !== 'locale:other' ? group.slice('locale:'.length) : undefined)
              setSelectedPath(value.length > 1 ? String(value.at(-1)) : undefined)
              setPage(1)
              setPathPage(1)
            }}
          />
          <Button onClick={() => void load()}>{tr("刷新")}</Button>
        </Space>
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={6}>
          <Card style={{ height: '100%' }}><Statistic title={selectedPath ? tr("该页面来源访问") : selectedLocale ? tr(`${selectedLocale.toUpperCase()} 语言访问总量`) : tr("来源访问总量")} value={stats?.total_count || 0} /></Card>
        </Col>
        <Col xs={24} lg={18}>
          <Card title={tr(`${scopeTitle} · 按天`)} loading={loading && !stats}>
            {stats && <DailyTrendChart stats={stats} filters={dateFilters} />}
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={8}><Card title={tr("来源占比")}><DistributionChart items={stats?.top_sources || []} label={tr("来源占比环形图")} /></Card></Col>
        <Col xs={24} xl={8}><Card title={tr("平台占比")}><DistributionChart items={stats?.top_platforms || []} label={tr("平台占比环形图")} /></Card></Col>
        <Col xs={24} xl={8}><Card title={tr("地区占比")}><DistributionChart items={stats?.top_locations || []} label={tr("地区占比环形图")} /></Card></Col>
      </Row>

      {!selectedPath && (
        <Card title={tr(`${selectedLocale ? tr(`${selectedLocale.toUpperCase()} 语言`) : tr("全站")} Path 点击排行`)}>
          <Space wrap style={{ marginBottom: 16 }}>
            <Input.Search
              allowClear
              value={pathQuery}
              placeholder={tr("搜索 Path")}
              aria-label={tr("搜索 Path")}
              style={{ width: 280, maxWidth: '100%' }}
              onChange={(event) => {
                setPathQuery(event.target.value)
                if (!event.target.value) { setPathPage(1); setAppliedPathQuery('') }
              }}
              onSearch={(value) => { setPathPage(1); setAppliedPathQuery(value.trim()) }}
            />
            <Input.Search
              allowClear
              value={pathSource}
              placeholder={tr("过滤来源，例如 chatgpt")}
              aria-label={tr("过滤来源")}
              style={{ width: 240, maxWidth: '100%' }}
              onChange={(event) => {
                setPathSource(event.target.value)
                if (!event.target.value) { setPathPage(1); setAppliedPathSource('') }
              }}
              onSearch={(value) => { setPathPage(1); setAppliedPathSource(value.trim()) }}
            />
          </Space>
          <Table
            rowKey="name"
            loading={loading}
            columns={pathColumns}
            dataSource={stats?.top_paths || []}
            size="small"
            scroll={{ x: 880 }}
            pagination={{ current: pathPage, pageSize: 10, total: stats?.top_paths_total || 0, showSizeChanger: false, onChange: setPathPage }}
            locale={{ emptyText: tr("当前范围暂无 Path 点击数据") }}
          />
        </Card>
      )}

      {selectedPath && (
        <Card title={tr(`${selectedPath} · 访问明细`)}>
          <Space wrap style={{ marginBottom: 16 }}>
            <Input.Search allowClear placeholder={tr("来源，例如 chatgpt")} style={{ width: 210 }} onSearch={(source) => { setPage(1); setDetailFilters((value) => ({ ...value, source })) }} />
            <Input.Search allowClear placeholder={tr("平台，例如 meta_ads")} style={{ width: 220 }} onSearch={(platform) => { setPage(1); setDetailFilters((value) => ({ ...value, platform })) }} />
          </Space>
          <Table
            rowKey="id"
            loading={loading}
            columns={columns}
            dataSource={rows}
            size="small"
            scroll={{ x: 2200 }}
            pagination={{ current: page, pageSize: size, total, showSizeChanger: true, onChange: (nextPage, nextSize) => { setPage(nextPage); setSize(nextSize) } }}
          />
        </Card>
      )}
    </Space>
  )
}
