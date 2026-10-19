import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261019093000 extends Migration {
  override async up(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"sku_number_counter\" IS 'ERP 商品规格 SKU 的持久化全局编号计数器';")
    this.addSql("COMMENT ON COLUMN \"public\".\"sku_number_counter\".\"id\" IS '单行计数器标识，固定为 true';")
    this.addSql("COMMENT ON COLUMN \"public\".\"sku_number_counter\".\"last_id\" IS '最后保留或分配的序号；生成 SKU 为 ERP 加上该序号与 10000 的和';")
  }

  override async down(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"sku_number_counter\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"sku_number_counter\".\"id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"sku_number_counter\".\"last_id\" IS NULL;")
  }
}

