import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261023032921 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "buyback_request" drop constraint if exists "buyback_request_customer_id_submission_key_unique";`);
    this.addSql(`alter table if exists "buyback_price" drop constraint if exists "buyback_price_product_id_capacity_unique";`);
    this.addSql(`create table if not exists "buyback_photo" ("id" text not null, "request_id" text not null, "item_id" text not null, "photo_type" text check ("photo_type" in ('front', 'back', 'edges', 'damage', 'packaging', 'sealed')) not null, "uploaded_at" timestamptz not null, "storage_key" text not null, "mime_type" text not null, "review_status" text check ("review_status" in ('pending', 'approved', 'retake', 'expired')) not null default 'pending', "review_note" text not null default '', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "buyback_photo_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_buyback_photo_deleted_at" ON "buyback_photo" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_buyback_photo_request_id_item_id" ON "buyback_photo" ("request_id", "item_id") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "buyback_price" ("id" text not null, "product_id" text not null, "capacity" text not null, "amount" integer not null, "currency_code" text not null default 'eur', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "buyback_price_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_buyback_price_deleted_at" ON "buyback_price" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_buyback_price_product_id_capacity_unique" ON "buyback_price" ("product_id", "capacity") WHERE deleted_at IS NULL;`);

    this.addSql(`create table if not exists "buyback_request" ("id" text not null, "customer_id" text not null, "submission_key" text not null, "contact_name" text not null, "email" text not null, "phone" text not null, "return_address" jsonb not null, "items" jsonb not null, "status" text check ("status" in ('awaiting_photos', 'photo_review', 'ready_to_ship', 'in_transit', 'received', 'final_offer', 'accepted', 'paid', 'return_requested', 'returning', 'returned', 'canceled')) not null default 'awaiting_photos', "reference_amount" integer null, "final_amount" integer null, "currency_code" text not null default 'eur', "shipping_instructions" text not null default '', "tracking_number" text not null default '', "return_tracking" text not null default '', "payment_reference" text not null default '', "review_note" text not null default '', "history" jsonb not null default '[]', "closed_at" timestamptz null, "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "buyback_request_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_buyback_request_deleted_at" ON "buyback_request" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_buyback_request_customer_id_created_at" ON "buyback_request" ("customer_id", "created_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_buyback_request_status_created_at" ON "buyback_request" ("status", "created_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_buyback_request_customer_id_submission_key_unique" ON "buyback_request" ("customer_id", "submission_key") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "buyback_photo" cascade;`);

    this.addSql(`drop table if exists "buyback_price" cascade;`);

    this.addSql(`drop table if exists "buyback_request" cascade;`);
  }

}
