# PayPal 金额与币种切换的本地会话退役

2026-11-03，承接 C02/C06 付款上下文的保存、恢复及客户接管。原安装插件 `@easypayment/medusa-payment-paypal` 的 `src/modules/paypal/payment-provider/base-provider.ts:715` 中 `deletePayment` 只返回空数据，不调用远端取消；该文件 SHA256 为 `17b7e161ff1688ef8205f91c8451b12633996a1eb491d5b3fe5f944f7e7502f7`。[PayPal Orders v2](https://developer.paypal.com/api/rest/integration/orders-api) 提供创建、读取、更新、批准/授权及扣款等操作，没有对应的 DELETE order 操作。

迁移保留本地清理行为，并增加查询和并发保护。服务端从已保护的原会话读取渠道订单ID，通过原商户适配器 GET 订单，核对商户、购物车、invoice、金额、币种及意图。只有没有 capture/authorization 明细的未结算订单可退役；已收款、待处理capture、授权、退款历史、未知结果或不匹配证据不能被清理。这里的成功只表示本地会话可退役，不表示 PayPal 远端订单取消。

国家/语言变更仍使用既有持久操作和会话效果记录。金额或币种变化时，在正式保存新金额前退役旧本地会话并解除原结账冻结；原创建请求及操作证据保留。查询结果不确定时只对同一原订单重查，不以新 DELETE/创建替代未知结果。恢复原设置需要重建付款时，沿用既有补偿请求的新UUID/会话与原provider，返回同渠道SDK付款状态。私有渠道响应和密钥不进入浏览器日志。

扣款和设置切换共用购物车owner锁。切换先冻结后，新的扣款请求无法取得当前权限；已有的扣款/授权发送记录（包括结果未知）阻止设置切换和本地退役。检查使用当前锁定读取，避免重复读事务的旧快照遗漏已提交发送记录。并发检查在设置事务建立快照后、取得owner前固定屏障，让正式 `SettlePayPal` 提交结果未知的发送记录，然后恢复设置事务。旧实现漏读并错误返回成功（owner `73a6182839ea`）；修复后钱包/卡两组均拒绝设置切换且没有生成退役指令。外部渠道为人工适配器，事务与持久化路径为正式实现。

固定子流程命令（Node≥22.23.4，本次Node24/Chromium）：

```powershell
node retail/scripts/test-context-payment-joint.mjs --paypal-context
node retail/scripts/test-context-payment-joint.mjs --scenario continuous --scenario currency-restore --matrix
```

最终[联合报告](paypal-context-retirement-native-report.json)：

- owner `ad25bd996f48`，并发检查两组及28组实际Gin/Astro浏览器通过。钱包/卡 × 保存/恢复/登录接管/金额变化/币种切换/补偿重建/不确定查询恢复 × 英文390/中文1440。保存或恢复丢失首次成功响应后刷新重试，沿用原指令UUID及输入版本。
- 金额变化保存后实际集合160 EUR、币种切换158.75 CAD，旧会话退役且付款记录清除；未派发新付款或扣款。补偿恢复原158.75 EUR时原provider保持，唯一替换创建、唯一有效会话，浏览器重新显示同渠道钱包/卡表单。原创建版本4/激活版本5保持，补偿指令输入5、结果6。查询不确定时正式查询恢复原订单，没有重派退役调用。
- owner `88a1af075600`，Mollie连续保存/恢复及跨币种补偿的中英×390/1440八组回归通过。所有联合运行均使用独立源码、构建、MySQL9.6/Redis和服务，冻结源码及自有进程/容器/网络/匿名卷清理核对通过。库存、预留、订单、实收、通知出站的完整行指纹保持。截图和原始日志留在忽略目录。
- 28组适配器状态测试确认只有OAuth及原订单GET，金额/商户/订单不匹配、404和网络失败均不能视作成功；已收款、待处理capture、授权及失败交易历史不会被丢弃。配置只复用显式启用的PayPal creator；未启用的方法不注册退役能力。payments/cart/checkoutconfig/identity/cmd-api Go测试及265项前端单测通过。无新增页面或样式修改。

早期 owner `25e9a21517ba` 是测试断言将 `requires_review` 误当错误，修正后运行；`09a819eaa801` 已通过28组浏览器，新增并发检查后发现并修复上述旧快照缺陷。最终证据使用修复后的源码，不以早期通过掩盖并发问题。

本机通过原 `scripts/start-retail.ps1` 受控加载最终后端，9100 health、9200 Admin及9300/gb-en/store均200。无持有凭证的结账按既有行为404；带凭证路由由联合覆盖。近期API/前台/付款及退款worker日志无新增异常；9001旧Medusa保持停止。

本机主预览仍使用 Mollie，没有开启 PayPal。真实 PayPal 沙箱、3DS、webhook、实际外部退款与整期 C01—C11仍需继续，不能以本地会话清理宣称远端撤销或全站验收完成。
