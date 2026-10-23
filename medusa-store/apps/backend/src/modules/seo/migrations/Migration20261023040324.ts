import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261023040324 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "seo_traffic_visit" drop constraint if exists "seo_traffic_visit_event_hash_unique";`);
    this.addSql(`alter table if exists "seo_setting" drop constraint if exists "seo_setting_key_unique";`);
    this.addSql(`alter table if exists "seo_provider" drop constraint if exists "seo_provider_provider_unique";`);
    this.addSql(`alter table if exists "seo_page" drop constraint if exists "seo_page_path_unique";`);
    this.addSql(`alter table if exists "seo_crawler_visit" drop constraint if exists "seo_crawler_visit_event_hash_unique";`);
    this.addSql(`create table if not exists "seo_crawler_visit" ("id" text not null, "source_id" text not null, "crawler_name" text not null, "request_path" text not null, "status_code" integer not null, "client_ip_hash" text not null, "occurred_at" timestamptz not null, "event_hash" text not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_crawler_visit_pkey" primary key ("id"));`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_seo_crawler_visit_event_hash_unique" ON "seo_crawler_visit" ("event_hash") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_crawler_visit_deleted_at" ON "seo_crawler_visit" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_crawler_visit_occurred_at_crawler_name" ON "seo_crawler_visit" ("occurred_at", "crawler_name") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_crawler_visit_source_id" ON "seo_crawler_visit" ("source_id") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_evidence" ("id" text not null, "run_id" text not null, "source_url" text not null, "source_title" text not null default '', "evidence_type" text check ("evidence_type" in ('provider_citation', 'answer_link')) not null default 'answer_link', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_evidence_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_evidence_deleted_at" ON "seo_evidence" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_evidence_run_id" ON "seo_evidence" ("run_id") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_monitor" ("id" text not null, "monitor_name" text not null, "brand_name" text not null, "provider" text check ("provider" in ('chatgpt', 'deepseek')) not null, "query_text" text not null, "interval_minutes" integer not null default 1440, "enabled" boolean not null default false, "last_run_at" timestamptz null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_monitor_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_monitor_deleted_at" ON "seo_monitor" ("deleted_at") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_page" ("id" text not null, "path" text not null, "title" text not null default '', "description" text not null default '', "canonical" text not null default '', "og_image" text not null default '', "hreflang" jsonb null, "structured_data" jsonb null, "robots" text not null default 'index,follow', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_page_pkey" primary key ("id"));`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_seo_page_path_unique" ON "seo_page" ("path") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_page_deleted_at" ON "seo_page" ("deleted_at") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_provider" ("id" text not null, "provider" text check ("provider" in ('chatgpt', 'deepseek')) not null, "api_key" text not null default '', "base_url" text not null, "model" text not null, "wire_api" text check ("wire_api" in ('responses', 'chat', 'chat_input')) not null default 'responses', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_provider_pkey" primary key ("id"));`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_seo_provider_provider_unique" ON "seo_provider" ("provider") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_provider_deleted_at" ON "seo_provider" ("deleted_at") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_run" ("id" text not null, "monitor_id" text not null, "provider" text not null, "model" text not null default '', "prompt_text" text not null, "answer_text" text not null default '', "status" text check ("status" in ('running', 'completed', 'failed')) not null default 'running', "brand_mentioned" boolean not null default false, "citation_count" integer not null default 0, "started_at" timestamptz not null, "finished_at" timestamptz null, "error_message" text not null default '', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_run_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_run_deleted_at" ON "seo_run" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_run_monitor_id_started_at" ON "seo_run" ("monitor_id", "started_at") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_setting" ("id" text not null, "key" text not null, "value" text not null default '', "is_secret" boolean not null default false, "group" text not null default 'site', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_setting_pkey" primary key ("id"));`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_seo_setting_key_unique" ON "seo_setting" ("key") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_setting_deleted_at" ON "seo_setting" ("deleted_at") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_source" ("id" text not null, "source_name" text not null, "source_type" text not null default 'nginx_file', "file_path" text not null, "interval_minutes" integer not null default 10, "enabled" boolean not null default true, "collect_ai_crawlers" boolean not null default true, "collect_traffic" boolean not null default true, "read_offset" numeric not null default 0, "read_lines" integer not null default 0, "file_fingerprint" text not null default '', "matched_lines" integer not null default 0, "traffic_matched_lines" integer not null default 0, "last_scanned_at" timestamptz null, "last_error" text not null default '', "raw_read_offset" jsonb not null default '{"value":"0","precision":20}', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_source_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_source_deleted_at" ON "seo_source" ("deleted_at") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_submission" ("id" text not null, "success" boolean not null default false, "submitted_count" integer not null default 0, "http_status" integer not null default 0, "urls" jsonb not null, "response_headers" jsonb null, "response_body" text not null default '', "error" text not null default '', "submitted_at" timestamptz not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_submission_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_submission_deleted_at" ON "seo_submission" ("deleted_at") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "seo_traffic_visit" ("id" text not null, "log_source_id" text not null, "platform" text not null, "source" text not null, "medium" text not null, "campaign" text not null default '', "click_id_type" text not null default '', "matched_by" text not null, "landing_path" text not null, "referrer_host" text not null default '', "status_code" integer not null, "client_ip" text not null, "location" text not null default '', "occurred_at" timestamptz not null, "event_hash" text not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "seo_traffic_visit_pkey" primary key ("id"));`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_seo_traffic_visit_event_hash_unique" ON "seo_traffic_visit" ("event_hash") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_traffic_visit_deleted_at" ON "seo_traffic_visit" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_traffic_visit_occurred_at_source" ON "seo_traffic_visit" ("occurred_at", "source") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_seo_traffic_visit_log_source_id" ON "seo_traffic_visit" ("log_source_id") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "seo_crawler_visit" cascade;`);

    this.addSql(`drop table if exists "seo_evidence" cascade;`);

    this.addSql(`drop table if exists "seo_monitor" cascade;`);

    this.addSql(`drop table if exists "seo_page" cascade;`);

    this.addSql(`drop table if exists "seo_provider" cascade;`);

    this.addSql(`drop table if exists "seo_run" cascade;`);

    this.addSql(`drop table if exists "seo_setting" cascade;`);

    this.addSql(`drop table if exists "seo_source" cascade;`);

    this.addSql(`drop table if exists "seo_submission" cascade;`);

    this.addSql(`drop table if exists "seo_traffic_visit" cascade;`);
  }

}
