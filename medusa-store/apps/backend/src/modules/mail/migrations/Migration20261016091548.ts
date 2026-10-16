import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261016091548 extends Migration {
  override async up(): Promise<void> {
    this.addSql(
      `alter table if exists "mail_message" drop constraint if exists "mail_message_provider_id_unique";`
    )
    this.addSql(
      `alter table if exists "mail_event" drop constraint if exists "mail_event_event_id_unique";`
    )
    this.addSql(
      `alter table if exists "mail_delivery" drop constraint if exists "mail_delivery_campaign_id_contact_id_unique";`
    )
    this.addSql(
      `alter table if exists "mail_contact" drop constraint if exists "mail_contact_unsubscribe_token_unique";`
    )
    this.addSql(
      `alter table if exists "mail_contact" drop constraint if exists "mail_contact_email_unique";`
    )
    this.addSql(
      `alter table if exists "mail_account" drop constraint if exists "mail_account_email_unique";`
    )
    this.addSql(
      `create table if not exists "mail_account" ("id" text not null, "name" text not null, "email" text not null, "reply_to" text not null default '', "enabled" boolean not null default true, "api_key" text not null default '', "webhook_secret" text not null default '', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mail_account_pkey" primary key ("id"));`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_account_deleted_at" ON "mail_account" ("deleted_at") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_account_email_unique" ON "mail_account" ("email") WHERE deleted_at IS NULL;`
    )

    this.addSql(
      `create table if not exists "mail_campaign" ("id" text not null, "name" text not null, "account_id" text not null, "subject" text not null, "text_body" text not null default '', "html_body" text not null default '', "status" text check ("status" in ('draft', 'queued', 'sending', 'paused', 'completed')) not null default 'draft', "created_by" text not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mail_campaign_pkey" primary key ("id"));`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_campaign_deleted_at" ON "mail_campaign" ("deleted_at") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_campaign_status" ON "mail_campaign" ("status") WHERE deleted_at IS NULL;`
    )

    this.addSql(
      `create table if not exists "mail_contact" ("id" text not null, "email" text not null, "name" text not null default '', "subscribed" boolean not null default false, "consent_source" text not null default '', "consent_at" timestamptz null, "suppressed" boolean not null default false, "unsubscribe_token" text not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mail_contact_pkey" primary key ("id"));`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_contact_deleted_at" ON "mail_contact" ("deleted_at") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_contact_email_unique" ON "mail_contact" ("email") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_contact_unsubscribe_token_unique" ON "mail_contact" ("unsubscribe_token") WHERE deleted_at IS NULL;`
    )

    this.addSql(
      `create table if not exists "mail_delivery" ("id" text not null, "campaign_id" text not null, "contact_id" text not null, "email" text not null, "status" text not null default 'queued', "provider_id" text null, "attempts" integer not null default 0, "error" text not null default '', "sent_at" timestamptz null, "opened_at" timestamptz null, "clicked_at" timestamptz null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mail_delivery_pkey" primary key ("id"));`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_delivery_deleted_at" ON "mail_delivery" ("deleted_at") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_delivery_campaign_id_contact_id_unique" ON "mail_delivery" ("campaign_id", "contact_id") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_delivery_campaign_id_status" ON "mail_delivery" ("campaign_id", "status") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_delivery_provider_id" ON "mail_delivery" ("provider_id") WHERE deleted_at IS NULL;`
    )

    this.addSql(
      `create table if not exists "mail_event" ("id" text not null, "event_id" text not null, "account_id" text not null, "type" text not null, "provider_id" text not null, "occurred_at" timestamptz not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mail_event_pkey" primary key ("id"));`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_event_deleted_at" ON "mail_event" ("deleted_at") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_event_event_id_unique" ON "mail_event" ("event_id") WHERE deleted_at IS NULL;`
    )

    this.addSql(
      `create table if not exists "mail_message" ("id" text not null, "account_id" text not null, "folder" text check ("folder" in ('inbox', 'draft', 'outbox', 'sent', 'archive', 'trash')) not null default 'draft', "previous_folder" text not null default 'inbox', "direction" text check ("direction" in ('inbound', 'outbound')) not null default 'outbound', "from_address" text not null default '', "to" jsonb not null default '[]', "cc" jsonb not null default '[]', "bcc" jsonb not null default '[]', "subject" text not null default '', "text_body" text not null default '', "html_body" text not null default '', "attachments" jsonb not null default '[]', "provider_id" text null, "message_id" text null, "in_reply_to" text null, "reply_to" jsonb not null default '[]', "read" boolean not null default true, "starred" boolean not null default false, "status" text not null default 'draft', "error" text not null default '', "attempts" integer not null default 0, "sent_at" timestamptz null, "received_at" timestamptz null, "created_by" text not null default '', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mail_message_pkey" primary key ("id"));`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_message_deleted_at" ON "mail_message" ("deleted_at") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_message_account_id_folder" ON "mail_message" ("account_id", "folder") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_message_provider_id_unique" ON "mail_message" ("provider_id") WHERE deleted_at IS NULL;`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_message_folder_read" ON "mail_message" ("folder", "read") WHERE deleted_at IS NULL;`
    )

    this.addSql(
      `create table if not exists "mail_template" ("id" text not null, "name" text not null, "subject" text not null default '', "text_body" text not null default '', "html_body" text not null default '', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mail_template_pkey" primary key ("id"));`
    )
    this.addSql(
      `CREATE INDEX IF NOT EXISTS "IDX_mail_template_deleted_at" ON "mail_template" ("deleted_at") WHERE deleted_at IS NULL;`
    )
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "mail_account" cascade;`)

    this.addSql(`drop table if exists "mail_campaign" cascade;`)

    this.addSql(`drop table if exists "mail_contact" cascade;`)

    this.addSql(`drop table if exists "mail_delivery" cascade;`)

    this.addSql(`drop table if exists "mail_event" cascade;`)

    this.addSql(`drop table if exists "mail_message" cascade;`)

    this.addSql(`drop table if exists "mail_template" cascade;`)
  }
}
