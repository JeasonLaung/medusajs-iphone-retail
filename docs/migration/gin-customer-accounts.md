# 新客户身份：注册与验证底层

本项服务于 C02 和整个前台闭环，当前已实现底层和客户 HTTP，尚不构成可用的注册页面。没有增加或修改原前台样式，也没有连接服务器。

## 来源与行为

原注册表单和 `lib/data/customer.ts` 提交邮箱、密码、名、姓、电话；邮箱去空白并转小写。注册后需邮箱验证，验证链接可以在其他设备打开，成功后回到登录。原 `registration-email.ts` 使用受配置约束的站点地址、国家/语言路径、15 分钟一次性链接；`registration-guards.ts` 限制自己的验证地址并执行 60 秒重发冷却。

新包 `retail/backend/internal/customer` 独立承接这些身份规则，不使用 Medusa 或管理员会话。`0038_customer_accounts.sql` 明确创建新账户、验证摘要和加密邮件意图三张表。新账号 ID 独立生成；代码没有根据邮箱查询或绑定历史客户、订单、回收记录。

## 已实现

- 注册在单个 MySQL 事务中保存 bcrypt 密码散列、待验证账户、SHA-256 验证摘要及加密邮件意图；后续写入失败时全部回滚。重复注册不能覆盖待验证账号或改变其密码。
- 邮件意图中包含站点配置生成的验证链接，采用已有 `protected.Ring` 密钥环并绑定用途、表、列及行 ID 加密。原始令牌不会出现在公开对象、错误或日志中。
- 密码正确但邮箱未验证时拒绝登录；重发须校验该账号密码，遵循 60 秒冷却并废止原链接和未发送邮件意图。仅校验链接并不会为打开它的浏览器登录。
- 验证事务先锁账户，再重新检查当前挑战；过期、重放、停用或失效挑战拒绝，并发验证只有一次成功。
- 登录使用现有 Redis 的 customer realm；读取当前客户时重新校验 MySQL 的停用状态、验证状态和撤销代数；退出撤销当前会话。公开客户对象不包含密码散列、撤销代数或会话令牌。

## 验证与限制

运行 `go test ./...`；现有 `test-admin-auth.py` 的隔离 AdminAuthentication 用例同时调用新客户身份用例，覆盖 MySQL/Redis、并发单次消费、过期、重发冷却、旧链接废止、重复注册、同邮箱历史记录隔离、撤销和事务晚失败。测试令牌只从隔离测试数据库解密，不存在公开读取令牌的接口。摘要见 `gin-customer-accounts-report.json`。

这次增加独立迁移 0038，未依赖工作区原有未提交的 0037 支付通知迁移。已提交的迁移版本允许有空号；未提交支付改动仍单独保留。联合工作区演练会包含两者，报告不将它们全部算成本次提交。

尚缺：账户页面、资料/地址、购物车登录转移及业务归属、Resend 配置/模板/实际发送及失败恢复。当前邮件表是持久化待处理意图，尚无发送调度，不能显示“已发送”或宣称通知闭环。会话返回值仅供后续可信 HTTP 层写 Cookie，不能直接 JSON 返回。

原前台资料页中的密码更新仍是 TODO；这项已有可见入口的功能也需要纳入前台缺口清单，不能因为保留了表单就计作完成。未发现独立找回密码入口，不能凭一般商城习惯宣称原项目已有该流程。

## 客户 HTTP 接入

生产 API 在 `/api/store/customer` 挂载独立客户接口。默认关闭（合法 JSON 请求返回 503），只有 `CUSTOMER_AUTH_ENABLED=true`、匹配 `SESSION_SECURE` 的 `STOREFRONT_ORIGIN` 及显式 `CUSTOMER_KEY_ACTIVE`/`CUSTOMER_KEYRING_JSON` 才会开启。密钥配置拒绝重复键、空值、大小写非规范 hex、零密钥及无效活动键；不能用旧账户或 Cookie secret 作为加密密钥。密钥环保留轮换前的密钥用于读取尚未处理的加密意图。

| 方法 | 相对路径 | 行为 |
| --- | --- | --- |
| POST | `/auth/challenge` | `{}` 取得一次性预登录 CSRF，设置 HttpOnly prelogin Cookie |
| POST | `/auth/register` | 邮箱/密码/名/姓/电话/国家/语言，新建待验证客户与邮件意图，返回 202、`verification_required` 和 `delivery: pending` |
| POST | `/auth/verify` | `token` 在 JSON 中提交；消耗验证链接，不给打开链接的浏览器自动登录 |
| POST | `/auth/resend` | 校验邮箱/密码/国家/语言，执行 60 秒冷却及旧链接失效，返回待发送 |
| POST | `/auth/login` | 验证密码与验证状态；轮换会话，撤销本浏览器旧会话 |
| GET | `/me` | 从独立客户会话读取当前客户安全投影 |
| POST | `/auth/logout` | `{}` 加当前会话 CSRF；撤销并清除客户 Cookie |

注册、验证、重发、登录均需先取得一次性预登录 CSRF；失败或成功消费后须获取新的 challenge。所有 POST 必须携带精确 Origin 且 Host 匹配，拒绝跨站 fetch；GET 同样拒绝外站 Origin/跨站 fetch。所有接口禁止查询字符串携带凭据，响应为 private/no-store、no-referrer。JSON 限 16 KiB，拒绝重复/未知/null 字段、多个文档及无效 UTF-8；全局 1 MiB 提前拒绝仍保留私有响应头。

本地 HTTP Cookie 为 `retail_customer_*`，HTTPS 为 `__Host-retail_customer_*`；Path=/、无 Domain、SameSite=Lax，session/prelogin 为 HttpOnly，csrf 供双重提交读取。不会接受 admin/cart Cookie 代替客户会话。按实际直连 IP 和规范邮箱使用独立 Redis customer realm 限流，未信任任何入站 X-Forwarded-For；Astro bridge 现使用独立密钥签名客户地址（见下文），不会采用任意 forwarded header。

`python retail/scripts/test-admin-auth.py --customer-accounts-only` 包含真实 Gin 的注册至退出用例、跨站/Host/重复 Origin/重复 Cookie 拒绝、一次性预登录 CSRF、登录轮换、管理员 Cookie 拒绝、严格 JSON/UTF-8/大小限制、邮箱/IP 限流和默认关闭状态。测试令牌仅在隔离夹具内从邮件意图解密；没有公开测试后门。报告 `http` 字段为 HTTP 证据，仍明确客户页面与实际发信未集成；Astro transport 的正式路由验证单独记录。

## Astro 客户传输层

`retail/storefront/src/lib/customer-bridge.ts` 复用已有 native Gin private transport。仅允许固定 customer 路径；只转发当前 HTTPS/HTTP 命名空间下的 prelogin/session/csrf Cookie、浏览器提供的 Origin/Fetch-Site 和规范 CSRF，不转发管理员、购物车、Medusa 凭据或任何 forwarded/client-IP 请求头。请求上限 16 KiB、响应上限 64 KiB、超时 10 秒，拒绝重定向和所有查询参数。

返回 Cookie 根据 challenge/login/logout 动作校验完整集合、作用域、Secure/HttpOnly/SameSite/有效期。错误响应仅允许清除已消耗的 prelogin；不会丢失“待验证”状态或 429 重试时间，也不会转发上游数据库错误与任意响应头。拒绝成功 JSON 意外出现密码散列、会话、验证令牌或加密载荷字段。

现已挂载 `/api/store/customer/[...path]`，必须配置两端相同的 server-only `CUSTOMER_BRIDGE_KEY` 才开启；缺省返回私有 503。账户页面尚缺，不能记成浏览器注册闭环。

验证：新增 5 项 customer bridge 单元测试（含真实 native HTTP 重定向/Host 测试）；前台合计 83 项全部通过。Astro check 检查 197 个文件，无 error/warning，保留原有两个 unused hints；本次没有改动页面或样式。见 `astro-customer-bridge-report.json`。

## 可信客户端地址与正式 API 路由

两端配置独立的 32 字节 lowercase hex `CUSTOMER_BRIDGE_KEY`，不得与业务加密密钥或旧系统密钥共用。生产 Gin 启用客户认证时强制要求该密钥；未验签请求不能触及账户逻辑。Go 底层 factory 保留无 key 的直连测试模式，生产 `cmd/api` 不采用该模式。

Astro 只签入 adapter 提供的连接地址，不复制浏览器的 `X-Retail-*` 或 forwarded 字段。协议为 HMAC-SHA256，输入绑定固定版本、秒时间戳、IP、方法、原始请求 URI、公共 Host、Origin、客户 Cookie、CSRF、正文 SHA-256。Gin 限定过去 30 秒/未来 5 秒窗口、规范时间/签名/IP，并将 IPv4-mapped IPv6 归一为 IPv4 后用于 Redis 限流。协议不缓存身份、不授权登录；重放仍受原 CSRF 一次消费/当前会话及限流约束。Go/Node 共用固定向量核对编码。

源码核实当前 Astro Node 适配器在 Host 验证成功时会读取 `X-Forwarded-For`。当前没有可信反向代理配置，正式客户 route 因此明确拒绝包含该头的请求，再使用 clientAddress；不将攻击者指定的地址签成可信身份。未来部署代理必须单独建立实际可信拓扑，不能直接删除这个校验；当前不操作部署服务器。

验证分为两层并各自记录边界：隔离 Gin/MySQL/Redis 的注册/验证/登录/退出 HTTP 用例现在全部携带签名，并校验签名地址下的限流；`tests/customer-runtime.mjs` 启动独占本地构建产物和 HTTP oracle，验证正式 Astro route 的 native transport/HMAC、adapter 地址、Cookie 过滤、伪造 forwarded IP/跨站 Origin/查询参数拒绝，退出后清理两个进程。后者没有真实 Gin/MySQL，不能把两层结果描述成已运行端到端浏览器注册。实际账户页面、发信、地址和交易归属仍是下一步。
