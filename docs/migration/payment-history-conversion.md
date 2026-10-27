# 支付历史完整行转换与真实 v2 验证

2026-10-27。T006/T018 的实现进展，不代表支付运行时、正式导入器或领域迁移已验收。证据见 [真实 v2 报告](payment-history-v2-report.json) 和 [人工 MySQL 报告](mysql-type-fixture-report.json)。

## 完整行入口

`rowimport.NewPaymentHistory(ring).Convert(sourceTable, row)` 只接受 `payment_session`、`payment`，复用 14 列完整字段计划、精确金额/raw 配对、源 CHECK、时间、枚举及 NULL 校验。未知/缺失列、畸形字段、未审查 provider/JSON、金额/引用不匹配均不返回部分行。输入不修改，原 status、时间、金额和业务引用不因迁移改写。

受保护内容按专用转换处理：data 继承 [provider 审查](payment-data-review.md)；context 继承 [旧身份资料投影](payment-context-review.md)；metadata 仅接受已审查的空对象或 JSON `null`，其他内容拒绝。非 NULL 值均使用独立业务密钥密封，metadata 的 AAD 绑定 provider/表/列/行，不能与 data/context 互换。SQL NULL 保持 SQL NULL，JSON `null` 是有内容的信封。

入口同时返回 `payment-operations / hold / legacy-payment-reconciliation` 处置。普通 `Convert` 和商户配置入口禁止转换这两张表，即使所有保护列都是 SQL NULL，也不能绕过历史处置。后续[同事务导入组件](payment-history-import.md)已补显式账本与写入/精确重放，人工 MySQL 验证通过；正式全域导入及 worker 强制执行仍缺。不得把返回的行直接接入扣款、退款、付款恢复或新客户归属。

## 实际来源与验证

本轮从 compose 指定的本机 `medusa-trial-postgres-1 / medusa / medusa_trial` 执行只读、repeatable-read、UTC 导出，在同一事务中先锁定表结构并核对全部 159 张表的字段类型/顺序/NULL 定义，随后输出 v2 单元格记录。只使用 ACCESS SHARE 锁；旧数据和原 v1 快照未改动。新流在 Git 忽略目录，哈希 `b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985`；不能把此传输哈希当作来源数字签名。约束/索引漂移尚未纳入导出 guard。

Go 验证全部 159 表、27,140 映射行的 framing、顺序、每表结束计数、完整 footer 与文件哈希后，才生成报告。42 个支付整行转换成功，临时密钥/新信封未落盘：

| 项目 | payment_session | payment |
| --- | --- | --- |
| 完整行 | 21 | 21 |
| data 信封往返 | 21 | 21 |
| context 投影/信封往返 | 21 | 不适用 |
| metadata 空对象信封往返 | 19 | 0 |
| metadata 明确 SQL NULL | 2 | 21 |
| 操作 hold | 21 | 21 |

context 共移除 34 个旧地址簿/账号绑定或默认标记字段。关系校验同时确认 224 条引用规则、88 个物理外键、43,496 个非 NULL 引用；43,121 个已解析，375 个已知缺失继续使关系验收失败，物理外键缺失为 0。没有凭整行转换通过而删除、修补或放行这些问题。

人工 MySQL 在完整 159 表审查结构中使用虚构 collection、session、payment，验证三行事务回滚无残留、完整行持久化、精确金额/raw JSON、微秒 UTC 时间和原状态、context 剥离、data 解密及 metadata SQL NULL/JSON `null` 区分。不是实际支付行导入，也未持久化操作 hold 账本；单核、禁网临时容器已清理。

```powershell
python retail/scripts/audit-payment-history.py --snapshot logs/gin-migration/source-v2-20261027-payment-rows.ndjson
# 在 retail/backend 中执行
go test ./...
go vet ./...
```

后续已补支付历史同事务导入、持久化 hold 和保留原密文的重放组件，人工验证并发/冲突/回滚及 active key 切换。仍缺全域 v2 值转换与正式导入组合、真实支付行隔离持久化演练、其他保护 JSON、历史缺失关系处置、远端付款/退款对账、读取权限和在途恢复；服务器与旧业务库继续保持不变。
