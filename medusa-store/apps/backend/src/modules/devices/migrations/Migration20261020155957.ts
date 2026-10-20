import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261020155957 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "device" drop constraint if exists "device_state_check";`);

    this.addSql(`alter table if exists "device" alter column "state" type text using ("state"::text);`);
    this.addSql(`alter table if exists "device" alter column "state" set default 'pending';`);
    this.addSql(`alter table if exists "device" add constraint "device_state_check" check("state" in ('pending', 'inspection', 'available', 'reserved', 'packed', 'shipped', 'repair', 'repair_ready', 'retired', 'supplier_returned'));`);
  }

  override async down(): Promise<void> {
    this.addSql(`update "device" set "state" = 'inspection' where "state" = 'pending';`);
    this.addSql(`alter table if exists "device" drop constraint if exists "device_state_check";`);

    this.addSql(`alter table if exists "device" alter column "state" type text using ("state"::text);`);
    this.addSql(`alter table if exists "device" alter column "state" set default 'available';`);
    this.addSql(`alter table if exists "device" add constraint "device_state_check" check("state" in ('inspection', 'available', 'reserved', 'packed', 'shipped', 'repair', 'repair_ready', 'retired', 'supplier_returned'));`);
  }

}
