# 客户订单进度与包裹物流

继承原 `order-timeline`、`shipment-tracking` 与 `shipping-details` 的业务投影和页面位置。Astro 账户订单详情通过签名私有桥调用 `GET /api/store/customer/orders/:id/progress`；不使用 Medusa SDK，也不开放匿名订单号查询。

Gin 在当前客户会话/账户版本检查和同一数据库事务内，复用已完成订单的显式 `customer_order_ownership` 校验。邮箱相同不授权、购物车 cookie 不参与此读取；其他客户和已删除订单返回404，失效会话返回401。响应和桥均 private/no-store，拒绝查询参数与额外路径，不转发 Admin 或购物车 cookie。

订单进度沿用真实记录：下单、付款 capture（无 capture 时使用 captured_at）、退款、设备 assign/release/replace_allocation、包裹创建/发货/送达/取消与订单取消；按时间稳定排序，时间显示沿用 Asia/Shanghai。金额复用现有 scalar/raw 精确数值解析，保持字符串，不经浮点转换。包裹创建时间对应原页面 packed 节点；不捏造额外在途轨迹。

配送信息下面展示未取消且已发货包裹的原始物流单号和可选承运商链接。取消包裹仍留在进度中，软删除记录不展示；同一有效包裹绑定多个订单时拒绝整个读取。仅选择 tracking_number/tracking_url，过滤控制字符、非 HTTP(S)、凭证链接；不返回 label_url、配送提供商 payload、内部地址/仓库/操作者、设备事件 data、成本或 metadata。

沿用原双栏布局与中英文文案；Ant Steps/Tag 使用现有公共主题。进度与主订单读取独立，物流请求失败时仍能看订单并重试；重试重新请求实际 Gin。确认页匿名访问、商品清单、金额、支付与收货信息布局保持原行为。本批没有迁移单台设备售后选择、售后命令、外部承运商轨迹或发货通知，整个前台尚未验收完成。

验证入口：`retail/scripts/test-customer-orders-live.py` 在独立源码快照构建 Astro，使用带资源限制的自有 MySQL/Redis 容器和实际 Gin。合成记录覆盖有效/取消/软删除包裹、不安全链接和敏感字段；真实浏览器完成客户登录、订单关联、移除购物车 cookie 后读取，中英文390/1440宽度及网络失败重试。测试不产生真实付款、发货或邮件。
