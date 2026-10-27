# 完整逐列快照与流式校验

日期：2026-10-27。export-cell-snapshot.py和Go rowimport.VerifyStream现已通过隔离人工PostgreSQL→磁盘→Go验证。没有重新采集旧业务库或替换固定v1快照；正式导入器、受保护转换和业务对账未完成。

## 导出

命令显式要求container/user/database/output，不带任何旧库或凭据默认值。只写不存在的Git忽略路径，既有文件或.sha256拒绝覆盖；查询文件亦独占创建并在finally仅清理本次拥有的元数据文件。源记录内容不进入工具输出、Git报告或stderr。数据输出直接从psql流向文件，不将整库读入Python/JavaScript内存。私有文件继承输出目录ACL；生产导出前须保证该目录只供预期操作者访问，本工具不承诺跨平台ACL加固或静态加密。

全部159张计划表在同一psql会话、REPEATABLE READ READ ONLY事务内查询；UTC固定，显式ACCESS SHARE锁保护查询期间表结构，限制5秒锁等待及单语句120秒/进程600秒超时。在输出任何记录前核对全部表的精确列集合、源ordinal_position（保留删列空位）、data_type、udt_name、is_nullable、字符上限、numeric_precision/scale及datetime_precision；schema变化失败。源metadata规范换行hash绑定到Go字段计划，映射生成器亦验证同一源hash，防止替换本地源清单后默许。域约束、默认值、索引、枚举标签顺序/值、collation及其他schema事实仍未由该检查验证。34项排除/重建表不导出。读取包括软删除历史，不做邮箱绑定或账号迁移。

记录为UTF-8 NDJSON：snapshot头含格式2及规范换行field_plans.json的SHA256，随后159表按固定计划顺序依次table_begin、零至多条[v2 cell记录](cell-record-contract.md)、table_end及同事务COUNT(*)，COMMIT成功后输出snapshot_complete/159。没有排序保证，同表行顺序由数据库给出；文件hash标识本次字节序列，不假定重导出必得相同hash。

psql或进程失败时部分数据保留但不产生完成checksum，不能导入或追加续传；输出路径不覆盖，后续应显式选择新运行位置。成功后分块计算整文件SHA256并独占创建sidecar。校验和是传输完整性证据，不是来源签名；需从可信导出运行取得期望hash，不能信任攻击者同时替换文件和sidecar。脚本不采集源序列/文件仓库或非事务外部事实，相关冻结与manifest仍须补齐。

## Go流式验证

VerifyStream消费io.Reader及外部完整SHA256，按行有界读取，强制LF及完整末尾换行，不接受CRLF隐式归一化。校验格式/计划hash、159表精确顺序、完整cell记录、逐表结束行数、唯一完整footer、EOF及整文件hash。每表证据含实际行数和该表原记录字节SHA256；空表亦保留证据。不将全量记录驻留内存。

visit回调可验证或暂存每行，但不能在VerifyStream成功前提交业务写入；错误可能在最后一行/hash才发现，任何错误都使整个流失效，必须回滚或废弃独立暂存。返回错误固定，回调错误亦不透传可能含秘密的内容。校验仅证明结构/cell与完整性，必须另调用Convert、受保护域处理、raw金额/引用/唯一登记及状态对账；不能将回调访问计数作为业务导入成功数。

## 验证

test-cell-record-postgres.py现执行真实export函数：已有本机postgres:16-alpine、禁网、无端口、1核/384MiB、人工159表，3条实际text/jsonb/timestamptz标签行，其他表为空、使用固定源类型/nullable/顺序的人工结构（含枚举类型及删列ordinal空位），未复现全部源CHECK/FK/default/index。Go TestActualPostgresSnapshotStream验证全部159表、3行、footer/COUNT/hash并调用Convert。该用例证明单事务导出脚本真实运行，不证明真实库全部类型/关系或数据转换已通过。

Go TestFullStreamIntegrityAndFraming另验证完整人工流、截断尾换行、额外行、错误表数/行数/版本/表名、错误SHA及回调失败全部拒绝。默认go test跳过需要实际PostgreSQL输出的项；完整复现：

```powershell
python retail/scripts/test-cell-record-postgres.py
```

实际负例将相同列名下的jsonb改json、移除NOT NULL、微秒改毫秒、VARCHAR255改254、无界numeric改numeric(38,18)，五类均在任何业务记录前拒绝且不产生checksum。报告cell-record-postgres-report.json只保存聚合结果、镜像及构造器/导出器/shape检查hash，人工记录和完整流仅在Git忽略目录。隔离容器结束核对所有权清理。旧开发服务仍独立运行。

后续仍需源schema约束/默认/索引/枚举标签等完整指纹、冻结真实一致快照的运行清单与可信hash、私有文件及序列位置关联、受保护数据转换、正式迁移及可恢复的导入运行账本、全量数据对账。T006/T018没有据此验收。
