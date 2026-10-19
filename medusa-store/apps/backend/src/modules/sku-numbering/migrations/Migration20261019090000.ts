import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261019090000 extends Migration {
  override async up(): Promise<void> {
    this.addSql(`
      LOCK TABLE "product_variant" IN SHARE ROW EXCLUSIVE MODE;
      CREATE TABLE "sku_number_counter" (
        "id" boolean PRIMARY KEY DEFAULT true CHECK (id),
        "last_id" bigint NOT NULL DEFAULT 0 CHECK (last_id >= 0)
      );
      INSERT INTO "sku_number_counter" (id, last_id)
      SELECT true, COALESCE(MAX(CASE WHEN sku ~ '^ERP[0-9]{5,18}$'
        THEN GREATEST(substring(sku FROM 4)::bigint - 10000, 0) ELSE 0 END), 0)
      FROM "product_variant";

      CREATE FUNCTION assign_erp_variant_sku() RETURNS trigger
      LANGUAGE plpgsql AS $$
      DECLARE
        sequence_id bigint;
      BEGIN
        IF NEW.sku IS NULL OR NEW.sku ~ '^[[:space:]]*$' THEN
          -- Clearing an existing generated SKU retains its identity.
          IF TG_OP = 'UPDATE' AND OLD.sku ~ '^ERP[0-9]{5,18}$'
            AND substring(OLD.sku FROM 4)::bigint > 10000 THEN
            NEW.sku := OLD.sku;
            RETURN NEW;
          END IF;
          -- The persistent row lock serializes concurrent writers.
          UPDATE "sku_number_counter" SET last_id = last_id + 1
            WHERE id = true RETURNING last_id INTO STRICT sequence_id;
          NEW.sku := 'ERP' || (sequence_id + 10000)::text;
        ELSIF NEW.sku ~ '^ERP[0-9]{5,18}$'
          AND substring(NEW.sku FROM 4)::bigint > 10000 THEN
          -- Explicit ERP codes reserve their number for future allocation.
          UPDATE "sku_number_counter"
            SET last_id = GREATEST(last_id, substring(NEW.sku FROM 4)::bigint - 10000)
            WHERE id = true;
        END IF;
        RETURN NEW;
      END;
      $$;

      CREATE TRIGGER product_variant_erp_sku
      BEFORE INSERT OR UPDATE OF sku ON "product_variant"
      FOR EACH ROW EXECUTE FUNCTION assign_erp_variant_sku();
    `)
  }

  override async down(): Promise<void> {
    this.addSql(`
      DROP TRIGGER IF EXISTS product_variant_erp_sku ON "product_variant";
      DROP FUNCTION IF EXISTS assign_erp_variant_sku();
      DROP TABLE IF EXISTS "sku_number_counter";
    `)
  }
}
