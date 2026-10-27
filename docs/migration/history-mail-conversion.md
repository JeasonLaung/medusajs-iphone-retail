# 旧注册邮件认证能力处置

日期：2026-10-27。T006/T018/T055的历史转换组件。不是已运行的投递禁令、通用秘密扫描器或正式数据导入。

## 实际来源

旧`src/workflows/registration-email.ts`把验证邮件写入mail_message，固定created_by=registration-verification、direction=outbound，ID为mail_verify_加验证码SHA256。`src/lib/registration-email.ts`生成中英两种固定subject，text_body/html_body包含/{country}-{locale}/verify-account?token=旧验证码。原先逐字段原文转换会把旧登录能力直接带入目标邮件正文，不能只靠加密或不重放说明解决。

普通`rowimport.Convert`和商户`ConfigSecrets.Convert`现在对全部mail_message返回固定ErrHistoryPolicy和空BoundRow，必须使用显式`ConvertHistoricalMail`入口。该入口先按完整源字段计划检查类型、列集合和CHECK，再生成转换结果及不可遗漏的Delivery处置。不会为了清空正文而隐藏源字段类型错误。

## 处置规则

识别出的旧注册邮件必须同时满足creator标记、outbound、精确mail_verify_[0-9a-f]{64} ID形态及已核对的两个固定subject。疑似ID但creator不同、标记相符但ID/方向/subject未知的行返回ErrHistoryPolicy，不能猜测其用途或复制可疑subject。

认证邮件的text_body、html_body及error清空，attachments置为[]，避免正文、错误回显或附件载荷保留旧验证能力。不做脆弱的URL局部替换，不把代码/链接移进另一列。保留原ID、联系信息、固定subject、status/folder、原尝试数和投递时间等审计事实；原始源行保持不变，仅受限旧快照留来源证据。ID中的不可逆hash不是旧可用验证码，不以该hash重建验证记录。

Delivery显式为source_table=mail_message、原ID、effect=mail-delivery、disposition=suppress-permanently、reason=excluded-legacy-registration-capability。其他业务邮件原文保留，但Delivery=hold，需先核对provider幂等窗口、首试时间和未完成副作用；不能在演练中继续发送。该组件不把status改成sent/canceled，也不补新时间。

**后续正式导入必须在同一事务持久化Row及Delivery，新worker必须尊重禁令/hold。** 现在已有下面的账本和同事务写入组件并通过人工MySQL验证；真实行尚未导入，worker尚未接入，不能声称真实邮件已由运行中的worker永久禁投。普通导入入口的拒绝仅防止现有Go整行转换绕过策略，旧目录草稿未替换、不获准运行。

该规则仅处理已核对的本项目注册邮件来源，不扫描所有任意邮件、HTML编码或附件文件中的所有能力，也不宣称未来未知认证邮件被自动识别。历史通知JSON、客户metadata和支付context中旧账号资料还需独立审查；不能因这两封邮件已移除链接就验收“不迁移旧登录材料”的全项目要求。

## 已运行证据

Go人工测试覆盖认证字段清空、原行不变、原状态保留、永久处置、普通业务邮件原文/hold、普通及商户入口拒绝、未知认证形态、缺字段/额外字段及非法正文类型拒绝。

`audit-history-mail.py`只接受Git忽略且SHA256与固定基线一致的旧快照，Go使用RawMessage检查真实两行。旧v1仅对非JSON标量null解释为SQL NULL，遇JSON null立即拒绝，不能猜来源。结果见[history-mail-redaction-report.json](history-mail-redaction-report.json)：2行、2个永久抑制处置、8个清理字段、4个源正文验证链接；普通业务邮件0行，不能当真实覆盖。无新行/处置持久化、无发信、无数据库连接，报告不包含ID、收件人、正文、验证码或链接。

```powershell
python retail/scripts/audit-history-mail.py --bundle logs/gin-migration/source-20261026.json
```

报告中的实现hash用于复查本轮审计；源快照及旧业务库没有修改。正式v2/MySQL导入、worker接入、其他历史认证能力审查仍未完成。

## 同事务持久化组件

新增显式migration `0004_migration_mail_dispositions.sql`，固定source_table/source_id二进制主键、快照/源行/目标行SHA256、策略版本1、首次写入随机标识及处置/原因。数据库CHECK仅允许已知hold或suppress-permanently组合，不提供allow值。账本只含必要历史身份和完整性摘要，不含正文/验证码；源行摘要也属于受限迁移元数据，不作为公开ID或账号能力。当前未声明跨表FK，因为正式邮件关系migration仍未整理完成；入口以事务确保两项原子写入，不能据此宣称数据库禁止所有绕过入口的直接SQL。

`ImportHistoricalMail(ctx, tx, snapshotSHA256, source)`在内部执行专用转换，不接受可伪造的BoundRow/Delivery结果；使用调用方的隔离目标事务、固定嵌入列名和绑定参数。调用方必须先保证完整快照与所有域校验，通过前不能commit，任何错误必须整笔rollback。函数不创建表、不begin/commit、不读取.env或发信；失败只返回固定错误，不回显SQL驱动可能附带的值。

先登记并锁定同一源ID的处置，再写完整脱敏目标行。新登记的随机初次标识区分新写入与重复写入，不依赖驱动RowsAffected/CLIENT_FOUND_ROWS。已有账本的快照、精确源行摘要、目标摘要、版本和处置必须全部相同；重跑还锁定目标并比较每个列值，NULL使用NULL安全比较。旧正文即使都被清空，源差异仍导致冲突。目标缺失/已更改或没有账本却已有同ID目标均不覆盖、不静默重建，失败由调用方回滚。源行摘要按字段名排序、长度编码、原始cell词法及SQL NULL标记计算，不经过浮点或JSON重编码。

`HistoricalMailDeliveryDisposition(ctx, tx, sourceID)`是供未来worker使用的内部前置查询，锁定账本，缺失/损坏返回错误；两个成功结果都表示不能投递，不返回允许发送值，也不提供解禁API。新业务邮件的独立作业来源与实际worker尚未实现，不能把“查询存在”当作生产投递控制已接入。

实际MySQL子测试`historical_mail_and_disposition_atomic_replay_and_conflict`已验证：脱敏邮件及处置同事务提交/回滚、相同输入重放、8路并发只留一组行、源正文变化/快照变化/目标变化/目标丢失拒绝、缺账本查询拒绝、额外数据库CHECK使业务INSERT在账本写入之后失败且回滚无残留、不支持allow处置，以及普通业务原文保留并hold。只有人工数据；隔离单核/768MiB/禁网容器已清理，结果收录mysql-type-fixture-report.json。正式导入运行账本、全部领域事务组合、失效恢复与worker集成仍缺。
