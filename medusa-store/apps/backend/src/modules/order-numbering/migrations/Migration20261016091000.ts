import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261016091000 extends Migration {
  override async up(): Promise<void> {
    // Preserve the clock's instant while changing from 100us to 10us ticks.
    this.addSql(`
      UPDATE "order_number_clock" SET last_tick = last_tick * 10;
      CREATE OR REPLACE FUNCTION assign_timestamp_order_number() RETURNS trigger
      LANGUAGE plpgsql AS $$
      DECLARE
        tick bigint;
      BEGIN
        UPDATE "order_number_clock"
        SET last_tick = GREATEST(
          last_tick + 1,
          floor(extract(epoch FROM clock_timestamp()) * 100000)::bigint
        )
        WHERE id = true
        RETURNING last_tick INTO STRICT tick;

        NEW.custom_display_id :=
          to_char(to_timestamp((tick / 100000)::double precision)
            AT TIME ZONE 'Asia/Shanghai', 'YYYYMMDDHH24MISS')
          || lpad((tick % 100000)::text, 5, '0');
        RETURN NEW;
      END;
      $$;
    `)
  }

  override async down(): Promise<void> {
    this.addSql(`
      UPDATE "order_number_clock" SET last_tick = (last_tick + 9) / 10;
      CREATE OR REPLACE FUNCTION assign_timestamp_order_number() RETURNS trigger
      LANGUAGE plpgsql AS $$
      DECLARE
        tick bigint;
      BEGIN
        UPDATE "order_number_clock"
        SET last_tick = GREATEST(
          last_tick + 1,
          floor(extract(epoch FROM clock_timestamp()) * 10000)::bigint
        )
        WHERE id = true
        RETURNING last_tick INTO STRICT tick;
        NEW.custom_display_id :=
          to_char(to_timestamp((tick / 10000)::double precision)
            AT TIME ZONE 'Asia/Shanghai', 'YYYYMMDDHH24MISS')
          || lpad((tick % 10000)::text, 4, '0');
        RETURN NEW;
      END;
      $$;
    `)
  }
}
