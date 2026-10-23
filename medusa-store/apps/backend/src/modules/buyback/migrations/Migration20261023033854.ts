import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261023033854 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "buyback_device_link" drop constraint if exists "buyback_device_link_device_id_unique";`);
    this.addSql(`alter table if exists "buyback_device_link" drop constraint if exists "buyback_device_link_item_id_unique";`);
    this.addSql(`create table if not exists "buyback_device_link" ("id" text not null, "request_id" text not null, "item_id" text not null, "device_id" text not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "buyback_device_link_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_buyback_device_link_deleted_at" ON "buyback_device_link" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_buyback_device_link_item_id_unique" ON "buyback_device_link" ("item_id") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_buyback_device_link_device_id_unique" ON "buyback_device_link" ("device_id") WHERE deleted_at IS NULL;`);

    this.addSql(`alter table if exists "buyback_price" alter column "amount" type real using ("amount"::real);`);

    this.addSql(`alter table if exists "buyback_request" alter column "reference_amount" type real using ("reference_amount"::real);`);
    this.addSql(`alter table if exists "buyback_request" alter column "final_amount" type real using ("final_amount"::real);`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "buyback_device_link" cascade;`);

    this.addSql(`alter table if exists "buyback_price" alter column "amount" type integer using ("amount"::integer);`);

    this.addSql(`alter table if exists "buyback_request" alter column "reference_amount" type integer using ("reference_amount"::integer);`);
    this.addSql(`alter table if exists "buyback_request" alter column "final_amount" type integer using ("final_amount"::integer);`);
  }

}
