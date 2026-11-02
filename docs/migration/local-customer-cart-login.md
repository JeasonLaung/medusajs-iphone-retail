# Windows 登录时接管当前购物车

2026-11-03，补 C02/C06 本机实际登录主流程。此前 Astro 的 `PUBLIC_CUSTOMER_CART_TRANSFER_ENABLED=true` 已启用，但恢复库 `customer_cart_runtime.enabled=FALSE`，所以有商品时登录 HTTP 200 后，接管 HTTP 503，页面报错。本机健康接口与空购物车登录不能发现这一问题。

`start.bat` 现在支持 backend 忽略配置 `LOCAL_PREVIEW_CUSTOMER_CART_ENABLED=true`，先执行 `configure-local-preview --customer-cart`，再给启动的 Astro 进程设置匹配的 PUBLIC 开关。命令仍限定 development、127.0.0.1:13307、专用 eurorefurb_local_trial，要求显式客户认证，并锁定/核对 runtime 版本1。缺少迁移或版本错误整笔拒绝；不会创建客户、归属、订单、库存、支付或 worker。示例默认关闭，当前本机已显式启用。原业务参与者、客户表单与样式不变。

来源 `medusa-store/apps/storefront/src/lib/data/customer.ts` 的 login 在保存会话后执行 transferCart；transferCart 只使用当前 cart cookie，没有 cookie 时直接返回。迁移继续采用该语义，不能凭邮箱自动发现另一浏览器的车或历史订单，也不添加没有来源的跨设备自动合并。

## 固定账号检查点验证

```powershell
python retail/scripts/test-local-customer-account.py --cart-login-from-checkpoint=790ce0fc9e71
```

最终 owner `604a7ea46c16` 从已有验证账号/已付款订单检查点恢复专属 MySQL 副本、Redis 与 19100/19300 Gin/Astro。复用账号，只新建两辆未付款匿名购物车；没有再次购物付款或下单，没有业务接口替身或渠道调用。

- 英文390与中文1440真实商品页选中原 SKU 加购，随后原登录表单接管同一辆车。副本先重现 gate 关闭时的登录200/接管503，再显式启用 v1 gate。
- 两组均将首次已提交接管成功响应丢弃，刷新读取持久原意图恢复，同一 UUID/版本/载荷重放；车版本均从2到3，只写一份归属/收据。原商品行、数量、精确合计、币种与语言保持。
- 实际购物车仍显示数量1，两个宽度无横向溢出，无页面脚本错误。退出后客户及购物车 cookie 全部清除，本人订单和原车详情均401。另一个浏览器仅登录同账户，没有 cart 能力，原车/current读取仍401。
- SQL 两份 customer_cart_ownership、两份 customer_cart_operations，购物车邮箱均来自当前验证账户。已有订单/付款/实收/退款/交易/预留/库存/设备/订单归属/订单出站11表指纹不变。
- 主库29表商品设备指纹及12项客户业务计数不变；临时数据库/授权、13条 Redis 键和专属服务已清理，私有 SQL 仍留忽略目录。

早期夹具错误使用了持久日志键、手机隐藏的桌面容器及无凭证404预期，已按原客户端常量、既有移动导航和正式401协议修正。最终重新从检查点完整执行本专项通过，未修改业务规则来适配断言。

本次不重复完整付款/发货/售后；已有付款下的金额变化、未知结果和补偿恢复证据见 [客户付款协调](customer-payment-coordination-report.json)，本项只证明当前恢复库普通登录接管及启动配置。主服务恢复、实际 v1 gate、Go检查和本轮去敏结果见 [报告](customer-cart-login-report.json)。真实邮件收件、PayPal页面、正式数据及本期整站联合验收继续，C01—C11未关闭。
