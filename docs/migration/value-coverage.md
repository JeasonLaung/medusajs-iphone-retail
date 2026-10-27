# 全量 v2 值转换覆盖

2026-10-27，执行结果见 [value-coverage-report.json](value-coverage-report.json)。这是完整流验证后的字段与普通整行转换证据，不是 SQL 持久化、业务数据授权或完整导入验收。

固定私有 v2 快照 SHA256 为 `b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985`。`InspectValues` 先检查源表及完整字段集合，再逐字段执行与 Convert 相同的严格转换，独立检查 raw-number 配对及源 CHECK；遇到非 NULL 保护字段仍继续检查后面的普通字段，不返回任何目标值。缺列、多列或未知表不返回部分检查结果。保护字段仅记录“需要专用转换”，不宣称已认证解密。

所有报告字段名来自固定映射计划；没有源 ID、业务值、个人资料、密文、密钥或原值摘要。快照文件 hash 与代码 hash 用于重现。原始快照不写入 Git，不连接源数据库或任何 SQL 目标。

| 项目 | 结果 |
| --- | ---: |
| 已验证源表 | 159 |
| 有真实行的表 / 空表 | 98 / 61 |
| 源行 | 27,140 |
| 字段检查 / SQL NULL | 220,235 / 73,405 |
| 普通完整 Convert 成功行 | 27,090 |
| 需要专用保护值或历史规则的行 | 50 |
| 含非 NULL 保护字段的行 | 48 |
| 需要显式历史处置的行 | 45 |
| 非法普通字段 / raw 配对失败 / CHECK 失败行 | 0 / 0 / 0 |

保护与历史计数有重叠，不能相加。50 行分布如下：

| 源表 | 行数 | 普通入口阻止原因 |
| --- | ---: | --- |
| account_holder | 1 | data 专用审查及历史操作 hold，涉及旧付款账号绑定 |
| mail_account | 1 | api_key、webhook_secret 认证解密及新密封 |
| mail_message | 2 | 历史邮件必须带处置，已知注册认证正文需要剥离 |
| mollie_config | 1 | api_key 认证解密及新密封 |
| payment | 21 | data 专用审查及强制历史操作 hold |
| payment_session | 21 | data/context/metadata 专用审查及历史操作 hold |
| paypal_connection | 1 | metadata 专用审查及新密封 |
| paypal_settings | 1 | data 专用审查及新密封 |
| webhook_endpoint | 1 | url/secret/token 认证解密及新密封 |

支付和五条商户配置另有真实临时 MySQL 演练；两条注册邮件已有离线清理及人工事务验证。[account_holder 专用转换](account-holder-conversion.md)已支持 system provider 空资料及强制历史 hold，真实一行离线往返通过，原子 SQL 仍缺。不能把本报告的受阻行计数当作这些专用实现失败，也不能因此绕过其处置。普通 Convert 成功也不代表该行可直接发布：字段内 JSON 只证明严格文本及既定形状，嵌入资料、权限、业务状态、关联及副作用仍需对应审查。

测试覆盖全部159映射计划与普通入口成功条件相等、历史/秘密不能掩盖较后时间错误或 raw 配对错误、SQL NULL 不能附带原始值、未知字段不泄漏私有名称或返回部分数据。61 张空表没有真实值样本，不能算业务验证通过。原有375项历史引用缺口、完整源约束/索引漂移指纹、正式 MySQL 导入/恢复及全部 Gin/前后台实现仍未完成。

```powershell
python retail/scripts/audit-value-coverage.py --snapshot logs/gin-migration/source-v2-20261027-payment-rows.ndjson
```

审计程序成功表示完整检查完成；若将来报告存在 invalid_rows，报告仍可用于对账，不等于允许导入。报告固定 `complete_import_accepted=false`，没有任何覆盖审批或修复源值的入口。
