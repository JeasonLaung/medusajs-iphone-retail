import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261023084730 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "buyback_price" add column if not exists "pricing_rules" jsonb null;`);
  }

  override async down(): Promise<void> {
    this.addSql(`alter table if exists "buyback_price" drop column if exists "pricing_rules";`);
  }

}
