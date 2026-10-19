import { Migration } from "@medusajs/framework/mikro-orm/migrations"

export class Migration20261019093200 extends Migration {
  override async up(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"after_sales_request\" IS '订单售后申请：退货、换机和维修处理记录';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"id\" IS '记录主键';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"created_at\" IS '记录创建时间（带时区）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"updated_at\" IS '记录最后更新时间（带时区）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"deleted_at\" IS '软删除时间；NULL 表示未删除';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"order_id\" IS '关联订单 ID（order.id）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"customer_id\" IS '申请客户 ID（customer.id）';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"type\" IS '售后类型：return 退货；exchange 换机；repair 维修';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"reason\" IS '客户提交的售后原因';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"items\" IS '售后明细及处理上下文 JSON；lines 保存申请商品行和设备信息';")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"status\" IS '申请状态：pending 待处理；processing 处理中；completed 已完成；rejected 已拒绝';")
  }

  override async down(): Promise<void> {
    this.addSql("COMMENT ON TABLE \"public\".\"after_sales_request\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"created_at\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"updated_at\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"deleted_at\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"order_id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"customer_id\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"type\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"reason\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"items\" IS NULL;")
    this.addSql("COMMENT ON COLUMN \"public\".\"after_sales_request\".\"status\" IS NULL;")
  }
}

