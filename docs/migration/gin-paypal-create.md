# PayPal 渠道订单创建子流程

2026-11-02：C06 的 PayPal 钱包和 PayPal Card 之前只有历史配置导入与金额数据，新结账只注册 Mollie。本项实现两入口共用的创建 adapter，接入已有 `CreateSessionAdapter` / `InvokeCreate` 契约；未注册主结账，前台 PayPal 尚不能付款。

来源为已安装插件的实际 exports 编译文件 `.medusa/server/src/api/store/paypal/create-order/route.js`：先获取 OAuth token，再创建 Orders v2 order；钱包使用 `payment_source.paypal`，卡使用 `payment_source.card`，由客户端 SDK 继续批准/确认。不能拿 Mollie 跳转按钮替代卡组件。来源路径和 SHA 见[本项报告](paypal-create-report.json)。

## 实现

`retail/backend/internal/payments/paypal_create.go` 接受服务端商户凭据、环境、merchant ID、CAPTURE/AUTHORIZE 意图及返回/取消 URL。API 主机固定，两请求共用八秒 deadline，禁止跳转及 HTTP POST 自动重发。凭据、旧渠道订单、私有请求上下文不进入订单 payload。

不可变本地付款 SessionID 同时作为 `PayPal-Request-Id` 和 `invoice_id`，purchase unit 的 `custom_id` 保存 cart ID。服务端金额通过精确有理数处理，超过 JS 安全整数仍无浮点转换。货币集合保留安装版插件支持范围；HUF/JPY/TWD 使用官方要求的零小数，其余为两位，半值向上，只量化渠道结算值，不改 SQL/raw 值。

`Prefer: return=representation` 获取完整订单，逐项复核唯一 purchase unit 的商户、购物车、invoice、币种、精确结算值、reference 和 intent。接受 201 创建及 200 同请求重放；CREATED/PAYER_ACTION_REQUIRED/APPROVED 都只输出本地 pending，买家批准不代表授权/实收。COMPLETED 等结果暂留 uncertain，后续必须验证 capture 明细，不能凭顶层状态记账。

网络异常、重复 JSON 键、超界响应、渠道错误或身份/金额矛盾均为 uncertain，无有效创建结果，不自动重发 POST。需要接入持久 effect/恢复入口，不能在页面失败后换新请求重发。

## 局部验证

```powershell
go -C retail/backend test ./internal/payments -run PayPal -count=1
go -C retail/backend test ./internal/payments ./internal/cart ./internal/checkoutconfig ./cmd/api
go -C retail/backend vet ./internal/payments
```

52 条 pass 事件（含父测试）通过：两入口×两环境×两意图、同键/同 payload、精确金额/零小数、未知结果不重发、认证失败不创建，以及商户/金额/币种/购物车等冲突拒绝。实际本地 TLS 服务验证正确固定主机及显式信任根可完成 OAuth→创建；错主机、不可信证书和跳转均拒绝。其他恶意响应由传输 fixture 验证；没有真实 PayPal 请求、数据库写入或完整订单链。

## 后续

仍需接购物车持久付款 effect/收据、恢复和并发保护，接原 SDK 钱包/卡表单和 client token，补服务端 capture/authorize、认证 GET 观察、webhook、退款及上下文失效协调，再做页面→Gin→MySQL 分段联合。此 adapter 不关闭 C06 或 C01—C11。主服务继续用现有二进制，支付门禁未开启。

官方核对：[Orders v2](https://developer.paypal.com/api/orders/v2)、[OAuth 请求](https://developer.paypal.com/api/make-api-requests)、[币种及零小数要求](https://developer.paypal.com/api/codes/currency)。
