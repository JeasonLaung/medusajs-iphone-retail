import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261019093500 extends Migration {
  override async up(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"order_number_clock\" IS '订单展示编号的持久化逻辑时钟，通过行锁保证并发编号唯一';")
    this.addSql("COMMENT ON COLUMN \"public\".\"order_number_clock\".\"id\" IS '单行时钟标识，固定为 true';")
    this.addSql("COMMENT ON COLUMN \"public\".\"order_number_clock\".\"last_tick\" IS '最后分配的逻辑时钟值；每个 tick 为 10 微秒，从 Unix 纪元起计；订单展示编号使用上海时区';")
  }

  override async down(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"order_number_clock\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"order_number_clock\".\"id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"order_number_clock\".\"last_tick\" IS NULL;")
  }
}

