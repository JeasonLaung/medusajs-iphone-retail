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

## 正式账户与验证页面：认证局部交付

`/{countryCode}/account` 与 `/{countryCode}/verify-account` 已接入原站公共页外壳与新客户 API。登录/注册表单、切换按钮、原账号文案翻译、验证 Result/Spin、输入与按钮包装器从来源复制；改动集中在 Next/Medusa 数据依赖替换为本地 context 和 Gin client。原 UI 使用较新 Ant Alert 的 title，当前新前台 antd 5 对应替换为 message，保留相同内容结构。未修改旧项目文件或全站样式。

注册返回待验证和邮件待发送；提示不会宣称已发送。重发密码只存在当前 React island 的 client 闭包中，既不进表单结果/JSON props/URL，也不持久化在浏览器 storage；离开页面或成功登录清除。每个写操作重新申请一次性 challenge，丢响应时不自动重试。浏览器验证页在一次 effect 中消耗链接，不自动登录；私有 no-store、no-referrer 和 noindex 防止验证链接成为公开内容。

认证成功后显示实际 API 返回的新客户姓名/邮箱并提供退出；资料、地址、订单、回收等尚未接入，当前显示明确的不可用提示，未制作假统计或订单。这一登录态区域是临时交付，不是原账户 dashboard 的完整迁移。页面采用 client-only island，JavaScript 加载前的登录表单、无 JS 状态、原图像素对照仍需补齐，因此 C02 继续未验收。

`tests/account.browser.mjs` 使用构建后的正式 Astro 页面、Chrome 和合成 API，验证登录/注册切换、待发送提示、重发冷却按钮、一次验证且不登录、登录后读取客户、退出、中英、390/1440 无横向溢出与无浏览器异常。未使用真实客户、未发送邮件，未将合成 API 浏览器检查声称为真实 MySQL 注册闭环。来源摘要见 `astro-account-sources.json`，本轮证据见 `astro-account-report.json`。

## 登录态联动

页头已有账号图标只切换原 `isLoggedIn` 状态，沿用原图标和样式，不增加新导航。配置客户 bridge 后，页头查询当前客户，登录/退出、返回页面和重新可见时重新观察；观察失败保留最后一次已确认的页头状态，401 才视为未登录。完整通知菜单仍未迁移，此处不以登录图标替代通知业务。

`customer-events.ts` 向当前页发出变更事件，并写一个随机 UUID 到独立 localStorage 键供其他标签页触发重新查询。该值不含邮箱、ID、密码、令牌或客户 JSON，也不能提供登录权限。每个订阅者仍调用 Gin `/me`；旧请求晚返回不能覆盖较新的查询结果，卸载后解除监听。账户区依赖失败显示可恢复错误，保留重新查询能力；只有明确 401 才返回原登录表单。

客户观察显式校验新客户 ID、已验证时间和资料字段，过滤意外私有字段，成功响应为空或未验证时视为错误而非“已退出”。新增单元验证此边界。正式页面 Chrome 用例覆盖页头登录/退出、第二个标签页同步退出、依赖故障/恢复、opaque storage marker；87 个前台单元检查与 Astro check/build 通过，手机截图已人工检查无溢出。浏览器仍使用合成 API，尚非真实注册邮件端到端证明。

## 姓名/电话资料写入与正式资料页面

来源 `profile/page.tsx` 实际挂载姓名、邮箱、电话和账单地址。姓名、电话调用原 updateCustomer；邮箱保存是空函数，密码组件在页面中被注释。因此本次承接真实姓名/电话写入；邮箱只读，未继承空操作的假成功。账单地址尚未迁移。没有将未启用的密码组件扩成已验收功能。

Gin `POST /api/store/customer/me` 只接受 first_name/last_name/phone 字符串，省略不修改，空 phone 可清除；拒绝未知/重复/null/非法长度及控制字符。调用需可信 bridge、匹配 Origin/Host、当前 customer session 与 CSRF，并限流。事务锁定该客户后再次核实 Redis 会话和 MySQL 验证/停用/撤销代数；修改目标来自会话，未接受客户 ID、邮箱或旧客户引用。更新不修改认证材料，现有会话保持可用。存储错误返回失败并回滚。

正式 `/{countryCode}/account/profile` 复用原 AccountInfo/Collapse、姓名和电话组件、编辑/取消/保存/反馈，沿用主题和原文案。成功后触发客户重新读取与标签页联动。邮箱只读；原完整侧栏、账单地址与完整账户工作区尚缺，暂未做像素一致性验收。登录根页提供资料入口，但完整 dashboard 仍显示明确不可用状态。

隔离 MySQL/Redis 验证部分更新、空电话、受保护字段拒绝、未知/撤销会话拒绝、写失败回滚和历史客户不变；真实签名 Gin HTTP 验证 CSRF 拒绝、保存结果、空请求/邮箱写入拒绝。Chrome 正式页面以合成 API 验证姓名/电话编辑、失败不改值、重试及刷新持久读取；该浏览器不访问真实 MySQL。89 项前台单元测试、219 文件 Astro check（0 error/warning、2 旧 hints）、构建、Go 全包检查通过；仍不能据此宣称邮件注册至真实交易的完整闭环。

## 新客户地址簿与默认账单地址

0039 显式迁移增加 `customer_account_addresses`，与历史地址分离。Gin 客户接口提供列表、新增、部分编辑、删除；客户 ID 来自当前 customer 会话，公开投影没有所有者、生成列或内部字段。接口仍需签名 bridge、可信 Origin/Host、写入 CSRF、严格有界 JSON，默认标记使用规范字符串 true/false，响应为布尔值。外部/不存在的地址统一返回404，禁止以管理员会话或客户端指定的 customer_id/旧地址引用访问。会话撤销或账号停用后拒绝读取/写入。

每个地址事务先锁客户行，再重验 Redis 会话及当前 MySQL 认证状态。切换默认账单/配送地址时，只取消该客户对应角色的旧默认值，与本次写入一起提交；两个默认角色独立。SQL 生成列唯一索引保证每个客户每种角色至多一个默认地址；并发命令串行化。晚到的插入失败回滚旧默认取消，删除失败不丢地址。删除当前默认不自动挑选其他地址，用户可重新设置。地址为联系人信息，不代表可配送；原选择器包含欧洲国家和地区，即使尚未配置商城配送区域也允许保存，结账必须另查当前配送规则。

正式 `/account/addresses` 复用原 AddressBook、AddAddress、EditAddress 的卡片/弹窗/字段；资料页复用 ProfileBillingAddress。使用原欧洲国家集合、国家名称修正及搜索归一化；沿用原 Ant CountrySelect/Modal 控件和原 SVG 图标，不引入 Medusa 运行时依赖。当前账户区仍为临时双链接导航，完整 dashboard/侧栏未迁移，也未做源页面像素一致性验收。

原删除包装函数会吞掉失败，本次让失败显示在卡片下并保留地址。React 表单动作返回错误也会触发 reset；国家选择器现在阻止该自动重置，以保留失败草稿供重试，取消/关闭时由编辑器重建恢复初始值。地址写成功才触发重新查询；切换客户重建地址组件，旧请求不会覆盖新客户的地址。

隔离 Gin/MySQL/Redis 验证当前客户CRUD、前导零邮编、部分更新及空电话、跨客户/未知地址一致拒绝、字段和CSRF/Origin检查、8个并发默认切换、物理唯一约束、晚失败回滚和删除失败回滚。Chrome 正式 Astro 页面使用合成API验证新增/编辑/删除、取消不写、失败保留草稿并重试、刷新读取、默认账单创建/再次编辑，以及中英与390/1440宽度；与真实SQL证据分开记录，尚不构成真实注册邮件→结账浏览器联合闭环。

本批还将暂存区后端和既有来源夹具导出到独立目录（38份迁移，不含工作区未提交的0037），通过 Go 全包检查及独占 MySQL/Redis 的账户/资料/地址/Gin HTTP 演练。客户专项报告以该独立快照为证据；主工作区包含39份迁移的额外验证不作为本批提交的依赖。完整浏览器仍为合成API，邮件及结账联合验证仍未完成。

## 注册验证邮件发送与恢复（局部交付）

0040 显式迁移给新客户验证队列增加次数、首次尝试、下次尝试、租约、加密发送快照、提供方观察及固定错误码，并增加数据库全局节流行。既有历史邮件不启用、不发送。原 `lib/registration-email.ts` 中英主题/正文/一次性15分钟说明由 Gin 发送器继承，API固定为 Resend `POST https://api.resend.com/emails`，8秒超时、不跟随跳转、不自动重试单次请求；未知服务商正文和网络错误详情不进入日志或公开投影。

`customer.DispatchVerification` 每次最多处理一封。客户行→队列行→节流行按固定顺序锁定，重查当前验证ID、收件邮箱、摘要、有效期、账号启用与未验证状态，再持久化一次尝试及30秒租约。完整JSON请求用独立purpose/row绑定加密保存；首次请求后重试解密原字节，不重新渲染模板。sender指纹绑定Resend key、from、reply-to与模板版本，未确认请求不能在更换发送账户/配置后直接重放。数据库节流使不同进程每秒最多取得一次发送资格。

提供方请求在事务外执行，固定幂等键 `mail/{cv_id}`。服务商成功后SQL失败，租约到期可用原键/原请求恢复；最多3次尝试，临时失败至少等一分钟且链接届时仍有效，永久拒绝直接失败。首次尝试超过23小时停止重放（Resend官方幂等窗口为24小时，沿用原业务23小时保护）。结果写入比对租约，验证或重发已取消的队列不会被迟到结果复活。外部请求进行中无法撤回已发出的邮件，旧链接仍因当前摘要/消费状态失效。sent仅表示Resend接收，不代表收件箱已送达；投递回调尚未接入。

显式命令：在 `retail/backend` 使用 `go run ./cmd/send-verification-mail --once` 处理最多一封，或不带参数作为每秒检查的worker。需显式 `CUSTOMER_VERIFICATION_MAIL_ENABLED=true`、独立MySQL、已应用完整迁移、CUSTOMER加密keyring、STOREFRONT_ORIGIN、REGISTRATION_RESEND_API_KEY与REGISTRATION_MAIL_FROM，可选REGISTRATION_MAIL_REPLY_TO。缺配置直接退出；API启动不会启用worker。当前仅环境变量引导发送人，后台邮箱配置/失败查看恢复UI仍待实现，不能视为C10完成。开发示例保持false，本轮未读取真实密钥或向真实客户发送邮件。

隔离MySQL/Redis使用人工sender验证失效链接拒发、加密请求、固定幂等键、首次成功但本地记录失败后的原字节恢复、3次上限/一分钟间隔、不同邮件共享节流、永久错误终止、23小时保护、发送人变更拒绝、8个并发worker仅1次provider调用、发送进行中验证后迟到结果不复活、缺解密key失败关闭。真实签名Gin HTTP从注册写入队列，经人工sender获取邮件链接，再走验证与登录；无直接SQL取token捷径。Resend传输单元检查固定URL/headers、状态分类、中英文案、无跳转/无自动重试，使用内存transport，不发生外部发信。真实收件、Astro浏览器→邮件→验证联合验收与后台配置仍缺。

提供方依据：[发送接口](https://resend.com/docs/api-reference/emails/send-email)、[幂等键和24小时窗口](https://resend.com/docs/dashboard/emails/idempotency-keys)。这些规则只约束提供方重放，不取代本地验证有效期和业务状态检查。

本批提交再次导出暂存区独立后端（39份迁移，不含未提交0037）并通过Go全包检查与独占MySQL/Redis账户/资料/地址/邮件/HTTP演练；报告以独立快照为准。本批不改前台布局，不将合成sender描述成真实Resend送达。
