import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261019073421 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "device" alter column "grade" type text using ("grade"::text);`);
    this.addSql(`alter table if exists "device" alter column "grade" drop not null;`);
    this.addSql(`alter table if exists "device" alter column "battery_health" type integer using ("battery_health"::integer);`);
    this.addSql(`alter table if exists "device" alter column "battery_health" drop not null;`);
  }

  override async down(): Promise<void> {
    this.addSql(`alter table if exists "device" alter column "grade" type text using ("grade"::text);`);
    this.addSql(`alter table if exists "device" alter column "grade" set not null;`);
    this.addSql(`alter table if exists "device" alter column "battery_health" type integer using ("battery_health"::integer);`);
    this.addSql(`alter table if exists "device" alter column "battery_health" set not null;`);
  }

}
