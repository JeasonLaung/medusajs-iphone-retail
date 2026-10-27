# 商户配置原子导入组件

2026-10-27。`retail/backend/internal/merchantimport` 组合已有整行转换、秘密信封及完整唯一键登记；不调用旧插件、HTTP、provider，也不自行提交事务或读取环境变量。它不是正式全域导入器。

固定允许七种源表：mail_account、seo_provider、seo_setting、webhook_endpoint、mollie_config、paypal_connection、paypal_settings。联系人退订能力和历史邮件使用各自入口，不能从此处绕过。字段、类型、SQL NULL、PayPal JSON 和旧秘密认证仍由既有 ConfigSecrets 转换器严格验证。

调用者提供独立目标事务、显式新密钥 Ring、显式旧域/PayPal reader 及已验证的完整快照 SHA256。按下列顺序暂存：

1. 整行转换并计算源行摘要（排序字段、长度前缀、原始 JSON 字节、SQL NULL 标志）及目标摘要。
2. 在 `0006_migration_merchant_dispositions.sql` 账本中登记来源、快照、两项行摘要、首次尝试随机值、固定 policy version 和 `hold / legacy-merchant-reconciliation`，读取加行锁。已有来源或快照不一致直接冲突。
3. 精确重放时读取目标保护列并按表/列/原 ID 认证解密，比较新转换原文；继续使用首次密文、nonce 和 key ID。合法重新密封但摘要不同仍拒绝，不通过重复导入轮换或覆盖密文。
4. 邮箱地址和 SEO 设置 key 同事务调用完整值唯一键登记。其他五表没有对应登记计划，不套用不相关 scope。保留 NO PAD、大小写、Unicode 和软删除参与语义，不截断长值。
5. 首次插入业务行，并用全部列的 SQL NULL 安全比较检查目标。重放缺失目标、非保护字段变化、密文或账本摘要变化均冲突，不补写或更新业务配置。

任何错误必须由调用者回滚整个事务。组件返回成功也不能先于完整流校验、关系审查及领域验收提交。首次账本与业务行缺一、唯一键失败或业务 SQL 失败都不能提交。版本化迁移必须显式应用于隔离目标，应用启动不建表。

`Disposition` 只返回已验证的 hold；缺记录、未知表、损坏或不匹配均失败。数据库 CHECK 拒绝 allow/release。它是未来 worker 的前置条件，不是授权入口；未来邮件投递、回调注册、支付 token 刷新等执行器必须接入它，不能仅检查历史 enabled/connected 状态。当前尚无这些新执行器，因此组件本身不能算运行时副作用限制已验收。

## 验证范围

Go 常规测试验证七表人工输入满足现有转换计划。隔离 MySQL 8.4 的 `TestMerchantImportMySQL` 验证七表整行与 hold 回滚/提交/原密文重放、带旧 key 的新 active key、缺旧 key 拒绝、原文读回、两个完整唯一键冲突、八路同源并发、源行/快照/目标冲突、合法重新密封和摘要篡改拒绝、缺目标拒绝、业务 CHECK 失败后的账本及唯一键回滚、缺 hold/未知表/非法 release 拒绝。

运行方式沿用 `retail/scripts/test-mysql-types.py --registry-test-binary logs/gin-migration/registry.test`（先在 backend 编译 Linux `./internal/uniquekey` 测试程序）。该入口仅用人工数据、单核、768 MiB、禁网、无发布端口；按所有权标签清理临时容器。聚合结果记录于 [mysql-type-fixture-report.json](mysql-type-fixture-report.json)，不含实际业务秘密。

新增[真实隔离演练](merchant-mysql-rehearsal.md)已验证固定快照的5条配置/5 hold/1邮箱唯一登记，8字段独立预期读回、3 SQL NULL、整批回滚/提交/原密文重放。临时库和新密文已清理，两个SEO表为空不算真实样本。尚缺其他保护 JSON 审查、可恢复密钥管理、读取授权、完整导入/恢复及 worker/Gin 运行时。T006/T018/T035 不因此验收。
