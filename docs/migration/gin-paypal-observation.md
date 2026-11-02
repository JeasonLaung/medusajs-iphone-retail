# PayPal 认证订单查询与购物车状态证据

2026-11-02 C06 接续。`PayPalCreateAdapter` 实现已有 `SessionObserver` 接口：认证 OAuth 后只 GET 固定沙箱/正式主机的 `/v2/checkout/orders/{id}`，共用八秒 deadline、禁止跳转，无 create/capture/authorize/退款副作用。使用服务端保存的订单 locator，仍需复核整个订单，不信任浏览器的状态。

## 金额和状态

逐项核对唯一 purchase unit 的订单 ID、配置 intent/merchant、cart/custom ID、invoice/session ID、reference、结算金额和币种。完整原始 JSON 通过重复键/大小验证，以原文数字保存在加密查询证据内。`APPROVED` 只代表买家批准，返回 pending；不能凭订单 `COMPLETED` 认定实收。

CREATED 的完整金额 authorization 返回 authorized，authorization PENDING 仍为 pending。单笔完整金额、final_capture=true、COMPLETED capture 才返回 paid；此处本地 session 状态保持 authorized，实收/成单仍由独立记账事务处理。capture PENDING 返回 pending，DECLINED/DENIED/FAILED 返回失败。已作废的订单/authorization 可返回取消。

多 purchase unit、多 capture/authorization、部分金额、非最终 capture、退款/部分退款和未知状态不推断为可成单的成功；返回需核对错误，保留冻结及先前事实。完整分次扣款/退款历史重建尚未接通，不能把这些状态当作没有发生付款。

`ValidatePayPalObservation` 在持久化前再次核对 adapter 私有投影。配置的商户和意图首先由 adapter 核对，该校验器只供可信 adapter 的私有数据使用，不是浏览器验签接口。

## 接入与证据

购物车 `payment-status` 从原加密 CREATE 结果取得 PayPal order ID、从原加密请求恢复 provider，再通过原 observer 边界调用和保存加密观察行。原始 CREATE 版本与金额不变，返回仍是 pending / authorized / paid_pending_order；不回显订单原文、merchant 或凭据。Mollie 保留原验证逻辑。

```powershell
go -C retail/backend test ./internal/payments -run PayPal -count=1
node retail/scripts/test-context-payment-joint.mjs --paypal-checkout
```

99条 pass 事件（含父测试）通过，包括钱包/卡状态、错误身份/金额/币种/回收状态、非法 locator、重复/超大 JSON、404/跳转，以及正确/错误主机/证书的实际本地 TLS OAuth→GET。没有真实 PayPal 请求。

独立103迁移 MySQL 9.6/Redis 的 Gin HTTP 四状态创建验收继续通过；两种成功入口各经 pending→authorized→paid 三次真实 `payment-status`，查询证据各3行，加密且不改创建版本，库存、预留、订单、实收和通知指纹不变。人工渠道 adapter 提供固定订单，不冒充真实 PayPal 联网。原 Mollie 替代付款 paid/authorized 恢复及连续保存的页面回归见[报告](paypal-observation-report.json)。

## 尚缺

原 PayPal SDK 钱包/卡表单、配置/client token、capture/authorize 持久命令、webhook、未知创建发现、取消协调及实际 PayPal 成单/记账尚未接通。已有 Mollie 成单入口仍拒绝直接消费 PayPal，防止错记 provider。主配置依然没有开放 PayPal；本项不关闭 C06 或 C01—C11，未跑完整业务链。

依据：[官方 Orders v2](https://developer.paypal.com/api/orders/v2)、[官方 Payments v2](https://developer.paypal.com/api/payments/v2)，以及安装版插件 `.medusa/server/src/modules/paypal/payment-provider/service.js` 的 `getPaymentStatus`。纠正源逻辑中顶层状态与实际现金证据混用，以完整 capture 明细为准。
