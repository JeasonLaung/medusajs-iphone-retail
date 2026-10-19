import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261019063454 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "product_review" add column if not exists "order_id" text null, add column if not exists "verified_purchase" boolean not null default false;`);
  }

  override async down(): Promise<void> {
    this.addSql(`alter table if exists "product_review" drop column if exists "order_id", drop column if exists "verified_purchase";`);
  }

}
