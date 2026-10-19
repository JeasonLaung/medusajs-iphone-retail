import { Migration } from "@medusajs/framework/mikro-orm/migrations";

export class Migration20261019094350 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "device" drop constraint if exists "device_imei2_unique";`);
    this.addSql(`alter table if exists "device" add column if not exists "imei2" text null;`);
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_imei2_unique" ON "device" ("imei2") WHERE imei2 IS NOT NULL AND deleted_at IS NULL;`);
    this.addSql(`COMMENT ON COLUMN "device"."imei" IS '设备号码一：15 位 IMEI 或 32 位 EID；按文本保存，两个号码栏之间不可重复';`);
    this.addSql(`COMMENT ON COLUMN "device"."imei2" IS '设备号码二：15 位 IMEI 或 32 位 EID；可空，按文本保存，两个号码栏之间不可重复';`);
  }

  override async down(): Promise<void> {
    this.addSql(`drop index if exists "IDX_device_imei2_unique";`);
    this.addSql(`alter table if exists "device" drop column if exists "imei2";`);
    this.addSql(`COMMENT ON COLUMN "device"."imei" IS '设备 IMEI；按文本保存，未删除设备中非空值唯一';`);
  }

}
