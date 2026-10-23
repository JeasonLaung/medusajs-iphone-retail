import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261023034048 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "buyback_price" add column if not exists "raw_amount" jsonb not null;`);
    this.addSql(`alter table if exists "buyback_price" alter column "amount" type numeric using ("amount"::numeric);`);

    this.addSql(`alter table if exists "buyback_request" add column if not exists "raw_reference_amount" jsonb null, add column if not exists "raw_final_amount" jsonb null;`);
    this.addSql(`alter table if exists "buyback_request" alter column "reference_amount" type numeric using ("reference_amount"::numeric);`);
    this.addSql(`alter table if exists "buyback_request" alter column "final_amount" type numeric using ("final_amount"::numeric);`);
  }

  override async down(): Promise<void> {
    this.addSql(`alter table if exists "buyback_price" drop column if exists "raw_amount";`);

    this.addSql(`alter table if exists "buyback_price" alter column "amount" type real using ("amount"::real);`);

    this.addSql(`alter table if exists "buyback_request" drop column if exists "raw_reference_amount", drop column if exists "raw_final_amount";`);

    this.addSql(`alter table if exists "buyback_request" alter column "reference_amount" type real using ("reference_amount"::real);`);
    this.addSql(`alter table if exists "buyback_request" alter column "final_amount" type real using ("final_amount"::real);`);
  }

}
