# Gin 匿名购物车 HTTP

2026-10-28。接入 `internal/cart/http.go`、独立 Redis 创建作用域和实际服务器路由。它提供新匿名购物车创建及精简详情，不包含行项目、购物车总额、客户转移或结账。`cart.create`/`cart.access` 仍默认关闭；未配置 `STOREFRONT_ORIGIN` 时路由返回不可用，公开商品的 checkout 能力仍为 false。

## 接口

| 请求 | 输入 | 结果/边界 |
| --- | --- | --- |
| `POST /api/store/cart-sessions` | JSON `{}`，精确同源 Origin | 分配 12 小时匿名创建作用域 cookie 与独立 CSRF；不会创建 MySQL 购物车。已有有效作用域需正确 CSRF，重用不续期；过期/丢失可重新分配，依赖故障不能冒充过期 |
| `POST /api/store/carts` | JSON `region_id` 与可选 `locale`；单个 UUIDv4 `Idempotency-Key`；作用域 cookie + `X-CSRF-Token`；精确 Origin | MySQL 创建/幂等重放后再检查当前授权、签发独立 possession/CSRF。首次 201，重试 200，同 key 改参数 409；不会接受浏览器指定店铺、作用域、客户或价格 |
| `GET /api/store/carts/:id` | possession cookie；无查询参数 | cookie 指向同一个 cart 且数据库当前授权有效才返回精简快照；仅 ID、创建作用域或后台登录 cookie 均不能访问。不签发 cookie、不更新 TTL，不返回 CSRF、邮箱、地址、metadata 或代数 |

创建会话和购物车 possession 在 Redis key/记录/用途上独立，与 Admin/Customer 会话隔离。作用域 token 的 SHA-256 才用于持久幂等键，不从请求 header/ID 选择作用域。SQL 提交后凭证签发失败返回503且不交付 ID/cookie；使用原作用域与原 operation 重试恢复已提交车，不重复创建。不同操作明确表示新建车，后续前台应只在当前车不存在或完成时调用，不能每次加购新建。

内部权限组件仍独立复核持久代数、active、完成/删除/客户归属和配置店铺/渠道/地区；不能仅凭 Redis 成功视为获准。当前不支持客户持有车，客户转移必须另行实现。

## Cookie、来源与错误

配置 HTTPS 时使用 `__Host-retail_cart_` 前缀，Secure、Path=/、无 Domain；本地回环 HTTP 使用 `retail_cart_`。`scope` 与 `possession` HttpOnly；对应 `scope_csrf` 与 `csrf` 可供同源 JavaScript 读取并放入请求头。SameSite=Lax 保留从外部付款/普通链接回到店铺的导航能力；写操作仍要求精确配置 Origin、拒绝跨站 Fetch Metadata，并校验服务器记录里的 CSRF。当前不提供付款回跳实现，也未在真实 TLS 上验收。

重复 cookie、Origin、CSRF 或幂等头拒绝；未知/重复 JSON 字段、null、非法类型、尾随文档和 query 拒绝。请求体限制沿用 1MiB，已知长度和分块超限均413。所有匹配的购物车响应包括早期413都 `private, no-store`，保留服务端请求 ID，日志只记录路由模板而非 cookie/正文/实际 ID。

缺失/失效凭证401，错误 CSRF/Origin403，凭证绑定其他车或数据库授权失效404，参数400/415、地区不可用422，幂等变参409，依赖/配置/禁用能力503。失败不会返回部分凭证或在 Redis 故障时建立新身份。创建作用域按 IP 每15分钟最多8次，创建/重试按已认证作用域每15分钟最多32次，超限429并返回 Retry-After；使用服务端 Redis 原子计数，无本地放行后备。

## 运行配置与证据

`STOREFRONT_ORIGIN` 显式启用 HTTP 配置，`STOREFRONT_STORE_ID` 为服务器店铺，`SESSION_SECURE` 必须与 HTTPS 一致。远程明文 HTTP、带 path/query/userinfo 的 Origin 均拒绝。新路由复用独立 session Redis；`.env.example` 只给注释示例，当前开发和生产业务不会自动开启新购物车能力。

`test-admin-auth.py` 的 CartRuntime 套件新增真实 Gin dispatch→自有 MySQL/Redis 检查，[联合报告](admin-auth-report.json) 与 [专项报告](cart-runtime-report.json) 的 `cart_http` 登记 HTTP 证据；`browser_verified=false` 明确没有因此宣称购物车浏览器验收。原 Ant/Astro 浏览器联合回归另行执行，用于检查这批路由改动没有破坏既有范围。

人工请求覆盖 cookie 属性、创建和重放、仅ID/错车/重复cookie拒绝、Origin/CSRF/幂等头、严格JSON和两类413、保护投影、真实持久撤销、Redis连接关闭/逐键丢失；单次签发错误注入仅用于验证 SQL 已提交后的恢复，其余正常路径使用真实依赖。独立 Redis 测试另核对作用域与持有/登录域隔离、哈希、不续期、过期/丢失和两个限流上限。Go 单元测试验证 JSON 边界。

后续工作：购物车真实浏览器 cookie 路径、行项目和可信价格/共享库存检查、区域更新及税/促销/配送重算、完整DTO/金额、客户注册/转移、付款与成单，并接回保持原页面样式的 Astro 前台。T032/T033 和全部交易仍未完成。

## 本批验证限制

专项真实 Gin/MySQL/Redis 首次运行通过并生成 `cart_http` 证据，随后补充了 Redis 关闭时 GET 503 的断言、源文件哈希范围及磁盘预检。当前全量 Go 测试与 vet 已通过；Redis 单组件含新创建作用域测试已通过。专项报告保留首次实际运行时的文件 SHA，不手改成最新校验值，最新增补仍需重跑。

联合回归期间本机 C 盘耗尽，旧 Next 前台短暂返回 ENOSPC/500。清除可重新生成的 Go 编译缓存释放约2.6 GiB，前台恢复200，未重启或清除 Next/Medusa 输出；后续本轮 Go 临时目录及缓存显式放在 D 盘任务目录。测试入口增加 workspace 至少2 GiB 空余检查。

Docker Desktop 日志在 2026-10-28 00:49:29 UTC 显示 engine running→stopping，随后 Docker API/inspect 超时。最新数据库/浏览器联合回归未完成，不能引用前一批绿色报告宣称本批全量通过。自有临时容器及runner未完成清理，准确所有权与路径登记在忽略文件 `logs/gin-migration/pending-docker-cleanup.json`；引擎恢复后按label核对清理，重跑专项/联合回归更新报告。当前代码仍默认关闭购物车命令，功能不作整体验收。

后续客户端补充：[Astro私有传输](astro-cart-client.md)已对人工HTTPfixture验证浏览器cookie与稳定操作键，不含实际页面挂载或真实Gin/MySQL/Redis端到端验收；上述Docker重跑与清理仍待完成。
