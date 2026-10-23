import tr from "./translate"
import { App as AntdApp, theme } from "antd"
import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Col, Empty, List, Row, Space, Statistic, Tag, Typography } from 'antd'
import { Table } from './ResizableTable'
import type { TableColumnsType } from 'antd'
import type { ReactNode } from 'react'
import { CheckCircleOutlined, DatabaseOutlined, LinkOutlined, PieChartOutlined, ReloadOutlined, RobotOutlined, SettingOutlined } from '@ant-design/icons'
import { Link } from 'react-router-dom'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { getCrawlerStats, getOperationTotals, getGoogleOverview, getSiteAudit, getTrafficStats, listCitationMonitors, listCrawlerSources, listProviders, listSettings, type CrawlerStats, type GoogleDailyMetric, type GoogleOverview, type SEOCrawlerSource, type SEOProviderConfig, type SEOSetting, type SiteAuditResult, type TrafficStats } from './api'
import DataSourceConfig from './DataSourceConfig'
import NginxLogSourceDialog from './NginxLogSourceDialog'

const { Text, Title } = Typography
echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])
function Metric({ title, value }: { title: string; value: string | number }) { return <div className="metric-card"><Text type="secondary">{title}</Text><div className="metric-value">{value}</div></div> }
const number = new Intl.NumberFormat('en-GB', { maximumFractionDigits: 1 })

function GA4TrendChart({ data }: { data: GoogleDailyMetric[] }) {
  const { token } = theme.useToken()
  const host = useRef<HTMLDivElement>(null)
  const ordered = useMemo(() => [...data].sort((a, b) => a.date.localeCompare(b.date)), [data])
  useEffect(() => {
    if (!host.current || ordered.length === 0) return
    const chart = echarts.init(host.current)
    const colors = [token.colorPrimary, token.colorSuccess, token.colorPrimaryActive, token.colorWarning]
    const axisColor = token.colorTextSecondary
    const splitColor = token.colorSplit
    chart.setOption({
      color: colors,
      tooltip: { trigger: 'axis', backgroundColor: token.colorBgElevated, borderColor: token.colorBorder, textStyle: { color: token.colorText } },
      legend: { data: [tr("活跃用户"), tr("新用户"), tr("会话"), tr("关键事件")], top: 0, textStyle: { color: token.colorTextSecondary } },
      grid: { left: 52, right: 20, top: 52, bottom: 42 },
      xAxis: { type: 'category', boundaryGap: false, data: ordered.map((item) => item.date.slice(5)), axisLabel: { color: axisColor }, axisLine: { lineStyle: { color: axisColor } } },
      yAxis: { type: 'value', minInterval: 1, axisLabel: { color: axisColor }, splitLine: { lineStyle: { color: splitColor } } },
      series: [
        { name: tr("活跃用户"), type: 'line', smooth: true, showSymbol: ordered.length <= 12, data: ordered.map((item) => item.active_users ?? 0) },
        { name: tr("新用户"), type: 'line', smooth: true, showSymbol: ordered.length <= 12, data: ordered.map((item) => item.new_users ?? 0) },
        { name: tr("会话"), type: 'line', smooth: true, showSymbol: ordered.length <= 12, data: ordered.map((item) => item.sessions ?? 0) },
        { name: tr("关键事件"), type: 'line', smooth: true, showSymbol: ordered.length <= 12, data: ordered.map((item) => item.key_events ?? 0) },
      ],
    })
    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(host.current)
    return () => { observer.disconnect(); chart.dispose() }
  }, [ordered, token])
  return <div ref={host} className="google-trend-chart" role="img" aria-label={tr("Google Analytics 4 每日趋势折线图")} />
}

function TrafficTrendChart({ data }: { data: TrafficStats['trend'] }) {
  const { token } = theme.useToken()
  const host = useRef<HTMLDivElement>(null)
  const ordered = useMemo(() => [...data].sort((a, b) => a.date.localeCompare(b.date)), [data])
  useEffect(() => {
    if (!host.current || ordered.length === 0) return
    const chart = echarts.init(host.current)
    const color = token.colorSuccess
    const axisColor = token.colorTextSecondary
    const splitColor = token.colorSplit
    chart.setOption({
      tooltip: { trigger: 'axis', backgroundColor: token.colorBgElevated, borderColor: token.colorBorder, textStyle: { color: token.colorText } },
      grid: { left: 52, right: 20, top: 20, bottom: 42 },
      xAxis: { type: 'category', boundaryGap: false, data: ordered.map((item) => item.date.slice(5)), axisLabel: { color: axisColor }, axisLine: { lineStyle: { color: axisColor } } },
      yAxis: { type: 'value', minInterval: 1, axisLabel: { color: axisColor }, splitLine: { lineStyle: { color: splitColor } } },
      series: [{ name: tr("可归因访问"), type: 'line', smooth: true, showSymbol: ordered.length <= 12, data: ordered.map((item) => item.count), lineStyle: { width: 3, color }, itemStyle: { color }, areaStyle: { color, opacity: 0.08 } }],
    })
    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(host.current)
    return () => { observer.disconnect(); chart.dispose() }
  }, [ordered, token])
  return <div ref={host} className="google-trend-chart" role="img" aria-label={tr("渠道来源最近 30 天每日趋势折线图")} />
}

const gscColumns = (): TableColumnsType<GoogleDailyMetric> => [
  { title: tr("日期"), dataIndex: 'date', width: 108, defaultSortOrder: 'descend', sorter: (a, b) => a.date.localeCompare(b.date), sortDirections: ['descend', 'ascend'] },
  { title: tr("自然点击"), dataIndex: 'clicks', width: 112, align: 'right', sorter: (a, b) => (a.clicks ?? 0) - (b.clicks ?? 0), render: (value?: number) => number.format(value ?? 0) },
  { title: tr("搜索曝光"), dataIndex: 'impressions', width: 112, align: 'right', sorter: (a, b) => (a.impressions ?? 0) - (b.impressions ?? 0), render: (value?: number) => number.format(value ?? 0) },
  { title: tr("搜索 CTR"), dataIndex: 'ctr', width: 112, align: 'right', sorter: (a, b) => (a.ctr ?? 0) - (b.ctr ?? 0), render: (value?: number) => `${((value ?? 0) * 100).toFixed(2)}%` },
  { title: tr("平均排名"), dataIndex: 'position', width: 112, align: 'right', sorter: (a, b) => (a.position ?? 0) - (b.position ?? 0), render: (value?: number) => (value ?? 0).toFixed(1) },
]

export default function OperationsPage() {
  const { message } = AntdApp.useApp()
  const [settings, setSettings] = useState<SEOSetting[]>([])
  const [stats, setStats] = useState<CrawlerStats | null>(null)
  const [trafficStats, setTrafficStats] = useState<TrafficStats | null>(null)
  const [crawlerSources, setCrawlerSources] = useState<SEOCrawlerSource[]>([])
  const [google, setGoogle] = useState<GoogleOverview | null>(null)
  const [totals, setTotals] = useState({ completed_runs: 0, mentioned_runs: 0, provider_citations: 0, answer_links: 0 })
  const [monitors, setMonitors] = useState(0); const [providers, setProviders] = useState<SEOProviderConfig[]>([]); const [audit, setAudit] = useState<SiteAuditResult | null>(null)
  const [loading, setLoading] = useState(false); const [configOpen, setConfigOpen] = useState(false); const [logSourcesOpen, setLogSourcesOpen] = useState(false)
  async function load() {
    setLoading(true)
    const end = Math.floor(Date.now() / 1000)
    const start = end - 29 * 86400
    try { const [s, c, traffic, sources, m, p, a, g, overview] = await Promise.all([listSettings(), getCrawlerStats(), getTrafficStats({ start: start.toString(), end: end.toString() }), listCrawlerSources(), listCitationMonitors(), listProviders(), getSiteAudit(), getGoogleOverview(31), getOperationTotals()]); setSettings(s); setStats(c); setTrafficStats(traffic); setCrawlerSources(sources); setMonitors(m.length); setProviders(p); setAudit(a); setGoogle(g); setTotals(overview) }
    catch (e) { message.error((e as Error).message) } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])
  const value = (key: string) => settings.find((s) => s.key === key)?.value
  const siteURL = value('seo.site_url') || audit?.site_url || tr("尚未配置站点 URL")
  const checks = audit?.checks || []
  const passed = checks.filter((item) => item.passed).length
  const providerConfigured = (provider: string) => providers.some((item) => item.provider === provider && item.api_key_configured)
  const configuredProviders = providers.filter((item) => item.api_key_configured).length
  const completedCount = totals.completed_runs
  const mentionedCount = totals.mentioned_runs
  const citationLinks = totals.provider_citations + totals.answer_links
  const aiLogSources = crawlerSources.filter((item) => item.enabled && item.collect_ai_crawlers)
  const trafficLogSources = crawlerSources.filter((item) => item.enabled && item.collect_traffic)
  const aiLogsReady = aiLogSources.length > 0 && aiLogSources.every((item) => !item.last_error)
  const trafficLogsReady = trafficLogSources.length > 0 && trafficLogSources.every((item) => !item.last_error)
  const sourceRows: Array<[string, string, boolean, ReactNode]> = [['Google Search Console', google?.gsc.connected ? tr(`${google.days} 天真实报表已同步`) : (google?.gsc.error || tr("搜索曝光、点击、CTR 与关键词")), Boolean(google?.gsc.connected), <DatabaseOutlined key="gsc" />], ['Google Analytics 4', google?.ga4.connected ? tr(`${google.days} 天真实报表已同步`) : (google?.ga4.error || tr("访客、来源、会话与关键事件")), Boolean(google?.ga4.connected), <LinkOutlined key="ga4" />], [tr("渠道归因"), tr(`${trafficStats?.total_count || 0} 条可归因访问`), trafficLogsReady, <PieChartOutlined key="traffic" />], [tr("AI 爬虫日志"), tr(`${aiLogSources.length} 个日志来源 · ${stats?.total_count || 0} 条访问记录`), aiLogsReady, <RobotOutlined key="crawler" />], [tr("AI 引用监测"), tr(`${monitors} 个任务 · ${configuredProviders} 个 Provider · ${completedCount} 次成功采样`), configuredProviders > 0 && monitors > 0, <CheckCircleOutlined key="citation" />]]

  return <div className="operations-dashboard">
    <div className="operations-heading"><div><Text className="eyebrow">DISCOVERABILITY</Text><Title level={2}>{tr("SEO / GEO 可发现性")}</Title><Text type="secondary">{tr("技术基础与第一方数据接入状态，帮助你快速判断站点是否可被搜索和 AI 系统发现。")}</Text></div><Space wrap><Button icon={<SettingOutlined />} onClick={() => setConfigOpen(true)}>{tr("配置数据源")}</Button><Button type="primary" icon={<ReloadOutlined />} loading={loading} onClick={load}>{tr("刷新数据")}</Button></Space></div>
    <Card className="discover-summary"><div><Title level={3}>{tr("站点可发现性总览")}</Title><Text type="secondary">{tr("实时检测")}<a href={siteURL} target="_blank" rel="noreferrer">{siteURL}</a>{tr("；外部数据源以真实 API 同步结果为准。")}</Text></div><div className="summary-score"><Statistic value={passed} suffix={`/${checks.length || 7}`} /><Text type="secondary">{tr("实时技术检查通过")}</Text></div></Card>
    <Row className="operations-card-row" gutter={[16, 16]}><Col xs={24} lg={15}><Card title={<span className="section-title">{tr("技术基础检查")}<Tag color={passed === checks.length && checks.length ? 'green' : 'orange'}>{passed}/{checks.length || 7}{tr("通过")}</Tag></span>}><div className="discover-check-grid">{checks.map((item) => <div className={`discover-check ${item.passed ? 'is-pass' : 'is-warn'}`} key={item.key}><div className="discover-check-icon">{item.passed ? <CheckCircleOutlined /> : '!'}</div><div className="discover-check-body"><Text strong>{item.title}</Text><Text type="secondary">{item.detail}</Text></div><Text className="discover-check-label">{item.category}</Text></div>)}</div></Card></Col><Col xs={24} lg={9}><Card title={<span className="section-title">{tr("数据接入状态")}<Tag color="green">{settings.length}{tr("项配置")}</Tag></span>}><List dataSource={sourceRows} renderItem={([title, detail, ok, icon]) => <List.Item><List.Item.Meta avatar={<span className="status-icon">{icon}</span>} title={title} description={detail} /><Tag color={ok ? 'green' : 'default'}>{ok ? tr("✓ 已配置") : tr("未配置")}</Tag></List.Item>} /></Card></Col></Row>
    <Row className="google-data-row" gutter={[16, 16]}>
      <Col xs={24} sm={24} lg={12}><Card title={<span className="section-title">Google Analytics 4 <Tag color="blue">GA4</Tag></span>} extra={<Text type="secondary">{tr("最近")}{google?.days || 31}{tr("天 · 每日趋势")}</Text>}><div className="metric-grid"><Metric title={tr("活跃用户")} value={google?.ga4.connected ? number.format(google.ga4.active_users) : '—'} /><Metric title={tr("新用户")} value={google?.ga4.connected ? number.format(google.ga4.new_users) : '—'} /><Metric title={tr("会话")} value={google?.ga4.connected ? number.format(google.ga4.sessions) : '—'} /><Metric title={tr("关键事件")} value={google?.ga4.connected ? number.format(google.ga4.key_events) : '—'} /></div>{google?.ga4.error ? <Alert type="warning" showIcon message={tr("GA4 数据获取失败")} description={google.ga4.error} /> : google?.ga4.trend.length ? <GA4TrendChart data={google.ga4.trend} /> : <Empty description={tr("当前周期暂无 GA4 数据")} />}</Card></Col>
      <Col xs={24} sm={24} lg={12}><Card title={<span className="section-title">{tr("Google 自然搜索")}<Tag color="blue">SEO</Tag></span>} extra={<Text type="secondary">{tr("最近")}{google?.days || 31}{tr("天 · 默认日期倒序")}</Text>}><div className="metric-grid"><Metric title={tr("自然点击")} value={google?.gsc.connected ? number.format(google.gsc.clicks) : '—'} /><Metric title={tr("搜索曝光")} value={google?.gsc.connected ? number.format(google.gsc.impressions) : '—'} /><Metric title={tr("搜索 CTR")} value={google?.gsc.connected ? `${(google.gsc.ctr * 100).toFixed(1)}%` : '—'} /><Metric title={tr("平均排名")} value={google?.gsc.connected ? google.gsc.position.toFixed(1) : '—'} /></div>{google?.gsc.error ? <Alert type="warning" showIcon message={tr("Search Console 数据获取失败")} description={google.gsc.error} /> : <Table<GoogleDailyMetric> className="google-search-table" rowKey="date" size="small" columns={gscColumns()} dataSource={[...(google?.gsc.trend || [])].sort((a, b) => b.date.localeCompare(a.date))} pagination={false} scroll={{ x: 556 }} locale={{ emptyText: <Empty description={tr("当前周期暂无自然搜索数据")} /> }} />}</Card></Col>
    </Row>
    <Row className="operations-card-row" gutter={[16, 16]}><Col xs={24} lg={12}><Card title={<span className="section-title"><RobotOutlined />{tr("AI 爬虫访问")}</span>} extra={<Space><Link to="/seo/crawler-logs">{tr("查看详情")}</Link><Button type="link" size="small" onClick={() => setLogSourcesOpen(true)}>{tr("配置日志源")}</Button></Space>}><div className="metric-grid"><Metric title={tr("AI Bot 访问")} value={stats?.total_count || 0} /><Metric title={tr("日志来源")} value={aiLogSources.length} /><Metric title={tr("Top 爬虫")} value={stats?.top_crawlers?.[0]?.name || '—'} /><Metric title={tr("Top URL 次数")} value={stats?.top_paths?.[0]?.count || 0} /></div><List size="small" dataSource={(stats?.top_paths || []).slice(0, 5)} renderItem={(item) => <List.Item><Text code>{item.name}</Text><Text type="secondary">{item.count}{tr("次")}</Text></List.Item>} /></Card></Col><Col xs={24} lg={12}><Card title={<span className="section-title"><LinkOutlined />{tr("AI 引用监测")}</span>} extra={<Space><Link to="/seo/citation-monitor">{tr("管理监控任务")}</Link><Link to="/seo/providers">{tr("配置 API")}</Link></Space>}><div className="metric-grid"><Metric title={tr("成功采样")} value={completedCount} /><Metric title={tr("品牌命中")} value={mentionedCount} /><Metric title={tr("引用链接")} value={citationLinks} /><Metric title={tr("监测任务")} value={monitors} /></div><Tag color={providerConfigured('chatgpt') ? 'green' : 'default'}>ChatGPT {providerConfigured('chatgpt') ? tr("已配置") : tr("未配置 API")}</Tag> <Tag color={providerConfigured('deepseek') ? 'green' : 'default'}>DeepSeek {providerConfigured('deepseek') ? tr("已配置") : tr("未配置 API")}</Tag></Card></Col></Row>
    <Card title={<span className="section-title"><PieChartOutlined />{tr("渠道归因")}<Tag color="blue">{tr("最近 30 天")}</Tag></span>} extra={<Space><Link to="/seo/traffic-sources">{tr("查看更多")}</Link><Button type="link" size="small" onClick={() => setLogSourcesOpen(true)}>{tr("配置日志源")}</Button></Space>}>
      <div className="metric-grid"><Metric title={tr("可归因访问")} value={trafficStats?.total_count || 0} /><Metric title={tr("来源数量")} value={trafficStats?.top_sources?.length || 0} /><Metric title={tr("落地页数量")} value={trafficStats?.top_paths_total || 0} /><Metric title={tr("Top 平台")} value={trafficStats?.top_platforms?.[0]?.name || '—'} /></div>
      {trafficStats?.trend.length ? <TrafficTrendChart data={trafficStats.trend} /> : <Empty description={tr("最近 30 天暂无渠道来源数据")} />}
      <Row gutter={[24, 16]}>
        <Col xs={24} lg={12}><Title level={5}>{tr("来源入口 Top 5")}</Title><List size="small" dataSource={(trafficStats?.top_sources || []).slice(0, 5)} locale={{ emptyText: tr("暂无来源数据") }} renderItem={(item) => <List.Item><Text>{item.name}</Text><Text type="secondary">{item.count}{tr("次")}</Text></List.Item>} /></Col>
        <Col xs={24} lg={12}><Title level={5}>{tr("落地页面 Top 5")}</Title><List size="small" dataSource={(trafficStats?.top_paths || []).slice(0, 5)} locale={{ emptyText: tr("暂无落地页数据") }} renderItem={(item) => <List.Item><Text code ellipsis={{ tooltip: item.name }} style={{ maxWidth: '75%' }}>{item.name}</Text><Text type="secondary">{item.count}{tr("次")}</Text></List.Item>} /></Col>
      </Row>
    </Card>
    <NginxLogSourceDialog open={logSourcesOpen} onClose={() => setLogSourcesOpen(false)} onSaved={load} />
    <DataSourceConfig open={configOpen} onClose={() => setConfigOpen(false)} onSaved={load} />
  </div>
}
