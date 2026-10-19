import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261019093300 extends Migration {
  override async up(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"product_review\" IS '商品评价及审核记录';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"id\" IS '记录主键';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"created_at\" IS '记录创建时间（带时区）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"updated_at\" IS '记录最后更新时间（带时区）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"deleted_at\" IS '软删除时间；NULL 表示未删除';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"product_id\" IS '被评价商品 ID（product.id）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"customer_id\" IS '评价客户 ID（customer.id）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"order_id\" IS '关联购买订单 ID（order.id），可为空';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"verified_purchase\" IS '是否已验证客户购买过该商品';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"author\" IS '评价展示的作者名称';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"rating\" IS '评价星级';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"title\" IS '评价标题';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"content\" IS '评价正文';")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"status\" IS '审核状态：pending 待审核；approved 已通过；rejected 已拒绝';")
  }

  override async down(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"product_review\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"created_at\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"updated_at\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"deleted_at\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"product_id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"customer_id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"order_id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"verified_purchase\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"author\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"rating\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"title\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"content\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"product_review\".\"status\" IS NULL;")
  }
}

