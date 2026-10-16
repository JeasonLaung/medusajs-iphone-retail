import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261016093527 extends Migration {
  override async up(): Promise<void> {
    this.addSql(
      `alter table if exists "mail_message" drop constraint if exists "mail_message_account_id_provider_id_unique";`
    )
    this.addSql(`drop index if exists "IDX_mail_message_provider_id_unique";`)

    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_message_account_id_provider_id_unique" ON "mail_message" ("account_id", "provider_id") WHERE deleted_at IS NULL;`
    )
  }

  override async down(): Promise<void> {
    this.addSql(
      `drop index if exists "IDX_mail_message_account_id_provider_id_unique";`
    )

    this.addSql(
      `CREATE UNIQUE INDEX IF NOT EXISTS "IDX_mail_message_provider_id_unique" ON "mail_message" ("provider_id") WHERE deleted_at IS NULL;`
    )
  }
}
