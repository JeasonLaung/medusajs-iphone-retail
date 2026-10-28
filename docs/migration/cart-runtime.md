# 新购物车创建与持久授权

2026-10-28。`0026_cart_runtime_authority.sql` 与 `internal/cart/runtime.go` 接续正式购物车关系表和 Redis 持有凭证。当前实现内部创建、精简快照和事务授权参与组件；没有 HTTP 路由，也没有行项目、税/促销/配送重算或结账。两个命令能力 `cart.create`、`cart.access` 均默认关闭，不能为此开启正式关系结构的通用 writes 标记。

## 创建事务及重试

MySQL 同一事务创建空购物车、`cart_ownership` 和 `cart_creation_operations`。`cart_creation_scopes` 的主键锁串行化同一匿名作用域的首次创建，避免“未找到回执”竞争；作用域和操作 UUID 为联合主键，规范请求 SHA 固定店铺/地区/locale。同作用域同操作重试返回原车，不重复新建；参数改变拒绝。不同作用域不取得另一作用域的回执，即使 operation UUID 相同。

`CreateCommand.StoreID` 必须由服务器配置提供，`ScopeHash` 必须来自单独认证的、服务器创建的匿名会话。它们不是可直接向浏览器开放的参数；当前匿名创建会话及 HTTP 协议尚未实现，不得直接从请求 ID、email、旧客户 ID 或任意客户端 header 生成作用域。购物车 ID、授权代数由服务器随机生成，不允许客户端指定。

创建核对正式结构与命令能力、店铺有效默认渠道、有效地区及店铺所配置币种；配置 locale 时必须匹配目录，未配置时保留原可选 locale（有格式和长度约束）。不接收客户端成交价、客户 ID、地址或 metadata。创建不预留库存、不建立付款。任意后续写失败，包括最后回执失败，整个事务回滚且不返回引用。拒绝传入已有事务作为 Create 的连接，避免在外层提交之前向 Redis 发放访问权。

Redis 凭证只能在 MySQL 提交成功后签发；提交或签发结果不确定时，以相同已认证作用域和 operation 重试。数据库不会持久保存原始 token/CSRF。回执重放仍核对当前授权及购物车生命周期；不能凭旧回执恢复已撤销代数或复用已成单购物车。当前并未提供自动过期清理、作用域恢复或客户登录转移。

## 持久授权与版本

`cart_ownership` 仅为新运行时创建记录。迁移不按历史 carts/customer_id 回填授权，不恢复旧账号。cart/store 外键、规范代数、正版本与 active 布尔约束明确；授权和创建回执均有持久引用。

`Inspect` 接收已由 Redis 校验的内部 `CartReference`，结合配置店铺，在事务中重读并加锁核对当前店铺、代数、active、版本和购物车状态。历史无授权记录、跨店铺、软删除、已完成以及带 customer_id 的车均拒绝匿名访问。当前渠道或地区/币种配置失效也拒绝，不返回猜测值。精简快照只投影 ID、region/channel、currency、locale 和字符串 version，不输出历史邮箱、地址、metadata、客户 ID 或代数；行项目与金额装配仍待实现。

`LockForWrite` 必须在调用方已有事务中执行，取得授权与购物车锁并核对预期版本，不自行提交。后续领域命令仍要校验 CSRF/Origin、幂等及业务状态，在同一事务更新版本，任何失败回滚全事务。不能在事务外拿到一次快照就视为永久授权。当前没有公开切换代数/active 的命令；测试中的直接更新只作用于人工隔离数据，客户转移及成单关闭仍需专门命令。

## 证据

- `go test ./...` 和 `go vet ./...` 检查编译及输入边界。非法 UUID/作用域/ID/locale/version 在数据库操作前拒绝。
- `python retail/scripts/test-admin-auth.py --cart-runtime-only` 使用自有禁网 MySQL+Redis，无发布端口，输出 [专项报告](cart-runtime-report.json)。完整 `test-admin-auth.py` 也运行该套件，输出 [联合报告](admin-auth-report.json)。应用数据库、旧账号和服务器均不参与。
- 人工数据验证 8 路重复创建仅 1 次新建、作用域隔离、变参拒绝、回执失败原子回滚、无旧账号创建、历史 cart ID 拒绝、跨店铺及生命周期拒绝、地区/币种/locale 校验、禁用能力拒绝。
- 在真实 Redis 签发/解析新引用后读取 MySQL；改变持久代数后，Redis 仍可解析的旧引用及旧创建回执均被拒绝。
- 通过 MySQL `information_schema.innodb_trx` 观察第二写事务实际 `LOCK WAIT`，第一写事务提交版本 2 后第二事务的版本 1 被拒绝。参与组件后的人工失败回滚，版本仍为 2，不以定时等待推断锁成功。

这不是 CT01—CT12 或 T032 完整验收。下一步是匿名创建会话、同源 HttpOnly cookie/CSRF 与严格 HTTP schemas，再接入行项目、可信选价/库存、完整金额/运费/税/促销、付款及成单协调和原前台。
