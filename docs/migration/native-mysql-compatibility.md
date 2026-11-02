# 本机 MySQL 外键处理与隔离 SQL 验收

2026-10-31，本机 Homebrew MySQL 为 9.6.0。主服务保持在 3306；现有开发服务及主数据库没有为本次测试重启或修改配置。

## 已复现的环境差异

完整 94 迁移测试在主 MySQL 的独立一次性测试库运行时，前六套通过，`TestOwnedTaxSchema` 在原断言 `UPDATE tax_rates SET tax_region_id='missing' WHERE id='product'` 失败。实际 `SHOW CREATE TABLE` 和 `information_schema.TABLE_CONSTRAINTS` 都存在该物理外键，全局/会话 `foreign_key_checks` 均为 1，人工测试行仍写入了不存在的父项。测试库随后删除，未写主业务库。

证据：`logs/gin-migration/native-sql-local-41d3d64a4bf9/TaxSchema.log`、`tax-native-diagnostic.log`、`cleanup.json`。诊断复现结束也确认两个测试库计数为 0。

MySQL 从 9.6 开始默认在 SQL 层处理外键。官方缺陷库 [Bug #121438](https://bugs.mysql.com/bug.php?id=121438) 报告复合索引包含非外键 NULL 值时可能漏掉外键检查；这与本次包含可空生成列的索引及实际异常相符，尚不等于上游已经确认本项目的复现。该记录给出的规避方式为 `--innodb-native-foreign-keys=ON`。官方[变量文档](https://dev.mysql.com/doc/refman/9.7/en/innodb-parameters.html#sysvar_innodb_native_foreign_keys)说明它是启动时全局选项，不能按会话修改。

不能通过关闭外键、改业务断言、移除原唯一索引或把当前环境的未通过结果记录为通过来处理此差异。主 3306 默认模式的完整数据库验收仍未通过；独立实例的通过也不能代替主服务配置验收。

## 独立本机 MySQL 验收

```sh
python3 retail/scripts/test-native-sql-local.py prepare --isolated-mysql
python3 retail/scripts/test-native-sql-local.py run --isolated-mysql
```

控制器复制并固定后端、迁移和原行为来源，保留全部 23 套原 SQL 测试与迁移数量断言。仅调整测试连接、37 个测试库名、Redis 命名空间，并增加固定数据库/连接白名单。生产 Go/SQL 不修改，测试前后验证 SHA。

MySQL 使用已安装的官方 Homebrew 二进制，在本次快照的独立数据目录初始化；只允许 0700 私有目录下的 Unix socket，禁用 TCP、MySQL X 和 binlog。InnoDB 外键处理及外键检查均启用。空 root 仅属于该新建、无网络的临时实例，不使用或改变主 MySQL 的 root 凭据。没有第二个业务后端或 worker。

每套测试结束清理全部白名单测试库及只属于本次随机前缀的 Redis key。整个运行结束关闭所创建的 MySQL 子进程，确认退出后删除它的数据目录与 socket，保留源码快照、测试日志、运行参数事实及清理收据。状态不允许重用已有测试事实；需要重跑时先检查原状态及清理证据，再准备新的独立快照。

这项验收证明固定源码在对应数据库模式下的原 SQL 行为，不证明真实付款、邮件收件、主库升级或 C01–C11 全部交付完成。

## 本轮实际结果

完整 94 迁移的 23 套 SQL 已全部通过，无跳过；1,743 个固定源文件、23 份测试日志和执行二进制摘要已复核。见 [native-sql-94-report.json](native-sql-94-report.json)。隔离实例最终正常关闭，37 个测试库、专属 Redis 前缀、临时 MySQL 数据目录及 socket 均已清理，日志保留。

主数据库另外完成只读核对：仍为 86 个迁移，`innodb_native_foreign_keys=0`，外键检查为 1；184 条物理外键（含 11 条复合外键、197 个列引用）按完整列组合检查，目前悬空引用观察为 0。这仅说明当前数据检查未发现物理悬空引用，不证明默认模式会拒绝将来的非法写入。聚合证据见 `logs/gin-migration/native-mysql-main-readonly-facts.json`。

0095 购物车新账号归属随后使用独立冻结快照 `91570716d79a` 完成完整原23组 SQL，全部通过且无跳过；1,754 个源文件、95个迁移、23份日志和二进制摘要均已复核。见 [native-sql-95-report.json](native-sql-95-report.json)。专用实例及全部自有资源清理通过；主86没有升级。本次仍采用原生 InnoDB 外键模式，不证明共享 MySQL 默认 SQL 层模式已通过。此前 `a7df385f58c9` 因原本机 Redis 未运行而在首组通过后的清理处中断，保留失败证据；恢复原 Redis 后新快照从头执行23组，不拼接部分结果。
