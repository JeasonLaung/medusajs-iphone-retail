import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261023085008 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "customer_notice" drop constraint if exists "customer_notice_event_key_unique";`);
    this.addSql(`create table if not exists "customer_notice" ("id" text not null, "customer_id" text not null, "event_key" text not null, "kind" text not null, "resource_id" text not null, "reference" text not null, "action" text not null default '', "read_at" timestamptz null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "customer_notice_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_customer_notice_deleted_at" ON "customer_notice" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_customer_notice_event_key_unique" ON "customer_notice" ("event_key") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_customer_notice_customer_id_created_at" ON "customer_notice" ("customer_id", "created_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_customer_notice_customer_id_read_at" ON "customer_notice" ("customer_id", "read_at") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "customer_notice" cascade;`);
  }

}
