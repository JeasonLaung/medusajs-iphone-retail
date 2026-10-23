import { model } from "@medusajs/framework/utils"

export const SeoSetting = model.define("seo_setting", {
  id: model.id().primaryKey(), key: model.text().unique(), value: model.text().default(""),
  is_secret: model.boolean().default(false), group: model.text().default("site"),
})
export const SeoPage = model.define("seo_page", {
  id: model.id().primaryKey(), path: model.text().unique(), title: model.text().default(""),
  description: model.text().default(""), canonical: model.text().default(""), og_image: model.text().default(""),
  hreflang: model.json().nullable(), structured_data: model.json().nullable(), robots: model.text().default("index,follow"),
})
export const SeoSource = model.define("seo_source", {
  id: model.id().primaryKey(), source_name: model.text(), source_type: model.text().default("nginx_file"),
  file_path: model.text(), interval_minutes: model.number().default(10), enabled: model.boolean().default(true),
  collect_ai_crawlers: model.boolean().default(true), collect_traffic: model.boolean().default(true),
  read_offset: model.bigNumber().default(0), read_lines: model.number().default(0),
  file_fingerprint: model.text().default(""), matched_lines: model.number().default(0),
  traffic_matched_lines: model.number().default(0), last_scanned_at: model.dateTime().nullable(), last_error: model.text().default(""),
})
export const SeoCrawlerVisit = model.define("seo_crawler_visit", {
  id: model.id().primaryKey(), source_id: model.text(), crawler_name: model.text(), request_path: model.text(),
  status_code: model.number(), client_ip_hash: model.text(), occurred_at: model.dateTime(), event_hash: model.text().unique(),
}).indexes([{ on: ["occurred_at", "crawler_name"] }, { on: ["source_id"] }])
export const SeoTrafficVisit = model.define("seo_traffic_visit", {
  id: model.id().primaryKey(), log_source_id: model.text(), platform: model.text(), source: model.text(),
  medium: model.text(), campaign: model.text().default(""), click_id_type: model.text().default(""),
  matched_by: model.text(), landing_path: model.text(), referrer_host: model.text().default(""),
  status_code: model.number(), client_ip: model.text(), location: model.text().default(""),
  occurred_at: model.dateTime(), event_hash: model.text().unique(),
}).indexes([{ on: ["occurred_at", "source"] }, { on: ["log_source_id"] }])
export const SeoProvider = model.define("seo_provider", {
  id: model.id().primaryKey(), provider: model.enum(["chatgpt", "deepseek"]).unique(),
  api_key: model.text().default(""), base_url: model.text(), model: model.text(),
  wire_api: model.enum(["responses", "chat", "chat_input"]).default("responses"),
})
export const SeoMonitor = model.define("seo_monitor", {
  id: model.id().primaryKey(), monitor_name: model.text(), brand_name: model.text(),
  provider: model.enum(["chatgpt", "deepseek"]), query_text: model.text(),
  interval_minutes: model.number().default(1440), enabled: model.boolean().default(false), last_run_at: model.dateTime().nullable(),
})
export const SeoRun = model.define("seo_run", {
  id: model.id().primaryKey(), monitor_id: model.text(), provider: model.text(), model: model.text().default(""),
  prompt_text: model.text(), answer_text: model.text().default(""),
  status: model.enum(["running", "completed", "failed"]).default("running"),
  brand_mentioned: model.boolean().default(false), citation_count: model.number().default(0),
  started_at: model.dateTime(), finished_at: model.dateTime().nullable(), error_message: model.text().default(""),
}).indexes([{ on: ["monitor_id", "started_at"] }])
export const SeoEvidence = model.define("seo_evidence", {
  id: model.id().primaryKey(), run_id: model.text(), source_url: model.text(), source_title: model.text().default(""),
  evidence_type: model.enum(["provider_citation", "answer_link"]).default("answer_link"),
}).indexes([{ on: ["run_id"] }])
export const SeoSubmission = model.define("seo_submission", {
  id: model.id().primaryKey(), success: model.boolean().default(false), submitted_count: model.number().default(0),
  http_status: model.number().default(0), urls: model.json(), response_headers: model.json().nullable(),
  response_body: model.text().default(""), error: model.text().default(""), submitted_at: model.dateTime(),
})
