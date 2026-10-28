# 匿名购物车持有凭证组件

2026-10-28。`internal/sessionstore/cart.go` 实现独立 Redis 持有凭证组件，供新 Gin 购物车协调器接入。它没有注册 HTTP 路由、创建购物车、恢复旧 cookie 或向客户账号授予历史归属。购物车实际加购、结账仍未开放。

## 凭证与授权边界

- 服务器分别生成 32 字节随机 token 与 CSRF，返回规范小写十六进制；Redis key 只有 token 的 SHA-256，记录只有 CSRF 的 SHA-256。原始凭证不写入 Redis 记录、key、报告或日志。
- `CartReference` 只含 cart ID、店铺 ID 和持久代数引用，不是客户登录身份。独立 `cart:possession` key 空间与 Admin/Customer 的 session/prelogin 相互拒绝，应用 namespace 仍隔离；cart ID 或旧 cart cookie 不是凭证。
- `IssueCart` 是内部接口，只能由协调器在新购物车及其持久授权记录提交后调用，或通过明确的恢复政策重新签发。不得直接暴露成“传入 ID 即签发”的 HTTP 路由。
- `ResolveCart` 只返回内部引用。`AuthorizeCartWrite` 对同一份 Redis 记录校验 CSRF，失败不返回引用。调用方仍须在 MySQL 业务事务中核对配置店铺、当前持久代数、购物车状态和客户转移，校验 Origin、版本及幂等；本组件不拥有提交、价格、库存或支付权限。

凭证有 12 小时绝对有效期及 Redis TTL，读取不续期。Redis 过期、逐键丢失、损坏记录或依赖不可用均拒绝；没有内存、SQL 或按 cart ID 恢复权限的后备通道。依赖错误与无效凭证分开，协调器后续应分别映射 503 与无权访问，而不是在故障时静默新建/覆盖当前购物车。

## 换发与撤销

`RotateCart` 校验旧 CSRF 后，以 Lua 比较完整旧记录、核对剩余 TTL，再 SET 新记录和 DEL 旧记录。新记录保留原绑定、绝对到期与剩余 Redis TTL；只换发 token/CSRF，不延期。16 路并发只有一个成功，旧 token 和旧 CSRF 均拒绝。网络结果不确定时不返回凭证、不盲目重试恢复旧 token。

`RevokeCart` 逐键幂等删除，不操作其他应用或登录域。它只撤销指定 token，不保证撤销先前已换发出的后继 token。客户登录转移、全部会话撤销和成单关闭必须同时使用 MySQL 持久代数/状态；不能把 Redis 单键撤销当作这些业务事务已经实现。换发本身不会更改持久代数或客户归属。

## 实际证据与未完成项

`python retail/scripts/test-redis-sessions.py` 在自有、禁网、无发布端口的单核/128 MiB 临时 Redis 运行原登录组件回归与新购物车测试。结果写入 [Redis 报告](redis-session-report.json) 的 `cart_possession`，包含实现 SHA；测试结束删除自有容器与临时 runner。

已验证引用与 token 拒绝边界、登录/namespace 隔离、哈希存储、CSRF、16 路换发单赢家、不延长 TTL、旧凭证重放、先撤销后 CAS 不复活、绝对及 Redis 过期、坏记录、逐键丢失和断开连接。没有连接应用 Redis、读取旧账号或访问来源数据库。Go 单元测试还验证非法引用在访问依赖前拒绝。

下一步需要持久授权代数/新购物车创建事务、事务内实时授权复核、同源 HttpOnly cookie 与可恢复 CSRF 交付、严格请求结构和幂等，再接入创建/详情/行项目及原前台。当前 `durable_binding_integrated=false`、`http_integrated=false`；T014/T019/T032 继续进行，不能据此验收完整购物车。
