import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261021142941 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`create table if not exists "webhook_endpoint" ("id" text not null, "name" text not null, "provider" text check ("provider" in ('feishu', 'wecom', 'dingtalk', 'slack', 'mattermost', 'generic')) not null, "url" text not null, "secret" text not null default '', "token" text not null default '', "keyword" text not null default '', "enabled" boolean not null default true, "events" jsonb not null, "language" text check ("language" in ('zh', 'en')) not null default 'zh', "last_status" text not null default '', "last_sent_at" timestamptz null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "webhook_endpoint_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_webhook_endpoint_deleted_at" ON "webhook_endpoint" ("deleted_at") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "webhook_endpoint" cascade;`);
  }

}
