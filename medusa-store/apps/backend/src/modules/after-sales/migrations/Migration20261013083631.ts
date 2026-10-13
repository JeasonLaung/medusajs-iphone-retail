import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261013083631 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_after_sales_one_open_request" ON "after_sales_request" ("order_id") WHERE status IN ('pending', 'processing') AND deleted_at IS NULL;`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop index if exists "IDX_after_sales_one_open_request";`);
  }

}
