# Windows 真实 Mollie 连续上下文保存与恢复

2026-11-02，新增 `python retail/scripts/test-local-customer-account.py --scenario=payment-context`，在当前 Windows 恢复库副本、原生 Gin/Astro、独立 Redis 前缀和真实 Mollie test key 上验收。主库没有新增测试账号、订单或付款。复用既有真实付款助手，在首次付款 ready 后插入连续上下文验证，再沿原托管付款和订单归属路径完成。

发现两个实际配置缺口：`CHECKOUT_CONTEXT_PAYMENT_ENABLED` 未启用，使已创建付款后的地区/语言操作立即503；启用后 `cart_payment_refresh_runtime` 仍关闭，协调事务仍503。新增严格限定 development、127.0.0.1:13307/eurorefurb_local_trial 的 `configure-local-preview --payment-context`，要求有效 Mollie test 配置、loopback 无webhook测试模式、显式上下文开关及可注册取消适配器；事务检查 runtime 版本1后启用，现有草稿配置仍需已准备。没有变更全局 schema 写门禁或任何业务单据。`start.bat` 读取该开关并显式重放，示例默认false。

成功浏览器 owner `6d4e233dd255`：注册及验证新客户、登录、779 EUR 商品加购、英国地址和10 EUR配送、首次创建真实Mollie付款。移动390页面从英文切中文，再切回英文；每次上下文保存先实际成功，再丢弃响应，检查本地持久意图，刷新页面后用相同 UUID、版本和内容重试。恢复后意图清除，原付款意图保持不变，购物车版本5→6→7，总额始终789 EUR。回到结账页继续同一远端收银台，选择测试Paid后服务端认证查询并唯一成单、自动归属新客户。

新订单 `order_f9e94559-a93a-425d-976c-72aef0250151` 仅存在于副本；最终本人订单页可读，浏览器错误0。SQL 核对上下文收据2、checkout attempt 1、成功create_session effect 1、delete_session effect 0、成单收据1、客户归属1。原创建命令 input_version=4、cart_version=5 保留；effective_cart_version=7，成单收据沿原 input_version=4，completed_version=8。没有因版本前进而取消或重新创建付款。

首轮结尾SQL断言误将成单input_version当作有效版本。浏览器实际已完成，因此保留忽略的 `payment-context-final.sql`，通过 `--verify-payment-context=6d4e233dd255` 恢复到新的专有数据库，以修正后的正式断言检查上述事实再清理，不创建另一笔付款。该命令要求同owner的成功浏览器报告和专有快照，只做本地SQL，不访问渠道。首轮定位错误、关闭配置以及一次创建超时的忽略日志也保留；超时仍为uncertain，没有手工改状态或重发原命令。

源账号/回收/售后十二表计数前后不变；专有进程/数据库/授权和20个Redis键清理完成。付款最终快照及浏览器状态仅留在忽略目录，不提交token、cookie、密钥或加密付款内容。[报告](local-payment-context-main-report.json)保留验收事实。Go cart/checkoutconfig及配置命令检查、vet、Python/JS/PowerShell语法检查通过。

当前主本机 `.env` 显式启用上下文协调，通过既有启动脚本一次受控重启；同一配置命令重放成功。原三个服务恢复，退款/邮件worker保持关闭。本次覆盖真实付款金额不变的连续语言保存与丢响应恢复；改地区后金额变化、取消未知与补偿恢复仍以此前隔离适配器证据为限，其他语言宽度、生产webhook及完整C01—C11未据此关闭。
