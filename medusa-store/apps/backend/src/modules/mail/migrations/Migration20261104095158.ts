import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261104095158 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "mail_contact" add column if not exists "notes" text not null default '';`);
  }

  override async down(): Promise<void> {
    this.addSql(`alter table if exists "mail_contact" drop column if exists "notes";`);
  }

}
