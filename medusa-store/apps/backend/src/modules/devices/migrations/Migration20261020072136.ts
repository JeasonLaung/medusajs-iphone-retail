import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261020072136 extends Migration {

  override async up(): Promise<void> {
    this.addSql(`alter table if exists "device" drop constraint if exists "device_eid_unique"`)
    this.addSql(`alter table if exists "device" add column if not exists "eid" text null;`)
    // Never discard a legacy second EID: require manual reconciliation if present.
    this.addSql(`DO $$ BEGIN IF EXISTS (SELECT 1 FROM "device" WHERE length(imei) = 32 AND length(imei2) = 32 AND imei <> imei2) THEN RAISE EXCEPTION 'Device has two legacy EIDs; reconcile before migration'; END IF; END $$;`)
    this.addSql(`UPDATE "device" SET eid = CASE WHEN length(imei) = 32 THEN imei WHEN length(imei2) = 32 THEN imei2 ELSE eid END, imei = CASE WHEN length(imei) = 32 THEN NULL ELSE imei END, imei2 = CASE WHEN length(imei2) = 32 THEN NULL ELSE imei2 END WHERE length(imei) = 32 OR length(imei2) = 32;`)
    this.addSql(`COMMENT ON COLUMN "device"."imei" IS 'IMEI 1：15 位国际移动设备识别码';`)
    this.addSql(`COMMENT ON COLUMN "device"."imei2" IS 'IMEI 2：15 位国际移动设备识别码';`)
    this.addSql(`COMMENT ON COLUMN "device"."eid" IS 'EID：32 位 eSIM 标识，每台设备最多一个';`)
    this.addSql(`CREATE UNIQUE INDEX IF NOT EXISTS "IDX_device_eid_unique" ON "device" ("eid") WHERE eid IS NOT NULL AND deleted_at IS NULL;`)
  }

  override async down(): Promise<void> {
    this.addSql(`DO $$ BEGIN IF EXISTS (SELECT 1 FROM "device" WHERE eid IS NOT NULL AND imei IS NOT NULL AND imei2 IS NOT NULL) THEN RAISE EXCEPTION 'Cannot downgrade devices containing two IMEIs and an EID'; END IF; END $$;`)
    this.addSql(`UPDATE "device" SET imei = CASE WHEN imei IS NULL THEN eid ELSE imei END, imei2 = CASE WHEN imei IS NOT NULL AND imei2 IS NULL THEN eid ELSE imei2 END WHERE eid IS NOT NULL;`)
    this.addSql(`drop index if exists "IDX_device_eid_unique"`)
    this.addSql(`alter table if exists "device" drop column if exists "eid"`)
  }

}
