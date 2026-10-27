# 长文本完整值唯一登记组件

日期：2026-10-27。`retail/backend/internal/uniquekey` 与独立 SQL `0002_unique_key_registry.sql` 是目标 Go 组件，尚未接入28项长文本源唯一键的领域命令及导入器；这些索引继续hold。它不替代数据库关系审查稿，也不说明现有0001草稿已符合全量字段设计。应用启动不建表。

## 语义与事务

每个源唯一约束使用固定scope，每个业务行使用原ID作为owner。一组字段按版本字节、字段数、各字段UTF-8字节长度与原字节编码，避免组合歧义；不trim、lower或经浮点处理。任意组成值为NULL或源参与条件不成立时释放登记；空字符串仍是值。范围、owner和字段类型须在调用前明确，不从客户端接受任意scope。

SHA256只定位锁桶；同桶逐字节比较完整LONGBLOB，不把哈希相等当作值相等。相同前缀、大小写、尾空格、Unicode规范化形式及组合边界保持区别。人工注入恒定哈希验证不同完整值能并存、相同完整值仍拒绝，公开API不暴露哈希算法切换。

调用者开启业务事务，锁业务对象、调用Apply、写业务行，最后提交；任何错误必须整体回滚。Apply不自行提交、不重试、不执行外部调用。批量可同时释放旧值后登记新值，从而原子交换。重复scope/owner输入拒绝；同owner同值可重试；返回错误不含完整值、owner或底层SQL。

先按字节顺序锁owner行，从该行读取current_digest，再按scope/digest顺序锁旧/新桶，删除当前批次旧claims、检查并写入新claims与owner位置。owner锁避免同一业务行改值/释放并发串行性缺口；桶及owner行保留，不随释放删除，避免删除锁记录造成并发裂缝。claims读取使用FOR UPDATE获取当前可用数据，不能使用REPEATABLE READ旧快照推断无人占用，依据[MySQL锁定读取说明](https://dev.mysql.com/doc/refman/8.4/en/innodb-locking-reads.html)。

多个Apply调用、不同桶范围锁、外部业务锁和FK仍可能死锁。调用者须有界重试整个纯数据库事务；ErrStorage必须先整体回滚，不能忽略错误或继续提交，更不能重放支付/发信副作用。当前组件以安全的通用存储错误返回，细分重试/运行日志策略仍待技术底座实现。桶/owner长期增长和极端哈希碰撞的候选量需要后续容量限制与观察，不能直接清理正在使用的锁行。

## 实际验证

本机Go单元测试验证组合编码反例；隔离本地MySQL8.4.11执行真实SQL迁移与Go组件，禁网、无发布端口、1核/768MiB，结束核对所有权并清理。没有源行数据、旧库连接、真实账号或远端服务器。

`TestRegistryMySQL`包含完整值强制碰撞、大小写/尾空格/范围隔离、NULL/非参与/软删释放与冲突恢复、批量交换、冲突回滚、显式回滚、同owner重试、重复owner拒绝、旧快照后新占用、同批重复值整体回滚及8路竞争仅一成功。默认go test跳过需要专用MySQL的集成项，不能把skip算通过。

复现：在`retail/backend`设置GOOS=linux、GOARCH=amd64、CGO_ENABLED=0，使用`go test -c -o ../../logs/gin-migration/registry.test ./internal/uniquekey`生成被忽略的Linux测试二进制，然后恢复环境变量。在仓库根运行：

```powershell
python retail/scripts/test-mysql-types.py --registry-test-binary logs/gin-migration/registry.test
```

该命令使用现有本地镜像，复制二进制到临时容器并从内置socket连接人工库。全部71项原SQL检查及1项Go集成套件通过时报告72项；不带参数只验证原71项。不得将固定路径的正式数据库凭据用于这套测试。

## 仍须完成

28项源键的固定scope、字段规范化及参与条件适配；全部写入、软删除、恢复、并发命令与导入器在同事务调用；基于快照重建、冲突对账与不一致检测；业务版本/权限及幂等；正式迁移执行器校验和和恢复。直接SQL写源映射表不会被登记表自动拦截，必须限制写入路径并验证完整覆盖。在这些完成前，不能宣称28项源唯一约束已迁移，T006/T012/T018均未验收。
