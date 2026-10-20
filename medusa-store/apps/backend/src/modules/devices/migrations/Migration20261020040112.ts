import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261020040112 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "device" add column if not exists "category" text check ("category" in ('sale', 'returned', 'recycled')) not null default 'sale', add column if not exists "photos" jsonb not null default '[]';`);
    this.addSql(`alter table if exists "device" alter column "state" type text using ("state"::text);`);
    this.addSql(`alter table if exists "device" alter column "state" set default 'available';`);
    this.addSql(`update "device" d set "category" = 'returned' where d."state" in ('inspection', 'repair', 'repair_ready', 'supplier_returned') and exists (select 1 from "device_assignment" a where a."device_id" = d."id" and a."status" = 'returned' and a."deleted_at" is null);`);
  }

  override async down(): Promise<void> {
    this.addSql(`alter table if exists "device" drop column if exists "category", drop column if exists "photos";`);

    this.addSql(`alter table if exists "device" alter column "state" type text using ("state"::text);`);
    this.addSql(`alter table if exists "device" alter column "state" set default 'inspection';`);
  }

}
