# 变体库存明细与预留对账查询

2026-10-28。0012 从审查 DDL 生成两张正式编号表：inventory_levels、
inventory_reservations，共 27 个来源字段；原 JSON、时间、软删及精确数量均保留。
保留库存层活动 item/location 唯一生成键及审查版物理外键，未虚构订单关系。
生成器 `retail/scripts/generate-inventory-schema.py --check` 校验产物。
inventory_runtime_schema 固定读模型校验和及关闭的写能力；应用启动和 health 要求
此模型及全部 16 文件执行记录。启动不创建或修改表。

Gin GET `/api/admin/products/{id}/variants/{variant_id}/inventory` 要求现行新员工会话，
先核对未删商品/变体归属，再按未删关联读取库存项及每变体 required_quantity。
只读 REPEATABLE READ 同时读取未删库存层和有效预留合计。
已删库存项保留引用，标题/SKU 不回显；已删仓库保留库存层、仓库名称为空，明确标识。
预留没有匹配有效库存层时单列，不静默丢弃。metadata、原 JSON、操作人/订单引用
及地址不进入此响应。未知 query、非法标识符为 400，跨商品/不存在为 404，
依赖失败为固定 503；每变体 1000 库存项、10000 库存层/预留位置分组上限，超限拒绝截断。

stocked/reserved/incoming、有效预留合计分别保持事实，不进行旧库存修复。持久差异另按仓库和原因显示，编辑不自动清除。
库存余额仅为 stocked-reserved，使用 scale18 大整数计算，不经过浮点数，不截负数。
预留合计与记录预留量用精确数比较，并显示数量一致/待核对。库存余额不是可购买量；
渠道、有效仓库、所需数量、缺货策略及对账 hold 仍须由后续库存/结算命令统一验证。
Redis 不承担库存事实，通用库存写入始终关闭；[0014 专用库存量编辑](admin-stock-edit.md)单独审计。

React 采用 Ant Drawer、Collapse、Descriptions、Table、Tag、Alert、Empty、Spin；
按库存项展开查看数量，服务失败、空关联、无有效库存层和缺失对象分别处理。
金额/数量字符串只去掉显示用末尾零，19 位 SKU 保持字符串。中英文及三种屏宽沿用主题。

证据：

- [实际 MySQL 报告](admin-auth-report.json)：16 文件及重复执行，活动唯一/FK，
  超过 JS 整数精度的小数差、负余额、共享库存项与不同 required_quantity、软删关系、
  未对应库存层预留、读模型漂移/禁用拒绝和读取指纹不变。
- [固定样本演练](catalog-mysql-rehearsal-report.json)：927 库存层与 99 预留的全字段
  转换、实际事务回滚/提交/精确重放；1558 个有效变体和 849 项库存层成员与 SQL 核对。
  已知 1 项预留合计差异保持原值；全域 375 项缺引用仍存在。两表已接入[逐行账本与差异证据](inventory-row-import.md)；不据此声明全域可提交或正式迁移已完成。
- [真实浏览器](admin-live-report.json)：实际 Gin/MySQL/Redis，精度、差异、共享/软删/
  无库存层预留与中英三宽。另有 [UI 人工 HTTP](admin-ui-report.json)失败恢复、空态和数字数量拒绝。

订单行解析、85 条软删预留孤儿的 resolved 字段及问题账本、全域正式导入协调器与库存命令 hold 执行、
并发预留/扣减/履约、完整 RBAC/仓库范围及 Astro 继续待办。此查询仅对当前 admin 开放，
不把供应商/仓库角色的范围授权视为已完成。T011/T012/T018/T020/T021/T023 未验收。
没有操作原开发数据或服务器，原服务继续运行。

后续补充：[0015 预留/释放事务参与组件](inventory-reservation-transactions.md)已通过独占人工 MySQL 验证；默认关闭能力、没有外部路由，订单及履约领域仍待接入。
