import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261104080459 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "mail_account" add column if not exists "provider" text check ("provider" in ('resend', 'relayme')) not null default 'resend', add column if not exists "api_url" text not null default '', add column if not exists "relayme_key_id" text not null default '', add column if not exists "imap_host" text not null default '', add column if not exists "imap_port" integer not null default 993, add column if not exists "imap_username" text not null default '', add column if not exists "imap_password" text not null default '', add column if not exists "imap_tls" text check ("imap_tls" in ('tls', 'starttls')) not null default 'tls';`);

    this.addSql(`alter table if exists "mail_event" add column if not exists "data" jsonb not null default '{}';`);
  }

  override async down(): Promise<void> {
    this.addSql(`alter table if exists "mail_account" drop column if exists "provider", drop column if exists "api_url", drop column if exists "relayme_key_id", drop column if exists "imap_host", drop column if exists "imap_port", drop column if exists "imap_username", drop column if exists "imap_password", drop column if exists "imap_tls";`);

    this.addSql(`alter table if exists "mail_event" drop column if exists "data";`);
  }

}
