# 公共商品可售库存观察

2026-10-28。Gin 公共商品列表和详情在原有只读 Repeatable Read 事务中增加 inventory_quantity、manage_inventory、allow_backorder 和 availability；成功响应 availability=true，checkout=false。默认观察数量 1；显式 quantity 继续要求 region_id，采用现有规范正整数验证。此接口不预留库存，也不授权下单。

仅使用服务端店铺销售渠道关联的有效仓库、有效库存项及当前变体库存关联。按 required_quantity 精确换算整件数，不使用浮点数。inventory_quantity 为各库存项跨健康仓库汇总后换算整件数的最小值；availability.max_fulfillable_quantity 则为同一个仓库同时满足全部库存项时的最大件数。后者与当前预留组件的一行一个仓库约束一致，不能用汇总数量承诺自动拆仓。

每个库存项/仓库重新核对有效预留合计与 reserved_quantity。持久迁移 hold、负预留、负库存/预留字段、实时合计不一致或超过单位置 10000 条预留均排除该位置；不清除历史差异，允许其他健康位置继续参与。正常背单导致 reserved 超过 stocked 时将余量截为零，仍可在健康位置按 allow_backorder 接受观察数量。已冻结的负余额仍由 hold 排除，背单不能绕过。

完整且不跟踪库存的变体为 untracked，可满足观察数量，两个数量字段均 null。不完整配置为 incomplete，不可满足。跟踪库存但无关联、无有效渠道仓库、无效 required_quantity 或库存项已删除时为 unavailable，数量未知 null。存在有效关系时返回精确整数字符串，包括真实零；没有所有库存项共同健康的仓库仍为 unavailable，即使 pooled 数量大于零。健康仓库不足本次数量时为 out_of_stock，允许背单则为 backorder；充足时为 available。

读取需通过正式库存模型就绪检查；一次最多 1000 个渠道仓库，仓库、库存关联、库存层、预留和不同 hold 位置共同最多 20000 行。超限整体 503，不截断计算。内部库存项/仓库标识、required_quantity、预留/在途数量、差异原因及 metadata 不公开。错误 no-store，成功仍按公共查询重验证。

[人工 SQL 报告](admin-auth-report.json) store_availability 覆盖渠道隔离、精确小数换算、共享库存、组合库存共同仓库、背单/不跟踪、不完整配置、删除过滤、持久 hold、实时对账、负预留、行数限制和读取指纹。纯函数另验超过 JS 安全整数的精度及跨仓汇总与单仓差异。[固定来源报告](catalog-mysql-rehearsal-report.json) 逐个公开变体用独立 SQL 对账与 big.Rat 算法核验响应；它验证新保护规则，不虚称冻结异常位置后的数量与旧无保护 helper 完全相同。[真实进程报告](admin-live-report.json) 回归公开接口和 Ant 后台，并验证 Astro SSR 运行。

购物车、库存实际预留与价格锁定、结账、客户认证、税费/配送/优惠、Astro 业务页面仍未完成，整个商城迁移继续进行。后台组件优先 Ant Design，图表统一 ECharts，前台继续保留现有页面设计。

本轮固定来源实际核验 30 商品、1558 变体：767 available、47 out_of_stock、714 unavailable、30 incomplete。未知库存保持未知，不将其改写为可购买。完整业务数据读取前后指纹相同，4 条历史库存 hold 保留。Go test/vet、独占 MySQL/Redis 边界、真实浏览器链路、19 项 UI 测试和 9 张中英三宽截图回归均通过；旧三个开发地址均 200。后台日志无新增错误，storefront 仍有既存 Fast Refresh 全量重载告警。
