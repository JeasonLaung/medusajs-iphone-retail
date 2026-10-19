import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261019034317 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "product_review" drop constraint if exists "product_review_product_id_customer_id_unique";`);
    this.addSql(`create table if not exists "product_review" ("id" text not null, "product_id" text not null, "customer_id" text not null, "author" text not null, "rating" integer not null, "title" text not null, "content" text not null, "status" text check ("status" in ('pending', 'approved', 'rejected')) not null default 'pending', "created_at" timestamptz not null default now(), "updated_at" timestamptz not null default now(), "deleted_at" timestamptz null, constraint "product_review_pkey" primary key ("id"));`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_product_review_deleted_at" ON "product_review" ("deleted_at") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_product_review_product_id_customer_id_unique" ON "product_review" ("product_id", "customer_id") WHERE deleted_at IS NULL;`);
    this.addSql(`CREATE INDEX IF NOT EXISTS "IDX_product_review_product_id_status" ON "product_review" ("product_id", "status") WHERE deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop table if exists "product_review" cascade;`);
  }

}
