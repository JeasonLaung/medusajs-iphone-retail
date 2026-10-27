# 支付历史同事务导入与操作 hold

2026-10-27。接续[完整行转换](payment-history-conversion.md)，提供独立 Go / MySQL 组件。没有调用 provider、写入真实付款或实现支付 worker。执行证据见 [人工 MySQL 报告](mysql-type-fixture-report.json) 的 `payment_history_atomic_hold_replay_conflict_and_concurrency`。

## 写入与重放

显式 migration `0005_migration_payment_dispositions.sql` 为每个来源表/行记录快照 SHA、完整源行 SHA、目标行 SHA、首次尝试标识、策略版本及固定 `hold / legacy-payment-reconciliation`。SQL CHECK 只接受 payment_session/payment 和版本 1，不接受 allow，不提供解除接口。账本不包含密钥、明文支付 JSON 或可登录旧账号。

`PaymentHistory.Import(ctx, tx, snapshotSHA, sourceTable, source)` 复用整行转换，将业务行和操作账本写在调用者的同一事务中。只允许嵌入计划中的表/列名，所有业务值使用参数绑定。先锁账本、后锁业务行；并发重放通过同一个来源主键串行。任何错误必须由调用者回滚；完整流校验和领域验收前不得提交。必须连接隔离目标库并预先准备父记录、正式业务结构及账本，不在启动时创建表。

精确重放要解决 AES-GCM 随机 nonce：每次转换生成的新密文不能直接与首次目标 hash 比较。组件对已有行执行当前读 `FOR UPDATE`，按 provider/表/列/行认证解密原信封，与本次转换后的投影内容逐字节比较，再使用**原密文**计算目标行 hash，并用 SQL 的 NULL 安全比较逐列核验业务值。原记录不覆盖、不重加密、不因目标缺失而重新创建。保留旧解密 key 后切换新 active key，精确重放仍成功；丢失旧 key 时失败。

快照不同、源行改变（即使改变的是将被剥离的旧认证内容）、目标标量改变、密文损坏、目标缺失或目标 hash 改变都会冲突。相同明文的另一份合法密文也属于目标变化，不能仅因解密成功而通过账本校验。

## 操作边界

`HistoricalPaymentDisposition(ctx, tx, sourceTable, id)` 只返回 hold，表示禁止付款相关操作；缺失、损坏、未知表/策略均报错，不能作为允许。未来 worker/命令入口必须将此检查与正常授权、provider 对账和事务规则结合。账本本身不会拦截绕过它的任意 SQL 或尚未实现的 worker，因此当前不宣称新支付流程已安全上线或全域已导入。

源状态、原金额、授权/实收时间等事实不因 hold 改写；新客户也不会因同邮箱获得历史交易。未来解除须另行设计对账、授权和审计流程，本组件没有放行路径。

## 本轮实际验证

禁网、单核、768 MiB 临时 MySQL 8.4.11，使用完整 159 表审查结构及虚构业务数据，测试通过并清理容器：

- 业务行/账本同时提交；调用者回滚两者均无残留。
- 原密文精确重放、新 active key 加保留旧 key 的重放均成功，原三列密文不变。
- 来源/快照变化、旧 key 缺失、状态篡改、明文冒充密文、丢失目标、账本 hash 改变均拒绝。
- 原数据合法重新密封后替换原密文仍被目标 hash 拒绝。
- 父外键失败发生在账本暂存之后，回滚不留下账本或业务行。
- 八路相同来源并发导入全部成功，最终只有一条业务记录和一条 hold。
- payment 的 data/metadata 都为 SQL NULL 时仍原子写入 hold，并可精确重放；缺失 hold 不允许操作；SQL 拒绝 disposition=allow。

`go test ./...`、`go vet ./...` 通过；本机普通测试跳过需要独立 MySQL 的测试，本次已另外运行上述实际 MySQL 套件。类型脚本报告 72 个顶层检查（71 项 SQL 与一个包含多项测试的 Go 套件），不是 72 个完整业务场景。

仍缺：真实 42 条支付的隔离持久化演练、全域导入协调器/恢复、源约束索引漂移校验、375 项历史引用缺失处置、真实密钥恢复管理、provider 远端对账及支付运行时/权限。T006/T018/T035 未验收，不操作服务器。
