# 新客户的长期订单归属与列表

已有前台账号订单列表/详情、近期订单及邮件转移入口，来源为 medusa-store/apps/storefront/src/lib/data/orders.ts 与账户页面。本增量补齐 Gin 中新客户购买后的长期归属和订单列表，不替代原邮件转移，也不宣称账户订单页面已闭环。

新增0050 customer_order_ownership，将新 customer_accounts 与本次成单分开关联，保留 orders.customer_id 的历史引用语义。不会迁移旧登录账号，也不按邮箱匹配导入订单。

POST /api/store/customer/orders/:id/attach 接受空JSON，要求受Origin/客户CSRF保护的新客户会话和本次购物车possession Cookie。事务复用私有确认页的当前商店/渠道、购物车generation/active、checkout attempt、完成版本、唯一收据、订单及捕获款快照校验，锁定当前购物车，再锁定新客户并重查两种Redis凭证与账号启用/验证/版本。关联与审计同事务写入，相同账号重试返回同一结果；已归属其他客户拒绝。订单ID、同邮箱或旧客户ID本身没有认领能力。

GET /api/store/customer/orders?offset=0 只查询当前新账号的显式关联，每页20项。返回订单编号、日期、状态、币种、精确总额及保存的商品图片/名称/数量，不返回邮箱、地址、历史客户ID、付款定位、密文、metadata或raw金额。订单软删除后不展示。购物车凭证过期或撤销不会撤销已建立的长期关联；客户会话/版本撤销仍立即拒绝读取。新归属不会更改原订单收件邮箱、库存、付款或其他业务状态。

独立 HEAD 快照仅加入本次源码与0050及迁移数量断言，排除未提交0037，共49项迁移。go test ./...、隔离MySQL/Redis的OrderRead、CartRuntime与AdminAuthentication通过。新增真实Gin用例覆盖双凭证/Origin/CSRF、拒绝请求体客户ID、查询边界、同邮箱无订单、无效购物车拒绝、4个并发请求单次审计、末尾审计失败整体回滚、其他客户不能抢占、私有列表与精确158.75金额、软删除、购物车撤销后长期读取、客户版本撤销拒绝。证据见 customer-order-ownership-report.json。

[Astro签名桥、付款确认后关联及近期订单/列表/详情](astro-customer-orders.md)已接入并通过合成API浏览器验证；仍缺真实联合验收、邮件转移、售后与履约信息。当前只允许本次购物车凭证关联新成单；没有开放历史订单认领。本次未操作生产数据、部署服务器、真实支付或邮件发送。
