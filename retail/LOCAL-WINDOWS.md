# Windows 本机启动

本机还设置 `LOCAL_PREVIEW_FULFILLMENT_ENABLED=true`：start.bat显式开启现有设备配货、装包和登记发货操作。配置本身不生成设备或修改库存；仍需原界面登记有效的可售设备，并满足实收、SKU、仓库和身份校验。已在独立订单副本通过平台Admin配货/装包/发货验证，主订单保持未发货，见 `docs/migration/local-fulfillment-main.md`。

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

## 本机仓库用户与授权范围

恢复库的 warehouse_user_runtime 和 warehouse_scope_runtime 原本关闭，仓库用户接口因此503。当前本机显式执行 `go run ./cmd/configure-local-preview --warehouse-users`，只允许既定development/loopback专用数据库并核对版本1；重复执行不创建员工、不授予仓库或改变库存。忽略.env中 `LOCAL_PREVIEW_WAREHOUSE_USERS_ENABLED=true` 使start.bat下次重复应用；示例默认false。实际仓库账号仍须由管理员通过现有页面创建和授权。

独立 `python retail/scripts/test-local-customer-account.py --scenario=warehouse` 已通过员工限定仓库的配货调整、释放取消、重新装包发货及客户物流通知；主库无测试员工/设备。详情见 `docs/migration/local-warehouse-main.md`。

## Mollie本机测试付款

本机忽略的 `.env` 现配置 `CHECKOUT_PAYMENT_ENABLED=true`、`MOLLIE_LOCAL_TEST_WITHOUT_WEBHOOK=true`、`LOCAL_PREVIEW_PAYMENT_ENABLED=true`、真实 Mollie test key 和独立持久 PAYMENT keyring。start.bat 显式执行 `configure-local-preview --payments`，只允许专用 development/127.0.0.1:13307/eurorefurb_local_trial；启用本地成单与库存预留并初始化独立测试编号，重复执行不重置编号。退款、外发邮件和后台worker仍关闭。

Mollie无法访问loopback webhook，本机test模式省略该参数，通过真实收银台返回后的服务端认证查询确认。live key或非本机地址不能使用该配置。已实测订单 `2026110218344757728`（789欧元），详情及限制见 `docs/migration/local-mollie-shopping.md`。生产编号切换与webhook需要独立正式配置。

## 通知邮件独立验证

`python retail/scripts/test-local-customer-account.py --scenario=mail` 复制本机恢复库，在专有19100/19200/19300及Redis前缀验证实际后台邮箱保存/密钥留空保留、注册邮件链接验证登录和订单失败重试。副本商户密钥临时生成；外部Resend传输由单独测试进程替换，未向外发信，不需要旧机私钥或真实Resend凭据。主环境邮件开关保持原状。详情与真实收件缺口见 `docs/migration/local-notification-mail-main.md`。

## 已创建付款的地区/语言保存

`CHECKOUT_CONTEXT_PAYMENT_ENABLED=true` 独立于普通付款开关，当前本机已显式启用。start.bat 调用 `configure-local-preview --payment-context`，限定既定development专用库及Mollie测试配置，启用版本1的付款恢复runtime；缺少配置拒绝启动，重复执行不修改付款或订单。只启用普通付款时，已付款上下文接口会保持关闭。

`python retail/scripts/test-local-customer-account.py --scenario=payment-context` 在副本验证真实Mollie付款创建后的英→中→英连续保存、成功响应丢失与刷新原操作恢复，再沿同一付款完成订单。保留忽略的最终SQL快照；`--verify-payment-context=OWNER` 可仅恢复快照复核终态并清理，避免因检查条件修正重复付款。真实测试渠道会被调用，详情及未覆盖分支见 `docs/migration/local-payment-context-main.md`。

`--scenario=payment-context-changed` 增加副本价格变化、旧会话退役与790 EUR新付款成单；`--resume-changed-context=OWNER` 从已有成单检查点验证本人订单，不再付款。该分支的 `--verify-payment-context=OWNER` 还会认证GET核对两条远端引用；不发起渠道写入。旧open且不可取消的Mollie链接只是本地退役，不能宣称远端取消或免除对账。见 `docs/migration/local-payment-context-changed.md`。

`--scenario=payment-context-restore` 在副本注入最终保存SQL约束失败，实际验证旧会话已处理后刷新、恢复原设置与替代付款、丢恢复响应再刷新，以及替代付款789 EUR成单。可用相同 `--verify-payment-context=OWNER` 复核保留检查点，包含只读渠道GET。测试故障触发器仅限副本指定购物车，主价格/单据不变。见 `docs/migration/local-payment-context-restore.md`。

## 本机商品评价

`LOCAL_PREVIEW_REVIEWS_ENABLED=true` 由start.bat调用 `configure-local-preview --reviews`，限定既定development专用库及版本1，启用评价读取/客户提交/后台审核，不创建或批准任何评价；SQL即时生效。示例默认false。

`python retail/scripts/test-local-customer-account.py --reviews-from-checkpoint=OWNER` 恢复已有真实已付款订单的专有SQL检查点，实际验证本人订单提交、重复拒绝、后台审核和中英公开展示，副本结账入口关闭，不再付款。检查前后九张交易/库存/价格保护表一致，导出指纹排除全实例GTID元信息。见 `docs/migration/local-product-reviews-main.md`。

## 已付款检查点继续仓库发货

`python retail/scripts/test-local-customer-account.py --warehouse-from-checkpoint=OWNER` 从已真实付款、原客户归属、尚未配货的检查点继续管理员装包与仓库员工调整/释放/发货、客户物流和通知。付款创建关闭，不再调用渠道，也不重新建立订单归属。预留按源副本的实际基数净减1，保留其他订单占用。`--verify-paid-warehouse=OWNER` 对该次执行保留的前后SQL独立核对，不重放业务命令。见 `docs/migration/local-paid-warehouse-main.md`。

本机API访问日志新增 `request_context=active|canceled|deadline_exceeded`，可区分已取消读取和有效请求故障，原HTTP状态保留。隔离运行器可用 `ACCOUNT_TEST_API_BINARY` 指向 `logs/retail-native` 内已构建的.exe，单独验证新API而保持主服务可用。见 `docs/migration/warehouse-request-cancellation.md`。

## 换机在线补款

本机忽略.env显式配置 `ORDER_PAYMENTS_ENABLED=true`、`MOLLIE_NOTIFICATIONS_ENABLED=true`、`LOCAL_PREVIEW_ORDER_PAYMENTS_ENABLED=true`。start.bat调用专用库限定的 `configure-local-preview --order-payments`，构建并启动一个原生补款worker；再次启动会核对精确可执行路径并停止记录的旧worker，防止没有端口的后台进程重复运行。示例默认false，退款/邮件不随此启动。worker PID与日志位于 `logs/retail-native/order-payment-worker.*`。

`--supplement-from-checkpoint=OWNER` 从已付款原客户订单继续真实发货/换机/差额Mollie沙箱付款/替换机发货；会产生一笔新的沙箱补款。`--verify-supplement=OWNER` 只恢复该次保留检查点并运行完成状态worker一次验证无重复账务，不重跑交易。细节和限制见 `docs/migration/local-exchange-supplement-main.md`。

## 本机退款worker

本机忽略.env当前为 `LOCAL_PREVIEW_REFUNDS_ENABLED=true`、`ORDER_REFUNDS_ENABLED=true`，取代此前退款worker关闭的阶段状态。start.bat以Mollie测试配置显式启用v4退款gate并启动一个原生 `order-refund-worker.exe`；重复启动核实旧PID精确路径后先停再启。PID和日志为 `logs/retail-native/order-refund-worker.*`。此配置会执行今后正式提交的沙箱退款指令，不能将pending显示为成功；邮件worker不随之启动。

`python retail/scripts/test-local-customer-account.py --verify-completed-refund=OWNER` 只恢复已完成真实退款检查点，用当前worker --once核对十表不重复写入，随后清理副本；不重建付款或退款。说明见 `docs/migration/local-refund-runtime.md`。

## 通知邮箱配置入口

当前本机已启用 `NOTIFICATION_MAIL_ENABLED=true`、`LOCAL_PREVIEW_NOTIFICATION_MAIL_ENABLED=true`，新独立持久MERCHANT keyring保存在忽略.env。start.bat校验该配置及已有邮件设置，不创建邮箱、不启动发信worker。可在 `http://127.0.0.1:9200/mailboxes` 配置Resend发信账户、查看订单邮件任务和原失败重试入口；主库目前空邮箱/1条pending订单邮件。注册和订单邮件worker仍关闭，未发送真实邮件。详情及只读启动验收见 `docs/migration/local-mailbox-configuration.md`。

### 商品录入入口

本机忽略.env的LOCAL_PREVIEW_PRODUCT_ENTRY_ENABLED=true使start.bat显式开启既有商品创建、SKU创建、基础价录入及新增仓库库存命令。只允许专用development本地库；不重置ERP序号、不生成测试商品或库存。访问http://127.0.0.1:9200/products操作；真实写入验收使用`python retail/scripts/test-local-customer-account.py --scenario=product-entry`副本，勿在主库制造测试商品。

### 仓库及设备来源入口

LOCAL_PREVIEW_WAREHOUSE_MANAGEMENT_ENABLED=true使start.bat显式开启既有仓库管理，独立于仓库员工开关。要求专用本地development库及既有设备身份/命令已就绪；不创建仓库或设备。后台http://127.0.0.1:9200/warehouses可管理仓库和设备来源。设备登记保留原独立数量库存语义；新入库验收使用`python retail/scripts/test-local-customer-account.py --scenario=device-entry`隔离副本。
