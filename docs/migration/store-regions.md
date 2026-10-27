# Gin 公共区域与国家

2026-10-28。替换旧 `Regions` 对 source payload 的解码，新增 GET `/api/store/regions/{id}`，为 Astro 路由、国家选择与区域币种提供正式关系读取。属于 T020/T021/T025 的局部实现；完整公共商品、价格选择和 Astro 页面仍在迁移。

## 明确公开字段

列表返回 regions/count；单条返回 region。Region 仅有 id、name、currency_code、automatic_taxes、countries。Country 仅有 iso_2、iso_3、num_code、name、display_name、region_id；数字代码保留字符串及前导零。metadata、payload、时间、支付服务配置及认证信息不返回。

从 commerce_regions 与 commerce_countries 读取；所有软删除区域/国家排除，未分配国家及属于已删除区域的国家不进入公开目录。地区按原 ID、国家按 iso_2 稳定排序；空列表及空 countries 为 []。币种必须是小写三字母代码、路由国家代码必须是小写两字母；错误配置整体 503，不默认补 EUR 或修改来源。

每个请求使用只读 Repeatable Read，并验证正式 catalog 模型。目录最多 100 个活动区域及 1000 个关联国家，超限整体 503；单条读取不受其他区域的数量影响。未知/重复/损坏查询参数均 400；缺失或已删除单条 404；结构或依赖问题 503。成功 `public, max-age=0, must-revalidate`，失败 `no-store`；不读取登录 cookie、设置会话或按 Cookie 改变结果。

## 证据

[独占 MySQL 测试](admin-auth-report.json) 的 store_regions 使用实际完整 Gin 路由，覆盖无登录及无效 cookie、软删除/未分配过滤、多个币种、空数组、精确代码、列表/详情一致、未知查询、大小写错误、区域/国家超限、模型禁用及读取前后指纹一致。仅改人工 fixture，未操作应用业务库。

[固定来源整合](catalog-mysql-rehearsal-report.json) 逐字段核对实际 1 个区域、7 个已分配国家；来源 250 条国家记录仍完整导入，未分配记录不会因此公开或删除。既有全域 375 项引用差异及业务指纹保留。

[Astro 运行验证](astro-runtime.md) 使用新的生产 Node adapter 连接独占 Gin 的非空区域数据，无 Medusa 或 Next 运行依赖。区域公开不表示其他公共目录接口已迁移，亦不替代价格、税、购物车和渠道规则。
