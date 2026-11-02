# Windows 本机启动

**2026-11-02最新：本机现已开启用户提供test key的Mollie测试付款，并完成购物至订单确认/后台读取。** 下文早期“付款关闭/付款方式503”属于此前排障记录；当前测试付款配置与范围见文末。

仓库根目录双击 `start.bat`，启动当前 Gin / React Admin / Astro。MySQL 按用户要求使用 Docker；Redis 使用本机服务，不启动旧 Medusa。原启动入口保留为 `start-medusa.bat`。

准备 Go、Node ≥22.23.4、Docker Desktop 和本机 Redis，以及 `retail/admin`、`retail/storefront` 的 npm 依赖。现有 MySQL 5.7 不能承载此项目的迁移结构；本项目通过 `compose.local.yml` 使用专用 MySQL 9.6 容器和持久卷 `eurorefurb-retail-local-mysql`，监听本机 13307，不覆盖其他项目的 3306 数据库。

本机连接与密钥写在 Git 忽略的 `retail/backend/.env` 和 `retail/storefront/.env`，可参考各目录 `.env.example`。backend 使用 `APP_BIND=127.0.0.1`、`APP_PORT=9100`、`ADMIN_ORIGIN=http://127.0.0.1:9200`、`STOREFRONT_ORIGIN=http://127.0.0.1:9300`；storefront 使用 `GIN_API_ORIGIN=http://127.0.0.1:9100`、相同 `STOREFRONT_ORIGIN`。MySQL 使用项目专用数据库，Redis 使用本机无密码6379和独立 `SESSION_REDIS_PREFIX=eurorefurb:windows-local:v1:`。迁移必须显式执行 `go run ./cmd/migrate`，启动脚本不会自动改表或开启外部付款/邮件。

启动先通知旧开发监督进程退出，再关闭 9001、8000、9100、9200、9300 的旧监听进程，保留本机 MySQL/Redis。脚本编译当前 Gin，再依次启动并检查：

- API：http://127.0.0.1:9100/health
- Admin：http://127.0.0.1:9200
- 商店：http://127.0.0.1:9300/dk-zh/store

进程在后台运行，日志与 PID 保存在 `logs/retail-native/`。再次执行脚本会关闭旧项目监听后重新启动；启动失败会指出对应日志文件。可通过 `RETAIL_NODE_EXE` 指定 Node；当前系统版本过旧时也会检测本机已有的 Codex Node runtime。本次新建后台账号 `admin@eurorefurb.local`，随机密码保存在 Git 忽略的 `.private-files/local/admin-credentials.txt`；原 Medusa 的 `.admin-credentials` 不适用于新后台。

本机已从 `mysql-schema102.sql.gz` 恢复专用库 `eurorefurb_local_trial`，显式执行0103字段迁移，并在完整逻辑备份后应用0104 PayPal持久命令表，重放确认0条待应用（共104）。备份存于忽略的 `logs/retail-native/`，尚未演练恢复。数据库密码及新本机账户加密密钥仅保存在忽略文件中，旧机的私有包没有恢复；历史加密配置及未携带的上传文件可能不可读取。真实付款、退款及外发邮件保持关闭。换机请携带数据库、对应私有配置及必要文件，不能仅依赖本脚本恢复数据。

## Windows Redis 空闲连接

## 本机购物主流程配置（2026-11-02）

恢复备份后的运行开关默认关闭，单看健康接口200不能证明能够加购。当前本机 `.env` 设置 `LOCAL_PREVIEW_CART_ENABLED=true`，`start.bat` 会显式运行 `go run ./cmd/configure-local-preview --enable`，仅为 development、127.0.0.1:13307、eurorefurb_local_trial 启用草稿购物车创建/访问/商品/地址/配送/上下文/优惠命令；不改变通用 schema 写入开关，不启用付款、退款、库存预留或发信。缺少迁移或版本不符时启动失败并给出提示。

本机 `MANUAL_SHIPPING_PROVIDER_IDS=manual_manual` 注册原库已有的手动配送渠道。浏览器已从 `/gb-en/products/swappie-iphone-17e` 实际加购至购物车、填写英国地址、选择 Standard Shipping，再进入付款页：商品779欧元、配送10欧元、总计789欧元。此前创建购物车503源于 cart.create/cart.access 关闭，配送空列表源于该手动渠道未注册。

当前付款方式接口仍503，因为本机支付配置关闭；付款至订单尚未跑通，不能宣称整体购物流程完成。优先推进用户可操作主链路，主链路跑通前不继续扩展支付细节。此前未完成的 PayPal 成单修改保存在 Git stash `wip PayPal completion deferred for local main-flow repair`，尚未应用到本机运行代码。

本机 Redis 的空闲超时为65秒。项目客户端在30秒时退休空闲连接，避免 Windows 闲置后首次请求因旧连接关闭而短暂503；不会自动重试会话写入，也不会修改共享 Redis 的服务器设置。专项说明见 `docs/migration/windows-redis-idle.md`。

## Mollie本机测试付款

本机忽略的 `.env` 现配置 `CHECKOUT_PAYMENT_ENABLED=true`、`MOLLIE_LOCAL_TEST_WITHOUT_WEBHOOK=true`、`LOCAL_PREVIEW_PAYMENT_ENABLED=true`、真实 Mollie test key 和独立持久 PAYMENT keyring。start.bat 显式执行 `configure-local-preview --payments`，只允许专用 development/127.0.0.1:13307/eurorefurb_local_trial；启用本地成单与库存预留并初始化独立测试编号，重复执行不重置编号。退款、外发邮件和后台worker仍关闭。

Mollie无法访问loopback webhook，本机test模式省略该参数，通过真实收银台返回后的服务端认证查询确认。live key或非本机地址不能使用该配置。已实测订单 `2026110218344757728`（789欧元），详情及限制见 `docs/migration/local-mollie-shopping.md`。生产编号切换与webhook需要独立正式配置。
