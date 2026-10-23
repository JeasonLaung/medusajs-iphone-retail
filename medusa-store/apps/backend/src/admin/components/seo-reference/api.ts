// SEO+GEO 运营 API
import client from './client'
import type { ApiEnvelope, PageResult } from './client'

export interface SEOSetting {
  id: string
  key: string
  value: string
  is_secret: boolean
  group: string
  created_at: number
}

export interface SEOCrawlerSource {
  id: string
  source_name: string
  source_type: string
  file_path: string
  interval_minutes: number
  enabled: boolean
  collect_ai_crawlers: boolean
  collect_traffic: boolean
  matched_lines: number
  traffic_matched_lines: number
  read_lines: number
  traffic_read_lines: number
  read_offset: number
  file_fingerprint: string
  last_scanned_at: number
  last_error?: string
}

export interface CrawlerScanSummary {
  sources: number
  succeeded: number
  failed: number
}

export interface SEOCrawlerVisit {
  id: string
  source_id: string
  crawler_name: string
  request_path: string
  status_code: number
  client_ip_hash: string
  occurred_at: number
}

export interface CrawlerStatsRow {
  name: string
  count: number
}

export interface TrafficPathStatsRow extends CrawlerStatsRow {
  daily_unique_ip_count: number
  sources: CrawlerStatsRow[]
}
export interface CrawlerStats {
  top_crawlers: CrawlerStatsRow[]
  top_paths: CrawlerStatsRow[]
  trend: Array<{ date: string; count: number }>
  total_count: number
}

export interface TrafficAcquisitionVisit {
  id: string
  log_source_id: string
  platform: string
  source: string
  medium: string
  campaign: string
  click_id_type: string
  matched_by: string
  landing_path: string
  referrer_host: string
  status_code: number
  client_ip: string
  location: string
  occurred_at: number
}

export interface TrafficStats {
  total_count: number
  top_sources: CrawlerStatsRow[]
  top_platforms: CrawlerStatsRow[]
  top_paths: TrafficPathStatsRow[]
  top_paths_total: number
  top_locations: CrawlerStatsRow[]
  trend: Array<{ date: string; count: number }>
}

export interface SitemapPage {
  url: string
  path: string
  sitemap: string
  lastmod?: string
  last_submitted_at?: string
}

export interface SEOCitationMonitor {
  id: string
  monitor_name: string
  brand_name: string
  provider: string
  query_text: string
  interval_minutes: number
  enabled: boolean
  last_run_at?: number
}

export interface SEOCitationRun {
  id: string
  monitor_id: string
  provider: string
  model: string
  prompt_text: string
  answer_text: string
  status: string
  brand_mentioned: boolean
  citation_count: number
  started_at: number
  finished_at?: number
  error_message?: string
}

export interface SEOCitationEvidence {
  id: string
  run_id: string
  source_url: string
  source_title: string
  evidence_type: 'provider_citation' | 'answer_link'
}

export interface SEOProviderConfig {
  id: string
  provider: string
  api_key_configured: boolean
  base_url: string
  model: string
  wire_api: string
}

export interface SEOPageMeta {
  id: string
  path: string
  title: string
  description: string
  canonical: string
  og_image: string
  hreflang: string
  structured_data: string
  robots: string
}

export interface SiteAuditCheck {
  key: string
  title: string
  passed: boolean
  detail: string
  category: 'SEO' | 'GEO'
}

export interface SiteAuditResult {
  site_url: string
  checks: SiteAuditCheck[]
}

export interface IndexNowSubmitResult {
  success: boolean
  submitted_count: number
  http_status: number
  response_headers: Record<string, string[]>
  response_body: string
  submitted_at?: string
  error?: string
}

export interface GoogleDailyMetric {
  date: string
  active_users?: number
  new_users?: number
  sessions?: number
  key_events?: number
  clicks?: number
  impressions?: number
  ctr?: number
  position?: number
}

export interface GoogleOverview {
  days: number
  ga4: {
    connected: boolean
    error?: string
    active_users: number
    new_users: number
    sessions: number
    key_events: number
    trend: GoogleDailyMetric[]
  }
  gsc: {
    connected: boolean
    error?: string
    clicks: number
    impressions: number
    ctr: number
    position: number
    trend: GoogleDailyMetric[]
  }
}

// ===== Settings =====
export const listSettings = async (group?: string) => {
  const { data } = await client.get<ApiEnvelope<{ list: SEOSetting[] }>>('/operations/seo/settings', {
    params: group ? { group } : {},
  })
  if (data.code !== 0) throw new Error(data.message)
  return data.data!.list
}
export const upsertSetting = (key: string, b: { value: string; is_secret?: boolean; clear_secret?: boolean; group?: string }) =>
  put<SEOSetting>(`/operations/seo/settings/${encodeURIComponent(key)}`, b)
export const deleteSetting = (key: string) =>
  del(`/operations/seo/settings/${encodeURIComponent(key)}`)
export const testGoogleConnection = (provider: 'ga4' | 'gsc') =>
  post<{ provider: string; connected: boolean; expires_in: number; scope: string; token_type: string }>(`/operations/seo/google-connections/${provider}/test`, {})
export const submitIndexNow = (urls?: string[]) =>
  post<IndexNowSubmitResult>('/operations/seo/indexnow/submit', urls === undefined ? {} : { urls })
export const getGoogleOverview = (days = 31) =>
  get<GoogleOverview>(`/operations/seo/google-overview?days=${days}`)
export const getOperationTotals = () => get<{ completed_runs: number; mentioned_runs: number; provider_citations: number; answer_links: number }>('/operations/seo/overview')
export const getSiteAudit = async () => {
  const { data } = await client.get<ApiEnvelope<SiteAuditResult>>('/operations/seo/site-audit')
  if (data.code !== 0) throw new Error(data.message)
  return data.data!
}

// ===== Crawler sources =====
export const listCrawlerSources = async () => {
  const { data } = await client.get<ApiEnvelope<{ list: SEOCrawlerSource[] }>>('/operations/seo/crawler-sources')
  if (data.code !== 0) throw new Error(data.message)
  return data.data!.list
}
export const upsertCrawlerSource = (id: string | null, b: Partial<SEOCrawlerSource>) =>
  id === null
    ? post<SEOCrawlerSource>('/operations/seo/crawler-sources', b)
    : put<SEOCrawlerSource>(`/operations/seo/crawler-sources/${id}`, b)
export const deleteCrawlerSource = (id: string) =>
  del(`/operations/seo/crawler-sources/${id}`)
export const scanCrawlerSources = async () => {
  const { data } = await client.post<ApiEnvelope<CrawlerScanSummary>>('/operations/seo/crawler-sources/scan', {}, { timeout: 120000 })
  return data.data!
}

// ===== Crawler visits / stats =====
export const listCrawlerVisits = (p: {
  page?: number
  size?: number
  source_id?: string
  crawler?: string
  path?: string
  start?: string
  end?: string
} = {}) => paged<SEOCrawlerVisit>('/operations/seo/crawler-visits', p)
export const getCrawlerStats = async (start?: string, end?: string) => {
  const { data } = await client.get<ApiEnvelope<CrawlerStats>>('/operations/seo/crawler-stats', {
    params: { start, end },
  })
  if (data.code !== 0) throw new Error(data.message)
  return data.data!
}

export const listTrafficVisits = (p: {
  page?: number
  size?: number
  source?: string
  platform?: string
  path?: string
  locale?: string
  start?: string
  end?: string
} = {}) => paged<TrafficAcquisitionVisit>('/operations/seo/traffic-visits', p)

export const getTrafficStats = async (p: { source?: string; platform?: string; path?: string; locale?: string; start?: string; end?: string; path_page?: number; path_query?: string; path_source?: string } = {}) => {
  const { data } = await client.get<ApiEnvelope<TrafficStats>>('/operations/seo/traffic-stats', { params: p })
  if (data.code !== 0) throw new Error(data.message)
  return data.data!
}

export const getSitemapPages = async () => {
  const { data } = await client.get<ApiEnvelope<{ list: SitemapPage[] }>>('/operations/seo/sitemap-pages')
  if (data.code !== 0) throw new Error(data.message)
  return data.data!.list
}

// ===== Citation monitors =====
export const listCitationMonitors = async () => {
  const { data } = await client.get<ApiEnvelope<{ list: SEOCitationMonitor[] }>>('/operations/seo/citation-monitors')
  if (data.code !== 0) throw new Error(data.message)
  return data.data!.list
}
export const upsertCitationMonitor = (id: string | null, b: Partial<SEOCitationMonitor>) =>
  id === null
    ? post<SEOCitationMonitor>('/operations/seo/citation-monitors', b)
    : put<SEOCitationMonitor>(`/operations/seo/citation-monitors/${id}`, b)
export const deleteCitationMonitor = (id: string) =>
  del(`/operations/seo/citation-monitors/${id}`)

// ===== Citation runs =====
export const listCitationRuns = (p: { page?: number; size?: number; monitor_id?: string } = {}) =>
  paged<SEOCitationRun>('/operations/seo/citation-runs', p)
export const getCitationRun = (id: string) =>
  get<{ run: SEOCitationRun; evidence: SEOCitationEvidence[] }>(`/operations/seo/citation-runs/${id}`)
export const createCitationRun = (b: { monitor_id: string }) =>
  post<SEOCitationRun>('/operations/seo/citation-runs', b)

// ===== Provider configs =====
export const listProviders = async () => {
  const { data } = await client.get<ApiEnvelope<{ list: SEOProviderConfig[] }>>('/operations/seo/providers')
  if (data.code !== 0) throw new Error(data.message)
  return data.data!.list
}
export const upsertProvider = (id: string | null, b: { provider?: string; api_key?: string; base_url?: string; model?: string; wire_api?: string }) =>
  id === null
    ? post<SEOProviderConfig>('/operations/seo/providers', b)
    : put<SEOProviderConfig>(`/operations/seo/providers/${id}`, b)
export const deleteProvider = (id: string) =>
  del(`/operations/seo/providers/${id}`)

// ===== Page metas =====
export const listPageMetas = (p: { page?: number; size?: number; q?: string } = {}) =>
  paged<SEOPageMeta>('/operations/seo/page-metas', p)
export const createPageMeta = (b: Partial<SEOPageMeta>) =>
  post<SEOPageMeta>('/operations/seo/page-metas', b)
export const updatePageMeta = (id: string, b: Partial<SEOPageMeta>) =>
  put<SEOPageMeta>(`/operations/seo/page-metas/${id}`, b)
export const deletePageMeta = (id: string) =>
  del(`/operations/seo/page-metas/${id}`)

// ===== helpers =====
async function paged<T>(path: string, params: Record<string, unknown> | object): Promise<PageResult<T>> {
  const { data } = await client.get<ApiEnvelope<PageResult<T>>>(path, { params })
  if (data.code !== 0) throw new Error(data.message)
  return data.data!
}
async function get<T>(path: string): Promise<T> {
  const { data } = await client.get<ApiEnvelope<T>>(path)
  if (data.code !== 0) throw new Error(data.message)
  return data.data!
}
async function post<T>(path: string, body: unknown): Promise<T> {
  const { data } = await client.post<ApiEnvelope<T>>(path, body)
  if (data.code !== 0) throw new Error(data.message)
  return data.data!
}
async function put<T>(path: string, body: unknown): Promise<T> {
  const { data } = await client.put<ApiEnvelope<T>>(path, body)
  if (data.code !== 0) throw new Error(data.message)
  return data.data!
}
async function del(path: string): Promise<void> {
  const { data } = await client.delete<ApiEnvelope<{ deleted: boolean }>>(path)
  if (data.code !== 0) throw new Error(data.message)
}
