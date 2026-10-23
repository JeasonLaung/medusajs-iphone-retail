import tr from "./translate"
import { App as AntdApp } from "antd"
import { useEffect, useMemo, useState, type Key } from 'react'
import { InfoCircleOutlined, ReloadOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Checkbox, Divider, Form, Input, Modal, Popover, Space, Spin, Tabs, Tag, Tree, Typography } from 'antd'
import type { TreeDataNode } from 'antd'
import PageMetas from './PageMetas'
import SEOSettings from './SEOSettings'
import SubmissionHistory from './SubmissionHistory'
import { getSitemapPages, listSettings, submitIndexNow, testGoogleConnection, upsertSetting, type IndexNowSubmitResult, type SEOSetting, type SitemapPage } from './api'

interface Props { open: boolean; onClose: () => void; onSaved?: () => void }
type Values = Record<string, string | boolean>

const fields = [
  ['site_url', 'seo.site_url'], ['site_name', 'seo.site_name'], ['home_path', 'seo.home_path'],
  ['default_title', 'seo.default_title'], ['default_description', 'seo.default_description'],
  ['robots_txt', 'seo.robots_txt'], ['llms_txt', 'seo.llms_txt'], ['gsc_verification_code', 'gsc.verification_code'],
  ['ga_measurement_id', 'ga4.measurement_id'],
  ['ga_property_id', 'ga4.property_id'],
  ['ga_enabled', 'analytics.enabled'],
  ['ga_client_id', 'ga4.oauth.client_id'],
  ['ga_client_secret', 'ga4.oauth.client_secret'],
  ['ga_refresh_token', 'ga4.oauth.refresh_token'],
  ['gsc_resource', 'gsc.resource'],
  ['gsc_sitemap_url', 'gsc.sitemap_url'],
  ['gsc_enabled', 'seo.google_search_console.enabled'],
  ['gsc_client_id', 'gsc.oauth.client_id'],
  ['gsc_client_secret', 'gsc.oauth.client_secret'],
  ['gsc_refresh_token', 'gsc.oauth.refresh_token'],
  ['indexnow_enabled', 'seo.indexnow_enabled'],
  ['indexnow_key', 'seo.indexnow_key'],
  ['indexnow_auto_submit', 'seo.indexnow_auto_submit'],
  ['bing_enabled', 'bing.verification_enabled'],
  ['bing_site_auth_xml', 'bing.site_auth_xml'],
] as const

const secretFields = new Set(['ga_client_secret', 'ga_refresh_token', 'gsc_client_secret', 'gsc_refresh_token'])

function sitemapLocale(path: string) {
  const segment = path.split('/').filter(Boolean)[0]
  const locale = segment?.match(/^[a-z]{2}-(en|zh)$/)?.[1] || segment
  return locale && /^[a-z]{2}$/i.test(locale) ? locale.toLowerCase() : 'other'
}

function buildSitemapTree(pages: SitemapPage[]): TreeDataNode[] {
  const sitemapGroups = new Map<string, Map<string, SitemapPage[]>>()
  for (const page of pages) {
    const sitemap = page.sitemap || 'sitemap.xml'
    const locale = sitemapLocale(page.path)
    const localeGroups = sitemapGroups.get(sitemap) || new Map<string, SitemapPage[]>()
    const localePages = localeGroups.get(locale) || []
    localePages.push(page)
    localeGroups.set(locale, localePages)
    sitemapGroups.set(sitemap, localeGroups)
  }
  return [...sitemapGroups.entries()].sort(([left], [right]) => left.localeCompare(right)).map(([sitemap, localeGroups]) => ({
    key: `sitemap:${sitemap}`,
    title: `${sitemap}（${[...localeGroups.values()].reduce((total, list) => total + list.length, 0)}）`,
    children: [...localeGroups.entries()].sort(([left], [right]) => left.localeCompare(right)).map(([locale, localePages]) => ({
      key: `sitemap:${sitemap}:locale:${locale}`,
      title: tr(`${locale === 'other' ? tr("其他页面") : locale.toUpperCase()}（${localePages.length}）`),
      children: localePages.sort((left, right) => left.path.localeCompare(right.path)).map((page) => ({
        key: `url:${page.url}`,
        title: page.url,
      })),
    })),
  }))
}

export default function DataSourceConfig({ open, onClose, onSaved }: Props) {
  const { message } = AntdApp.useApp()
  const [form] = Form.useForm<Values>()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState<'ga4' | 'gsc' | null>(null)
  const [submittingIndexNow, setSubmittingIndexNow] = useState(false)
  const [indexNowResult, setIndexNowResult] = useState<{ success: boolean; message: string } | null>(null)
  const [indexNowResponse, setIndexNowResponse] = useState<IndexNowSubmitResult | null>(null)
  const [rows, setRows] = useState<SEOSetting[]>([])
  const [sitemapPages, setSitemapPages] = useState<SitemapPage[]>([])
  const [selectedSitemapKeys, setSelectedSitemapKeys] = useState<Key[]>([])
  const [loadingSitemap, setLoadingSitemap] = useState(false)
  const [lastIndexNowSubmittedAt, setLastIndexNowSubmittedAt] = useState('')

  const sitemapTree = useMemo(() => buildSitemapTree(sitemapPages), [sitemapPages])
  const sitemapURLByKey = useMemo(() => new Map(sitemapPages.map((page) => [`url:${page.url}`, page.url])), [sitemapPages])

  async function loadSitemap() {
    setLoadingSitemap(true)
    try {
      const pages = await getSitemapPages()
      setSitemapPages(pages)
    } catch (e) {
      setSitemapPages([])
      setSelectedSitemapKeys([])
      message.error(tr(`读取 Sitemap 失败：${(e as Error).message}`))
    } finally {
      setLoadingSitemap(false)
    }
  }

  useEffect(() => {
    if (!open) return
    void loadSitemap()
    setLoading(true)
    listSettings().then((items) => {
      setIndexNowResult(null)
      setRows(items)
      setLastIndexNowSubmittedAt(items.find((item) => item.key === 'seo.indexnow_last_submitted_at')?.value || '')
      const values: Values = {}
      for (const [name, key] of fields) {
        const row = items.find((item) => item.key === key)
        if (name.endsWith('_enabled') || name === 'ga_enabled' || name === 'indexnow_auto_submit') values[name] = row?.value === 'true'
        else values[name] = row?.value === '********' ? '' : (row?.value || '')
        if (secretFields.has(name)) values[`${name}_clear`] = false
      }
      form.resetFields()
      form.setFieldsValue(values)
    }).catch((e) => message.error((e as Error).message)).finally(() => setLoading(false))
  }, [open, form])

  useEffect(() => {
    setSelectedSitemapKeys(sitemapPages.filter((page) => {
      const lastSubmitted = Date.parse(page.last_submitted_at || '')
      if (!Number.isFinite(lastSubmitted) || !page.lastmod) return true
      const lastModified = Date.parse(page.lastmod)
      return !Number.isFinite(lastModified) || lastModified > lastSubmitted
    }).map((page) => `url:${page.url}`))
  }, [sitemapPages, lastIndexNowSubmittedAt])

  async function save() {
    const values = await form.validateFields()
    const touchedFields = fields.filter(([name]) => form.isFieldTouched(name) || Boolean(values[`${name}_clear`]))
    if (!touchedFields.length) {
      message.info(tr("没有需要保存的更改"))
      return
    }
    const touched = (name: string) => touchedFields.some(([field]) => field === name)
    const measurementID = String(values.ga_measurement_id || '').trim().toUpperCase()
    if ((touched('ga_enabled') || touched('ga_measurement_id')) && values.ga_enabled && !/^G-[A-Z0-9]{4,20}$/.test(measurementID)) {
      message.error(tr("启用 GA4 前，请填写有效的 Measurement ID（例如 G-XXXXXXXXXX）"))
      return
    }
    values.ga_measurement_id = measurementID
    const indexNowKey = String(values.indexnow_key || '').trim()
    if ((touched('indexnow_enabled') || touched('indexnow_key')) && values.indexnow_enabled && !/^[A-Za-z0-9-]{8,128}$/.test(indexNowKey)) {
      message.error(tr("启用 IndexNow 前，请填写 8–128 位字母、数字或连字符组成的 Key"))
      return
    }
    values.indexnow_key = indexNowKey
    const bingXML = String(values.bing_site_auth_xml || '').trim().replace(/^\uFEFF/, '').trim()
    if ((touched('bing_enabled') || touched('bing_site_auth_xml')) && values.bing_enabled && (!bingXML.startsWith('<?xml') || !/<users(?:\s[^>]*)?>[\s\S]*<user(?:\s[^>]*)?>[^<]+<\/user>[\s\S]*<\/users>/.test(bingXML))) {
      message.error(tr("启用 Bing 验证前，请粘贴 Bing Webmaster Tools 下载的完整 BingSiteAuth.xml 内容"))
      return
    }
    values.bing_site_auth_xml = bingXML
    setSaving(true)
    try {
      await Promise.all(touchedFields.map(([name, key]) => {
        const value = String(values[name] ?? '')
        const secret = key.includes('client_secret') || key.includes('refresh_token')
        const group = key.startsWith('ga4') || key.startsWith('analytics') ? 'ga4' : key.startsWith('gsc') || key.startsWith('seo.google') ? 'gsc' : key.startsWith('index') || key.includes('indexnow') ? 'indexnow' : key.startsWith('bing') ? 'bing' : 'site'
        return upsertSetting(key, { value, is_secret: secret, clear_secret: secret && Boolean(values[`${name}_clear`]), group })
      }))
      message.success(tr("数据源配置已保存"))
      onSaved?.()
      onClose()
    } catch (e) { message.error((e as Error).message) } finally { setSaving(false) }
  }

  async function testConnection(provider: 'ga4' | 'gsc') {
    setTesting(provider)
    try {
      await testGoogleConnection(provider)
      message.success(tr(`${provider === 'ga4' ? 'GA4' : 'Search Console'} OAuth 与报表权限验证成功`))
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setTesting(null)
    }
  }

  async function pushIndexNow() {
    const selectedURLs = selectedSitemapKeys.map((key) => sitemapURLByKey.get(String(key))).filter((url): url is string => Boolean(url))
    if (!selectedURLs.length) {
      message.warning(tr("请从 Sitemap 选择至少一个 URL"))
      return
    }
    setSubmittingIndexNow(true)
    setIndexNowResult(null)
    setIndexNowResponse(null)
    try {
      const result = await submitIndexNow(selectedURLs)
      setIndexNowResponse(result)
      if (result.success) {
        if (result.submitted_at) setLastIndexNowSubmittedAt(result.submitted_at)
        setSelectedSitemapKeys([])
        await loadSitemap()
      }
      const text = result.success
        ? tr(`推送成功：已提交 ${result.submitted_count} 个 URL（IndexNow HTTP ${result.http_status}）`)
        : tr(`推送失败：${result.error || `IndexNow HTTP ${result.http_status}`}`)
      setIndexNowResult({ success: result.success, message: text })
      if (result.success) message.success(text)
      else message.error(text)
    } catch (e) {
      const text = (e as Error).message || tr("IndexNow 推送失败")
      setIndexNowResult({ success: false, message: text })
      message.error(text)
    } finally {
      setSubmittingIndexNow(false)
    }
  }

  const secretValue = (name: string) => rows.find((item) => item.key === fields.find(([field]) => field === name)?.[1])?.value === '********'
  const input = (name: string, label: string, placeholder?: string) => <div><Form.Item name={name} label={label}><Input.Password placeholder={secretValue(name) ? tr("已保存；留空保持不变") : placeholder} /></Form.Item>{secretValue(name) && <Form.Item name={`${name}_clear`} valuePropName="checked"><Checkbox>{tr("清除已保存的")}{label}</Checkbox></Form.Item>}</div>
  const textInput = (name: string, label: string, placeholder?: string) => <Form.Item name={name} label={label}><Input placeholder={placeholder} /></Form.Item>
  const check = (name: string, label: string) => <Form.Item name={name} valuePropName="checked" noStyle><Checkbox>{label}</Checkbox></Form.Item>
  const gscOAuthHelp = <div style={{ maxWidth: 420, lineHeight: 1.6 }}>
    <Typography.Text strong>{tr("获取 Search Console Refresh Token")}</Typography.Text>
    <ol style={{ margin: '8px 0 0', paddingLeft: 20 }}>
      <li>{tr("在 Google Cloud 项目中启用 Search Console API，并创建 Web application 类型的 OAuth Client。")}</li>
      <li>{tr("将回调地址")}<Typography.Text code>https://developers.google.com/oauthplayground</Typography.Text>{tr("加入 Authorized redirect URIs。")}</li>
      <li>{tr("打开")}<a href="https://developers.google.com/oauthplayground" target="_blank" rel="noreferrer">Google OAuth 2.0 Playground</a>{tr("，点击右上角设置，勾选 Use your own OAuth credentials，填写同一组 Client ID 和 Client Secret。")}</li>
      <li>{tr("在 Step 1 输入")}<Typography.Text code>https://www.googleapis.com/auth/webmasters</Typography.Text>{tr("，授权有权访问该 Search Console 资源的 Google 账号。")}</li>
      <li>{tr("点击 Exchange authorization code for tokens，在 Step 2 复制 Refresh token。")}</li>
      <li>{tr("将 Client ID、Client Secret 和 Refresh Token 填入本页 Search Console 配置后保存。")}</li>
    </ol>
    <Typography.Text type="secondary">{tr("如果没有返回 Refresh Token，请先撤销该 OAuth 应用的授权，再重新授权；Refresh Token 属于敏感凭据，请勿公开。")}</Typography.Text>
  </div>

  const tabs = [
    { key: 'site', label: tr("站点设置"), children: <section className="datasource-section"><div className="form-two-col">{textInput('site_url', tr("站点 URL"), 'https://example.com')}{textInput('site_name', tr("站点名称"))}{textInput('home_path', tr("首页路径"), '/dk-en')}{textInput('default_title', tr("默认标题"))}</div>{textInput('default_description', tr("默认描述"))}{textInput('gsc_verification_code', tr("Google 验证码"))}<Form.Item name="robots_txt" label="robots.txt"><Input.TextArea rows={5} /></Form.Item><Form.Item name="llms_txt" label="llms.txt"><Input.TextArea rows={5} /></Form.Item></section> },
    { key: 'ga4', label: 'Google Analytics 4', children: <section className="datasource-section"><div className="form-two-col">{textInput('ga_measurement_id', 'Measurement ID', 'G-XXXXXXXXXX')}{textInput('ga_property_id', 'Property ID', '123456789')}</div><div className="datasource-check">{check('ga_enabled', tr("启用"))}</div><Divider orientation="left">{tr("GA4 OAuth 授权信息")}</Divider><div className="form-two-col">{textInput('ga_client_id', 'Client ID')}{input('ga_client_secret', 'Client Secret')}{input('ga_refresh_token', 'Refresh Token')}</div><Typography.Text type="secondary">{tr("仅授权 analytics.readonly 权限；敏感字段在后台接口中脱敏显示。")}</Typography.Text><div><Button loading={testing === 'ga4'} disabled={saving} onClick={() => testConnection('ga4')}>{tr("测试 OAuth 连接")}</Button></div></section> },
    { key: 'gsc', label: 'Google Search Console', children: <section className="datasource-section">{textInput('gsc_resource', tr("资源地址"), 'sc-domain:example.com')}{textInput('gsc_sitemap_url', tr("Sitemap 地址"), 'https://example.com/sitemap.xml')}<div className="datasource-check">{check('gsc_enabled', tr("启用"))}</div><Divider orientation="left">{tr("Search Console OAuth 授权信息")}<Popover content={gscOAuthHelp} title={null} trigger="click" placement="rightTop"><InfoCircleOutlined style={{ color: '#1677ff', cursor: 'pointer' }} /></Popover></Divider><div className="form-two-col">{textInput('gsc_client_id', 'Client ID')}{input('gsc_client_secret', 'Client Secret')}{input('gsc_refresh_token', 'Refresh Token')}</div><Typography.Text type="secondary">{tr("授权 webmasters 权限，用于读取 Search Console 报表和访问站点资源。")}</Typography.Text><div><Button loading={testing === 'gsc'} disabled={saving} onClick={() => testConnection('gsc')}>{tr("测试 OAuth 连接")}</Button></div></section> },
    { key: 'indexnow', label: 'IndexNow', children: <section className="datasource-section"><div className="datasource-check">{check('indexnow_enabled', tr("启用"))}</div>{textInput('indexnow_key', 'IndexNow Key')}<div className="datasource-check">{check('indexnow_auto_submit', tr("发布后自动提交"))}</div><Typography.Text type="secondary">{tr("启用后会在网站根目录提供 /")}{'{Key}'}{tr(".txt，并按每个 URL 的成功记录自动提交新增或更新页面。")}</Typography.Text><Divider orientation="left">{tr("手动推送")}</Divider><Typography.Paragraph type="secondary">{tr("从 Sitemap 树勾选本次需要提交的完整 URL。系统会根据 lastmod 自动取消未更新页面的勾选；缺少 lastmod 的页面继续保留，避免漏推。")}</Typography.Paragraph><Typography.Paragraph><Typography.Text strong>{tr("上次成功推送：")}</Typography.Text><Typography.Text>{lastIndexNowSubmittedAt ? new Date(lastIndexNowSubmittedAt).toLocaleString() : tr("暂无记录")}</Typography.Text></Typography.Paragraph><Card className="indexnow-sitemap-card" size="small" title={tr(`Sitemap 页面（${sitemapPages.length}）`)} extra={<Button type="text" size="small" icon={<ReloadOutlined />} loading={loadingSitemap} onClick={() => void loadSitemap()}>{tr("刷新")}</Button>}><Spin spinning={loadingSitemap}><div className="indexnow-sitemap-tree">{sitemapTree.length ? <Tree height={340} virtual checkable defaultExpandAll treeData={sitemapTree} checkedKeys={selectedSitemapKeys} onCheck={(keys) => setSelectedSitemapKeys(keys as Key[])} /> : <Typography.Text type="secondary">{tr("暂无可选页面")}</Typography.Text>}</div></Spin></Card><Space wrap><Button type="primary" loading={submittingIndexNow} disabled={saving || loading || loadingSitemap} onClick={pushIndexNow}>{tr("推送选中的 URL")}</Button><Typography.Text type="secondary">{tr("已选择")}{selectedSitemapKeys.filter((key) => sitemapURLByKey.has(String(key))).length}{tr("个更新页面")}</Typography.Text></Space>{indexNowResult && <Alert showIcon type={indexNowResult.success ? 'success' : 'error'} message={indexNowResult.success ? tr("IndexNow 推送成功") : tr("IndexNow 推送失败")} description={indexNowResult.message} />}{indexNowResponse && <Card className="indexnow-response" size="small" title={tr("IndexNow 接口响应")} extra={<Tag color={indexNowResponse.success ? 'success' : 'error'}>HTTP {indexNowResponse.http_status}</Tag>}><Typography.Text strong>Response Headers</Typography.Text><pre>{Object.entries(indexNowResponse.response_headers || {}).sort(([left], [right]) => left.localeCompare(right)).map(([name, values]) => `${name}: ${values.join(', ')}`).join('\n') || tr("（无响应头）")}</pre><Typography.Text strong>Response Body</Typography.Text><pre>{indexNowResponse.response_body || tr("（空响应正文）")}</pre></Card>}</section> },
    { key: 'bing', label: 'Bing Webmaster Tools', children: <section className="datasource-section"><div className="datasource-check">{check('bing_enabled', tr("生成 BingSiteAuth.xml"))}</div><Form.Item name="bing_site_auth_xml" label={tr("BingSiteAuth.xml 内容")}><Input.TextArea rows={6} placeholder={'<?xml version="1.0"?>\n<users>...</users>'} /></Form.Item><Typography.Text type="secondary">{tr("从 Bing Webmaster Tools 下载 BingSiteAuth.xml，用文本编辑器打开并粘贴完整内容；保存后网站根目录资源即时生效。")}</Typography.Text></section> },
    { key: 'page-metas', label: tr("页面 SEO"), children: <PageMetas /> },
    { key: 'advanced', label: tr("高级配置"), children: <SEOSettings /> },
    { key: 'submissions', label: tr("提交记录"), children: <SubmissionHistory /> },
  ]

  return <Modal rootClassName="seo-reference" open={open} width={900} centered destroyOnClose styles={{ body: { maxHeight: '76vh', overflowY: 'auto' } }} onCancel={onClose} footer={<><Button onClick={onClose}>{tr("取消")}</Button><Button type="primary" loading={saving} onClick={save}>{tr("保存配置")}</Button></>} title={<div><Typography.Title level={4} style={{ margin: 0 }}>{tr("配置搜索数据源")}</Typography.Title><Typography.Text type="secondary">{tr("保存连接配置，并管理搜索引擎验证与 URL 提交。")}</Typography.Text></div>}>
    <Spin spinning={loading}><Form form={form} layout="vertical" className="datasource-form">
      <Tabs defaultActiveKey="ga4" items={tabs} />
    </Form></Spin>
  </Modal>
}
