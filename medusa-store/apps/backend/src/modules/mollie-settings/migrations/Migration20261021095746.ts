import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261021095746 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`create table if not exists "mollie_config" ("id" text not null, "environment" text check ("environment" in ('test', 'production')) not null default 'test', "api_key" text not null default '', "redirect_url" text not null default '', "enabled" boolean not null default false, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "mollie_config_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_mollie_config_deleted_at" ON "mollie_config" ("deleted_at") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "mollie_config" cascade;`);
  }

}
