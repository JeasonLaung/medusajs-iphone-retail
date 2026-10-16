import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261016090000 extends Migration {
  override async up(): Promise<void> {
    this.addSql(`
      CREATE TABLE "order_number_clock" (
        "id" boolean PRIMARY KEY DEFAULT true CHECK (id),
        "last_tick" bigint NOT NULL
      );
      INSERT INTO "order_number_clock" (id, last_tick) VALUES (true, 0);

      CREATE FUNCTION assign_timestamp_order_number() RETURNS trigger
      LANGUAGE plpgsql AS $$
      DECLARE
        tick bigint;
      BEGIN
        -- A tick is 100 microseconds. The row lock serializes all writers,
        -- including separate processes, and survives application restarts.
        -- Advance the logical clock on collisions, clock rollback or overflow.
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

      CREATE TRIGGER order_timestamp_number
      BEFORE INSERT ON "order"
      FOR EACH ROW EXECUTE FUNCTION assign_timestamp_order_number();
    `)
  }

  override async down(): Promise<void> {
    this.addSql(`
      DROP TRIGGER IF EXISTS order_timestamp_number ON "order";
      DROP FUNCTION IF EXISTS assign_timestamp_order_number();
      DROP TABLE IF EXISTS "order_number_clock";
    `)
  }
}
