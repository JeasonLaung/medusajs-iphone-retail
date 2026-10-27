# 长文本完整值唯一登记组件

日期：2026-10-27。`retail/backend/internal/uniquekey` 与独立 SQL `0002_unique_key_registry.sql` 是目标 Go 组件，28项源键已固定适配计划，尚未接入生产领域命令及导入器；这些索引继续hold。它不替代数据库关系审查稿，也不说明现有0001草稿已符合全量字段设计。应用启动不建表。

## 源键适配

`build-unique-key-plans.py`从已审查的源索引和六批字段映射生成被Go嵌入的`source_plans.json`；每项记录源定义、目标字段/类型/可NULL、参与条件和独立固定scope，附全部输入规范换行hash。`--check`验证漂移。未知索引语法/条件拒绝生成，不读源业务行。

`SourceChanges`消费内部目标行投影并一次生成该表全部登记变化，`ApplySourceRow`在调用者事务内执行。不能直接绑定公开请求；缺失key/owner/参与条件字段失败，显式NULL只按可NULL字段接受；布尔false保留为独立值，数值不自动转字符串，时间字符串须有效且不超微秒，拒绝无效UTF-8或未配对UTF-16 surrogate，避免JSON解码替换造成身份合并。空字符串/尾空格/大小写原文保留。输入全行业务验证、字段最大长度、锁和版本仍由领域命令负责。

27项有普通文本/布尔适配；另1项mail_contact.unsubscribe_token是受保护信封，明确ErrProtected，不能把令牌明文写进登记表或按随机密文比较。由于一个mail_contacts行同时有email和token唯一性，该表两项整组拒绝，当前可调用的是其他26项，email适配亦不能单独绕过token。受保护完整值比较/重加密仍待实现。

历史customer键使用目标`legacy_had_account`而非新登录状态，只有历史业务身份约束，没有创建账号、授予权限或邮箱绑定行为。税区两项条件保留province=NULL与非NULL/空串区别；exclusive商品选项不参与全局标题唯一。软删除行仍要求完整投影，不以跳过缺失字段隐藏导入损坏。

## 语义与事务

每个源唯一约束使用固定scope，每个业务行使用原ID作为owner。一组字段按版本字节、字段数、各字段UTF-8字节长度与原字节编码，避免组合歧义；不trim、lower或经浮点处理。任意组成值为NULL或源参与条件不成立时释放登记；空字符串仍是值。范围、owner和字段类型须在调用前明确，不从客户端接受任意scope。

SHA256只定位锁桶；同桶逐字节比较完整LONGBLOB，不把哈希相等当作值相等。相同前缀、大小写、尾空格、Unicode规范化形式及组合边界保持区别。人工注入恒定哈希验证不同完整值能并存、相同完整值仍拒绝，公开API不暴露哈希算法切换。

调用者开启业务事务，锁业务对象、调用Apply、写业务行，最后提交；任何错误必须整体回滚。Apply不自行提交、不重试、不执行外部调用。批量可同时释放旧值后登记新值，从而原子交换。重复scope/owner输入拒绝；同owner同值可重试；返回错误不含完整值、owner或底层SQL。

先按字节顺序锁owner行，从该行读取current_digest，再按scope/digest顺序锁旧/新桶，删除当前批次旧claims、检查并写入新claims与owner位置。owner锁避免同一业务行改值/释放并发串行性缺口；桶及owner行保留，不随释放删除，避免删除锁记录造成并发裂缝。claims读取使用FOR UPDATE获取当前可用数据，不能使用REPEATABLE READ旧快照推断无人占用，依据[MySQL锁定读取说明](https://dev.mysql.com/doc/refman/8.4/en/innodb-locking-reads.html)。

多个Apply调用、不同桶范围锁、外部业务锁和FK仍可能死锁。调用者须有界重试整个纯数据库事务；ErrStorage必须先整体回滚，不能忽略错误或继续提交，更不能重放支付/发信副作用。当前组件以安全的通用存储错误返回，细分重试/运行日志策略仍待技术底座实现。桶/owner长期增长和极端哈希碰撞的候选量需要后续容量限制与观察，不能直接清理正在使用的锁行。

## 实际验证

本机Go单元测试验证组合编码反例；隔离本地MySQL8.4.11执行真实SQL迁移与Go组件，禁网、无发布端口、1核/768MiB，结束核对所有权并清理。没有源行数据、旧库连接、真实账号或远端服务器。

`TestRegistryMySQL`包含完整值强制碰撞、大小写/尾空格/范围隔离、NULL/非参与/软删释放与冲突恢复、批量交换、冲突回滚、显式回滚、同owner重试、重复owner拒绝、旧快照后新占用、同批重复值整体回滚及8路竞争仅一成功。默认go test跳过需要专用MySQL的集成项，不能把skip算通过。

额外实际SQL验证SourceChanges→ApplySourceRow→生成的catalog_options业务表插入在同事务工作：重复完整标题拒绝、尾空格不同可并存、回滚后同标题可再写且回滚业务行不存在。此测试是人工调用路径，不表示现有HTTP商品写入或导入器已经接通。Go单元测试覆盖28项计划和缺字段、布尔/NULL、exclusive/软删、历史客户字段重命名、税区双约束及受保护表整体拒绝。

复现：在`retail/backend`设置GOOS=linux、GOARCH=amd64、CGO_ENABLED=0，使用`go test -c -o ../../logs/gin-migration/registry.test ./internal/uniquekey`生成被忽略的Linux测试二进制，然后恢复环境变量。在仓库根运行：

```powershell
python retail/scripts/test-mysql-types.py --registry-test-binary logs/gin-migration/registry.test
```

该命令使用现有本地镜像，复制二进制到临时容器并从内置socket连接人工库。全部71项原SQL检查及1项Go集成套件通过时报告72项；不带参数只验证原71项。不得将固定路径的正式数据库凭据用于这套测试。

## 仍须完成

受保护token完整比较方案；全部写入、软删除、恢复、并发命令与导入器在同事务调用；基于快照重建、冲突对账与不一致检测；业务版本/权限及幂等；正式迁移执行器校验和和恢复。直接SQL写源映射表不会被登记表自动拦截，必须限制写入路径并验证完整覆盖。在这些完成前，不能宣称28项源唯一约束已迁移，T006/T012/T018均未验收。
