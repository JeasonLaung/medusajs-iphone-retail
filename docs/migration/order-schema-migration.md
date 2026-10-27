# 订单正式证据结构

2026-10-28。0016 将订单、地址、成交商品快照、版本数量、税费/折扣、运费版本、
汇总、交易/抵扣、变更、索赔、换货、退回和历史客户主体共 29 表、390 个来源字段
接入显式 MySQL 迁移计划。此交付是订单领域的数据结构基础，尚未实现订单 API 或结账。

## 来源与保留规则

生成器 [generate-order-schema.py](../../retail/scripts/generate-order-schema.py) 从已有
[全字段计划](../../retail/backend/internal/rowimport/field_plans.json) 与
[审查版 DDL](mysql-relational-review.sql) 提取本领域结构，校验物理外键的父表已经存在。
`python retail/scripts/generate-order-schema.py --check` 可重现并拒绝生成物漂移。
产物 [schema_plan.json](../../retail/backend/internal/orders/schema_plan.json) 记录范围、
模型哈希、未解析语义依赖、暂缓索引与五项显式枚举约束。

PostgreSQL 原生枚举不一定在 pg_constraint 中有 CHECK 记录；审查版 DDL 中相应
VARCHAR 不能单独限制允许值。0016 按已审查字段 allowlist 增加 orders.status、
order_changes.status、order_claims.type、order_claim_items.reason、returns.status
的 CHECK；不能把付款成功等同于把订单 status 写成 paid。

保留 order_line_items 与 order_items 的区别，后者按 order/version 记数量和价格，
前者保留成交时商品/变体快照。保留 nullable totals_id 的循环外键关系；未来导入需
先插入 NULL 关联的快照、插入版本行，再在同一事务填回原 totals_id，且校验最终精确
目标与回执，不能关闭 FK 或提交半成品。当前没有实现该订单导入器。

金额/数量为 DECIMAL(38,18)，raw_* 独立保留；展示编号、SKU、自定义单号不能经由
JavaScript Number。没有用现价或现商品覆盖历史快照。保留原软删和 UTC 微秒时间。

historical_customer_subjects 只承接业务主体。legacy_had_account 是历史事实，
不是新登录权限。无密码、令牌、会话字段；受保护 metadata 为 LONGBLOB，仍需要
专用审查与密封入口，不能直接导入旧账号资料，也不能凭相同邮箱自动关联新账号。
客户 email 完整唯一登记仍为 hold，不能用前缀唯一索引或恢复旧 has_account 规则代替。

## 迁移与运行边界

应用 0016 前，guard 要求全部目标表及 order_runtime_schema 尚不存在。
遇到任意同名原型表先拒绝，不 DROP、RENAME 或覆盖其数据。失败后的 DDL 步骤仍由
已有迁移 journal 拒绝自动猜测恢复；需要核对实际执行状态。

orders.Ready 已接入 Gin 启动与 /health：要求明确版本/模型哈希、reads_enabled=true、
writes_enabled=false；常规写能力受 CHECK 强制关闭。模型 ready 仅表示结构可用，
不授予任何记录权限，不表示历史数据已导入或整个订单领域已验收。

order_carts、order_payment_collections、order_fulfillments 和 return_fulfillments 的
跨域语义父对象尚未全部接入，当前只有保留结构。原复合键不等于“每个 cart 只能成一单”；
未来 checkout 仍需额外持久唯一约束、当前归属与版本验证及恢复状态。
原全域 375 项缺失引用（包含交易 256 项）继续保留且未修复。不能伪造父对象、删除历史
关联或把未解析关联当作授权依据。

## 验证与后续

[实际 MySQL 联合报告](admin-auth-report.json) 的 order_schema 覆盖完整 16 文件计划、
29 表/390 列、重复应用、两版本与 nullable 循环外键、精确金额/19 位字符串、
无效枚举/JSON/布尔和缺失地址外键拒绝、历史主体不创建登录账号、孤儿引用原样保留、
同名原型表保护与模型漂移拒绝。均为人工数据，独占临时 MySQL，不连接旧库或服务器。

[目录固定快照报告](catalog-mysql-rehearsal-report.json) 只验证新完整 schema 下已有
目录/库存导入与读取保持精确重放，不证明订单数据已经导入。

下一步仍需订单逐行导入/差异账本、成交汇总与明确投影查询、后台 Ant 页面与权限、
购物车/订单唯一成单、价格税费配送、付款绑定、库存预留/履约/取消的事务接入和恢复。
Astro 与完整业务替换仍未完成，不启用预留能力或生产交易。
