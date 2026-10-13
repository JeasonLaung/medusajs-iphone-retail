import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261013083026 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`create table if not exists "after_sales_request" ("id" text not null, "order_id" text not null, "customer_id" text not null, "type" text check ("type" in ('return', 'exchange', 'repair')) not null, "reason" text not null, "items" jsonb not null, "status" text check ("status" in ('pending', 'processing', 'completed', 'rejected')) not null default 'pending', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "after_sales_request_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_after_sales_request_deleted_at" ON "after_sales_request" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_after_sales_request_order_id" ON "after_sales_request" ("order_id") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "after_sales_request" cascade;`);
  }

}
