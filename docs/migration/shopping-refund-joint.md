# 新购物订单到退货退款与本人通知

`CHECKOUT_REFUND=1` 连续使用四个新注册、验证并实际购买/付款/发货的账号，运行真实 Astro、Gin、React Admin 与专用 MySQL/Redis。没有预造申请、退回、检测、退款、现金流水或通知，没有替换业务 API 响应；外部付款页、首次付款观察、邮件投递及退款渠道是隔离替身，不代表真实 Mollie 收款/退款或真实邮件收件。

客户本人选择原发货设备申请退货退款；平台受理、按原 SN 扫描收回、上传私有检测照片并检测通过。员工在现有退款抽屉选择原付款，提交精确 EUR158.75 退款。申请/收回/检测/退款的已提交响应被丢弃，实际页面重试原 UUID/正文。客户随后看到退款进度、售后完成及本人通知，并从通知打开真正的本人订单详情；中英文 × 390/1440 各一单。跨账号、同浏览器切换账号、退出及无购物车凭证私有读取仍使用原联合检查。

## 实际独立 worker 与渠道边界

脚本从当前源码构建 Linux `cmd/process-order-refunds` 二进制，只复制到本轮自有容器；每次以 `--once` 启动独立进程，读取原加密付款请求和显式默认关闭的退款配置。本联合不验证常驻模式的长期信号/退出管理，也不打开真实运行配置。

worker 使用真实 Mollie 适配器、固定 `https://api.mollie.com` 路径、证书主机验证及原 UUID。只有这个测试子进程配置本轮生成 CA 与本地 CONNECT 代理；代理仅接受该固定目标，TLS 证书含正确 SAN，所有响应都来自隔离渠道。生产代码没有测试 URL、关闭证书验证或替换商户的开关。原付款 GET 核对 session/cart/模式/金额/币种，可退余额通过后才 POST。

首个退款在渠道侧保存成功结果后丢失 POST 响应；原进程退出，新进程等待实际45秒SQL租约到期，以列表/单笔GET发现同一退款，不重发POST。故障触发器随后使退款通知 INSERT 失败，实际 worker 事务回滚现金退款、负交易和通知，保留已验证的渠道观察。清除本轮故障后，再一个新进程从观察恢复本地入账。四笔渠道 POST 总数严格为4，每单一笔；没有修改SQL租约、伪造业务状态或执行真实付款来通过测试。

## 退款通知与订单金额

显式0085只扩展站内通知 kind CHECK。退款已验证观察之后，在现金退款/负交易/支付集合/当前订单汇总/applied同一事务调用既有 EmitOrderTx，事件键为 `refund.created:ref_<operation UUID>`。收件人只来自已验证新账号的订单 ownership，不按原订单 email 或历史customer_id 推导。通知没有商户/远端引用、检测照片或内部员工信息。

通知投影/中英文标题接入既有组件；订单通知详情从旧 `/account/orders/:id` 修正为实际 `/account/orders/details/:id`，回收通知原路径保持。点击仍先执行受客户会话和CSRF保护的已读，再打开本人私有订单。没有修改主题或布局。

退款命令沿原设备退款规则允许正额、不超剩余实收；不会自动免除剩余运费或改写订单金额。联合原生退回只移除商品行金额，仍保留8.75运费，退款则按员工输入158.75入账；这不是订单剩余差额全部清零的证明。非零换机/其他补差结算仍单独待完成。

## 验证与剩余范围

脚本退出0，四个账号浏览器错误0、后置SQL与完整 TestOwnedCartRuntime 通过，自有容器/网络/临时二进制及浏览器清理完成。后置SQL验证每单唯一本人申请/收回/照片检测、唯一 applied退款158.75、唯一负交易-158.75、当前汇总退款158.75、原设备returned且检测后sale/available、本人四条已读通知及其中唯一退款通知。新增数量库存净扣4、预留净增0；检测不回补数量库存。

生成报告 `shopping-refund-joint-report.json` 绑定124份相关源码SHA，已逐项匹配。实际客户中英手机/桌面截图及Admin截图位于忽略的 `logs/gin-migration/refund-*.png`。其他历史联合报告仍是各自当时源码快照，不伪造更新为此次验收。此结果不关闭整个 C06/C09/P10 或 C01–C11；常驻worker生命周期、真实渠道/收件、非零换机补差、非序列号商品退款及其他前台/数据缺口仍需继续。

运行：

```sh
CHECKOUT_PRODUCT_BROWSE=1 CHECKOUT_FRESH_CART=1 CHECKOUT_ACCOUNTS=1 \
CHECKOUT_FULFILLMENT=1 CHECKOUT_REFUND=1 \
STOREFRONT_TEST_NODE=/opt/homebrew/bin/node \
ADMIN_TEST_BROWSER='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' \
python3 retail/scripts/test-customer-orders-live.py --checkout
```

此模式与维修/换机模式互斥。源码中的测试故障只作用于自有数据库，失败也按所有权清理；运行前需已有本机 MySQL/Redis 镜像、Docker、Go及前后台依赖。

完整同版回归：23组专用SQL、85个显式迁移及清理退出0；Go全包/相关vet、通知投影2项测试和Astro check（395文件、0错误/0警告）通过。主SQL报告413份、退款SQL报告108份与本联合124份源码SHA逐项匹配；原health/Admin/store均200，既有可选locales 404不影响商城页面。
