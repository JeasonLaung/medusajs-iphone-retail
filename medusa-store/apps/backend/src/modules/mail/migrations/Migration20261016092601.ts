import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261016092601 extends Migration {
  override async up(): Promise<void> {
    this.addSql(
      `alter table if exists "mail_delivery" add column if not exists "first_attempt_at" timestamptz null;`
    )

    this.addSql(
      `alter table if exists "mail_message" add column if not exists "first_attempt_at" timestamptz null;`
    )
  }

  override async down(): Promise<void> {
    this.addSql(
      `alter table if exists "mail_delivery" drop column if exists "first_attempt_at";`
    )

    this.addSql(
      `alter table if exists "mail_message" drop column if exists "first_attempt_at";`
    )
  }
}
