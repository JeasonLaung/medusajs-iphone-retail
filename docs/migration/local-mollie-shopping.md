# Windows 本机 Mollie 测试购物主流程

2026-11-02，按用户“先主流程再细节”的新优先级，在当前恢复数据、本机 Gin/Ant/Astro、Docker MySQL 9.6 和原生 Redis 上验证。使用用户提供的 Mollie test key，真实请求 Mollie；不是人工渠道适配器、API 拦截或手工改 paid。

实际浏览器从 `/gb-en/products/swappie-iphone-17e` 加购，填写英国地址，选择 Standard Shipping，进入付款/复核，创建 Mollie hosted checkout，跳转远程收银台，选择其 PayPal 测试方式和 Paid，再自动返回本机。服务端 payment-status 认证查询后，complete 和私有 confirmation 均200，确认页显示已付789欧元。商品779、配送10、税0，与原金额一致。

订单 `order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d`，自定义号 `2026110218344757728`。数据库核对一笔 Mollie payment、一笔 capture、一笔 order transaction、一项库存预留、一条成单收据及一条待发订单通知。登录新 React Admin 只读检查，订单列表和详情可见相同订单、商品、金额和付款集合。详见[可提交报告](local-mollie-shopping-report.json)；浏览器截图/状态存于忽略的 `logs/retail-native/`，没有提交 cookie、凭据或远程 checkout token。

## 实际修复与配置

- 原恢复库购物车开关关闭、手动配送未注册的问题已由前一提交修复。
- `MOLLIE_LOCAL_TEST_WITHOUT_WEBHOOK=true` 仅允许 development + test key + 两端 loopback。创建请求不传无法访问的本机 webhook；返回路径仍需认证 GET 核实，浏览器状态不能认定付款。默认及正式环境保留原 webhook。依据：[Mollie create payment](https://docs.mollie.com/reference/create-payment)。
- `configure-local-preview --payments` 显式配置本机 checkout/completion 和库存预留，并以历史订单快照 SHA 初始化独立本机编号区间（1000000001起）。这是专用开发库的测试编号，不是生产 sequence 切换证据；生产须按原编号契约审核来源高水位。已初始化时不重置，重复执行通过。
- `LOCAL_PREVIEW_PAYMENT_ENABLED=true` 使 start.bat 重用上述专用开发库配置；测试 key 和独立持久 PAYMENT keyring 仅保存在 Git 忽略的 backend/.env。
- 支付拒绝日志仅输出 HTTP 状态和固定白名单字段类别，不输出渠道响应原文、密钥或客户信息。

局部 payments/checkoutconfig 测试、go vet 和 `go test ./...` 通过；需显式数据库环境的其他 SQL 套件仍按原规则跳过，未声称本轮全跑。新增测试证明省略 webhook 的 live key/远程地址/非development/模糊开关均被拒绝，原请求回归保留。

## 本轮范围

这是本机一次真实 Mollie **测试**购物至订单/后台读取验收。未支付真实资金，没有向客户发邮件；确认邮件仅可靠入队，worker关闭。未执行配货发货、回收、售后、账号完整链、PayPal原生SDK，不能据此关闭C01—C11。此前两个浏览器探针过早退出留下不确定创建记录，保留以供只读发现恢复，未绕过持久记录重发同车付款。后续继续优先其他未跑通的大流程。
