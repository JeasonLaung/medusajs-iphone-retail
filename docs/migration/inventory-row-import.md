# 库存逐行导入账本与差异证据

2026-10-28。0013 增加 `migration_inventory_rows` 与
`inventory_migration_holds`。[0014 专用库存量编辑](admin-stock-edit.md)已单独接入；扣减、预留、履约接口尚未实现。

`inventoryimport.Import` 仅接受 inventory_level / reservation_item，完整执行
27 列严格转换，保留 SQL NULL、JSON 文本、时间、软删和 DECIMAL(38,18)。
生成计划必须通过 `generate-inventory-import-plan.py --check`。
来源身份、固定快照、完整来源行和目标行的 SHA-256 与事务初次写入标记共同记录。
相同导入只校验，不覆盖目标；不同快照、来源变化或目标末位小数变化均拒绝。
调用者必须对任何错误回滚，并在完整输入、父依赖与领域检查通过后才提交。

`PreserveHolds` 必须在同一事务、完整导入后执行。目标只能是独立迁移库，
须先由显式 schema 执行器核对完整 15 文件计划，不能与旧应用 worker 共用。
组件锁定关闭写能力的库存 marker 及库存/预留行，再分别计算预留合计差异、
负余额、无有效库存层的预留。保存固定快照及精确 JSON 证据；重复证据须完全一致。
不修复数量、不删除历史记录、不自动释放旧记录。数据库 CHECK 拒绝 released=TRUE。
专用库存量编辑已检查这些记录；后续预留、履约命令还须统一检查这些记录与仓库授权。

证据见 [固定快照 MySQL 演练](catalog-mysql-rehearsal-report.json) 与
[人工 MySQL 事务验证](admin-auth-report.json)。固定样本 927 库存层、99 预留
共 1,026 条逐行账本，回滚不留业务/账本/差异记录，提交及精确重放保持完整指纹。
1 项预留合计差异及 3 项负余额原样保留，共 4 条差异记录。
报告仅输出聚合值和校验和，未连接原业务库或服务器。

全域写入/恢复协调器、85 条历史预留孤儿的订单行解析、权限范围、并发库存命令
与人工差异处置仍待实现；全域 375 项缺引用未解决，不代表整个迁移已验收。

后续补充：[0015 预留/释放事务参与组件](inventory-reservation-transactions.md)已通过独占人工 MySQL 验证；默认关闭能力、没有外部路由，订单及履约领域仍待接入。
