import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261013092008 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "device_variant" drop constraint if exists "device_variant_inventory_item_id_unique";`);
    this.addSql(`alter table if exists "device_variant" drop constraint if exists "device_variant_variant_id_unique";`);
    this.addSql(`alter table if exists "device_supply" drop constraint if exists "device_supply_source_id_variant_id_unique";`);
    this.addSql(`alter table if exists "device_source" drop constraint if exists "device_source_location_id_unique";`);
    this.addSql(`alter table if exists "device_assignment" drop constraint if exists "device_assignment_device_id_unique";`);
    this.addSql(`alter table if exists "device" drop constraint if exists "device_imei_unique";`);
    this.addSql(`alter table if exists "device" drop constraint if exists "device_serial_number_unique";`);
    this.addSql(`create table if not exists "device" ("id" text not null, "serial_number" text null, "imei" text null, "variant_id" text not null, "source_id" text not null, "location_id" text not null, "grade" text not null, "battery_health" integer not null, "cost_cents" integer not null default 0, "currency_code" text not null default 'eur', "bin" text not null default '', "notes" text not null default '', "state" text check ("state" in ('inspection', 'available', 'reserved', 'packed', 'shipped', 'repair', 'repair_ready', 'retired', 'supplier_returned')) not null default 'inspection', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "device_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_deleted_at" ON "device" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_serial_number_unique" ON "device" ("serial_number") WHERE serial_number IS NOT NULL AND deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_imei_unique" ON "device" ("imei") WHERE imei IS NOT NULL AND deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_variant_id_location_id_state" ON "device" ("variant_id", "location_id", "state") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "device_assignment" ("id" text not null, "device_id" text not null, "order_id" text not null, "item_id" text not null, "source_id" text not null, "status" text check ("status" in ('reserved', 'packed', 'shipped', 'returned', 'canceled')) not null, "fulfillment_id" text null, "return_id" text null, "after_sales_request_id" text null, "replacement_of" text null, "tracking_number" text null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "device_assignment_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_assignment_deleted_at" ON "device_assignment" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_assignment_device_id_unique" ON "device_assignment" ("device_id") WHERE status IN ('reserved', 'packed', 'shipped') AND deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_assignment_order_id_item_id" ON "device_assignment" ("order_id", "item_id") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "device_event" ("id" text not null, "device_id" text null, "order_id" text null, "actor_id" text not null, "action" text not null, "data" jsonb not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "device_event_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_event_deleted_at" ON "device_event" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_event_device_id" ON "device_event" ("device_id") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_event_order_id" ON "device_event" ("order_id") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "device_source" ("id" text not null, "name" text not null, "type" text check ("type" in ('self', 'supplier')) not null, "location_id" text not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "device_source_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_source_deleted_at" ON "device_source" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_source_location_id_unique" ON "device_source" ("location_id") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "device_supply" ("id" text not null, "source_id" text not null, "variant_id" text not null, "quantity" integer not null default 0, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "device_supply_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_supply_deleted_at" ON "device_supply" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_supply_source_id_variant_id_unique" ON "device_supply" ("source_id", "variant_id") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "device_variant" ("id" text not null, "variant_id" text not null, "inventory_item_id" text not null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "device_variant_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_device_variant_deleted_at" ON "device_variant" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_variant_variant_id_unique" ON "device_variant" ("variant_id") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_variant_inventory_item_id_unique" ON "device_variant" ("inventory_item_id") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "device" cascade;`);

    this.addSql(`drop table if exists "device_assignment" cascade;`);

    this.addSql(`drop table if exists "device_event" cascade;`);

    this.addSql(`drop table if exists "device_source" cascade;`);

    this.addSql(`drop table if exists "device_supply" cascade;`);

    this.addSql(`drop table if exists "device_variant" cascade;`);
  }

}
