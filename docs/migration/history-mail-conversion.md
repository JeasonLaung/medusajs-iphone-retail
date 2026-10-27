# 旧注册邮件认证能力处置

日期：2026-10-27。T006/T018/T055的历史转换组件。不是已运行的投递禁令、通用秘密扫描器或正式数据导入。

## 实际来源

旧`src/workflows/registration-email.ts`把验证邮件写入mail_message，固定created_by=registration-verification、direction=outbound，ID为mail_verify_加验证码SHA256。`src/lib/registration-email.ts`生成中英两种固定subject，text_body/html_body包含/{country}-{locale}/verify-account?token=旧验证码。原先逐字段原文转换会把旧登录能力直接带入目标邮件正文，不能只靠加密或不重放说明解决。

普通`rowimport.Convert`和商户`ConfigSecrets.Convert`现在对全部mail_message返回固定ErrHistoryPolicy和空BoundRow，必须使用显式`ConvertHistoricalMail`入口。该入口先按完整源字段计划检查类型、列集合和CHECK，再生成转换结果及不可遗漏的Delivery处置。不会为了清空正文而隐藏源字段类型错误。

## 处置规则

识别出的旧注册邮件必须同时满足creator标记、outbound、精确mail_verify_[0-9a-f]{64} ID形态及已核对的两个固定subject。疑似ID但creator不同、标记相符但ID/方向/subject未知的行返回ErrHistoryPolicy，不能猜测其用途或复制可疑subject。

认证邮件的text_body、html_body及error清空，attachments置为[]，避免正文、错误回显或附件载荷保留旧验证能力。不做脆弱的URL局部替换，不把代码/链接移进另一列。保留原ID、联系信息、固定subject、status/folder、原尝试数和投递时间等审计事实；原始源行保持不变，仅受限旧快照留来源证据。ID中的不可逆hash不是旧可用验证码，不以该hash重建验证记录。

Delivery显式为source_table=mail_message、原ID、effect=mail-delivery、disposition=suppress-permanently、reason=excluded-legacy-registration-capability。其他业务邮件原文保留，但Delivery=hold，需先核对provider幂等窗口、首试时间和未完成副作用；不能在演练中继续发送。该组件不把status改成sent/canceled，也不补新时间。

**后续正式导入必须在同一事务持久化Row及Delivery，新worker必须尊重禁令/hold。当前未实现该账本与worker执行，因此不能声称永久禁令已在数据库生效。** 普通导入入口的拒绝仅防止现有Go整行转换绕过策略，旧目录草稿未替换、不获准运行。

该规则仅处理已核对的本项目注册邮件来源，不扫描所有任意邮件、HTML编码或附件文件中的所有能力，也不宣称未来未知认证邮件被自动识别。历史通知JSON、客户metadata和支付context中旧账号资料还需独立审查；不能因这两封邮件已移除链接就验收“不迁移旧登录材料”的全项目要求。

## 已运行证据

Go人工测试覆盖认证字段清空、原行不变、原状态保留、永久处置、普通业务邮件原文/hold、普通及商户入口拒绝、未知认证形态、缺字段/额外字段及非法正文类型拒绝。

`audit-history-mail.py`只接受Git忽略且SHA256与固定基线一致的旧快照，Go使用RawMessage检查真实两行。旧v1仅对非JSON标量null解释为SQL NULL，遇JSON null立即拒绝，不能猜来源。结果见[history-mail-redaction-report.json](history-mail-redaction-report.json)：2行、2个永久抑制处置、8个清理字段、4个源正文验证链接；普通业务邮件0行，不能当真实覆盖。无新行/处置持久化、无发信、无数据库连接，报告不包含ID、收件人、正文、验证码或链接。

```powershell
python retail/scripts/audit-history-mail.py --bundle logs/gin-migration/source-20261026.json
```

报告中的实现hash用于复查本轮审计；源快照及旧业务库没有修改。正式v2/MySQL导入、投递处置账本和恢复、其他历史认证能力审查仍未完成。
