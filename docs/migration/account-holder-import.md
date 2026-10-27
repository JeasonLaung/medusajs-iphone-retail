# 历史付款主体原子 SQL 导入

2026-10-27。`retail/backend/internal/holderimport` 组合[专用转换](account-holder-conversion.md)、完整 provider/external_id 唯一登记和 `0007_migration_holder_dispositions.sql` 历史操作 hold。它没有环境读取、客户账号关联、HTTP/provider、startup DDL 或自动 commit。

调用者必须提供隔离目标事务、新 Ring 和经过完整流校验的快照 SHA256，任何错误回滚整事务；组件成功也不能先于关系和领域验收提交。它固定操作 payment_account_holders 与专用账本，不接受任意表名或 scope。其他 provider 的资料仍需独立审查。

首次导入先执行九列严格转换，使用排序字段、长度前缀、原始值 JSON 字节及 SQL NULL 标志计算来源摘要。账本保存完整快照摘要、来源/目标行摘要、随机首次尝试值及固定 policy 1 / hold / legacy-account-holder-reconciliation。读取账本加行锁，防止并发把一次导入当成不同尝试。

已有记录要求来源行和快照相等，读取目标加锁并认证打开 data/metadata，比较专用转换后的预期原文，再使用首次密文重新计算目标摘要。因此，重放保留旧 nonce/key ID；带旧 key 的新 active key 可读取，缺旧 key、认证失败、合法重新密封但摘要不同均冲突。SQL NULL 与加密的 JSON null 不混同。目标丢失不重新创建，其他列变化不覆盖。

完整唯一登记使用已有固定 `source_uk_81dae4f5552d02c1dcaedcd4`，比较 provider_id + 完整 external_id，按 deleted_at IS NULL 参与。它与账本及业务行同事务，不用文本前缀作为身份，不按邮箱做新客户匹配。已软删除历史行继续保留，但不占用活跃唯一声明；不自动修改或恢复其状态。

目标整行插入后使用全部九列 SQL NULL 安全比较。重复身份、业务 SQL 失败或整行不一致必须回滚。失败时只有 rollback 能清除本事务已暂存的账本和登记，组件不自行提交/修复。`Disposition` 只能返回经过校验的 hold，缺记录/损坏失败，数据库 CHECK 拒绝 allow/release。未来付款主体执行器必须调用它；目前尚未实现执行器，因此不能宣称运行时限制已验收。

人工 SQL 验证位于 `internal/uniquekey/holder_import_test.go`（外部测试包避免依赖循环）。使用现有单核/768MiB/禁网临时 MySQL 流程，创建独立 holder_fixture，以完整审查 schema 的实际 payment_account_holders 表结构建表，再显式应用 0002/0003/0007。测试不读取实际业务数据，也不连接应用 DSN。

覆盖：长 Unicode external_id 的整行/hold/登记回滚、三种 metadata NULL/JSON 状态的提交和原密文重放、带旧 key 的轮换读取、缺旧 key 拒绝、八路同源重放、八路不同主体竞争同一外部身份仅一个成功、大小写/尾空格/相同长前缀不同完整值、软删除不参与、来源/快照/普通目标变化、合法重新密封冲突、业务 CHECK 失败完整回滚及随后声明成功、缺目标不补写、缺 hold 和 release 拒绝、未知 provider 和非法快照拒绝。

结果见 [mysql-type-fixture-report.json](mysql-type-fixture-report.json) 的 TestHolderImportMySQL。顶层72项仍为71项SQL检查加一个Go套件，不是72个业务场景。结束按所有权删除临时容器。

新增[真实SQL演练](account-holder-mysql-rehearsal.md)已将1主体/1实际provider/1 hold/1唯一登记在临时MySQL执行回滚、提交、精确重放和保护读取，临时库清理完成，没有恢复客户绑定。仍缺正式全域导入组合及恢复、持久密钥/读取授权、其他 provider 资料和 Gin/worker 运行时。原有375项历史引用缺口保持未验收。T006/T018 不因此验收。
