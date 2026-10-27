# 历史付款主体专用转换

2026-10-27。`rowimport.AccountHolderHistory` 处理源 `account_holder` 到 `payment_account_holders` 的完整九列转换，结果见 [account-holder-conversion-report.json](account-holder-conversion-report.json)。当前是内存转换和读取组件，没有 SQL 导入、操作账本或运行时授权实现。

本地已安装 Medusa payment 2.21.2 的 `models/account-holder.js` 定义 provider_id/external_id、可空 email、默认空 data 和可空 metadata。`providers/system.js` 的 createAccountHolder 返回旧 `input.context.customer.id` 作为 provider 主体 ID，`services/payment-module.js` 再写到 external_id 并复制旧客户 email。因此，这行外部引用可能等于旧客户 ID，保留它是历史来源证据，绝不能按该 ID 或邮箱给新登录账号恢复付款绑定。

固定 v2 快照中只有一条 `pp_system_default` 主体，data 是空对象，metadata 是明确 SQL NULL。用户不迁移旧登录账号的决定继续有效：旧客户到付款主体的关联表和 context 内 customer.account_holders 仍排除；不根据 historical external_id 重建任何新账号、自动归属或付款方法。此转换不启用系统支付、不调用 provider create/retrieve/delete，也不将主体作为新客户的 context 输入。

专用规则：

- 仅接受已核对的 system provider baseline；其他 provider 需独立审查，返回失败。
- data 只接受空对象，metadata 只接受空对象、JSON null 或列允许的 SQL NULL。保留 JSON 原始字节、SQL NULL 独立标志，不把 JSON null 改为 SQL NULL。
- 非空 JSON、token、账号绑定、保存的付款方法、未知字段、重复 JSON key、非法 UTF-8/时间、缺列、多列、SQL NULL 带载荷均不返回部分目标行。
- data/metadata 使用新 Ring 认证密封，AAD 包含专用 purpose/provider、目标表、列及原 ID，目标 LONGTEXT 存储 `ers1.` + 严格 canonical base64 的版本信封；不把旧秘密直接当新秘密，也不接受明文回退。读取再次校验 provider 和 JSON baseline。
- 普通 Convert、商户 ConfigSecrets 入口均要求 `account-holder-history` 专用策略，不能绕过处置。专用 Convert 同时返回 `payment-account-operations / hold / legacy-account-holder-reconciliation`。未来 SQL 入口必须把整行、hold 及 provider/external_id 完整唯一登记纳入同一事务，worker 必须强制执行 hold；不能仅保存 Row 丢弃 Operations。

人工测试覆盖完整行、三种 metadata NULL/JSON 状态、保护值原文、错误行/provider/列拒绝、原行不变、未知载荷/账号字段拒绝、缺 key、轮换保留旧 key 读取、篡改及认证成功但载荷未审查仍拒绝。真实离线审计先完整校验 159 表/27,140 行及固定 hash，再转换唯一主体：九列、一个保护 JSON 往返、一个 SQL NULL、一次错行拒绝、一个必须持久化的 hold。新 key/密文仅内存，无 SQL、源数据库、网络或 provider 调用。

```powershell
python retail/scripts/audit-account-holder.py --snapshot logs/gin-migration/source-v2-20261027-payment-rows.ndjson
```

剩余：完整唯一登记及原子 SQL hold、真实 SQL 持久化/回滚/重放、其他 provider 非空资料审查、持久密钥管理、读取授权、正式导入组合/恢复和 worker/Gin 付款运行时。支付 context 的顶层交易主体证据与本表不是同一存储路径，不能拿一项验证替代另一项。T006/T018 仍未验收。
