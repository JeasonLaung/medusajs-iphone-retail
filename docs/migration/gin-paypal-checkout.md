# PayPal 购物车创建命令与持久收据

2026-11-02 C06 接续。上一项 PayPal adapter 只验证渠道 HTTP。本项将创建接入已有购物车短事务→持久 effect claim→事务外渠道调用→加密结果→原购物车再授权/激活流程，复用现有表，不新增 migration。主配置仍只注册 Mollie；没有启用实际 PayPal 或对外请求。

`POST /api/store/carts/:id/payment-attempts` 接受可选 `provider_id`。省略时保持旧 Mollie 语义；显式选择仅允许三个已知入口，还须同时存在服务端 adapter、数据库启用及当前地区关联。cart possession、CSRF、Origin、版本和冻结检查保留，浏览器不能提供金额、商户或渠道订单 ID。

服务端将选中 provider 和币种结算规则保存到原加密请求、payment session 和原 CREATE 收据。原 UUID 重放先读加密请求，比对所选渠道，不能利用同编号将钱包换成卡或 Mollie；省略 provider 的旧命令也不能消费 PayPal 收据。新 UUID 不能绕过现有 checkout freeze。输入版本4/冻结后5保持不可变。

渠道创建成功仅返回 `sdk_ready`、`provider_id`、`provider_order_id`；`checkout_url` 为 null。原订单内容和商户私有信息不回显，订单 ID 供后续原 PayPal SDK 使用，并非已付款证明。未知结果返回 requires_review，重放查询原 effect，不再次 CREATE。Mollie 原 ready/checkout_url 投影不变。

## 独立验证

```powershell
node retail/scripts/test-context-payment-joint.mjs --paypal-checkout
```

复用固定人工购物车夹具，只运行 Gin HTTP→MySQL 9.6/Redis 子流程，省去 Astro 构建和完整订单/配货链。创建专用容器、Unix socket MySQL、容器回环 Redis，无真实渠道凭据；验证后按 owner 标签清理容器/网络/匿名卷。

四组钱包/卡×成功/未知结果通过，每组含六路并发原 UUID 重放，渠道 CREATE 均一次。验证原请求和结果加密、session provider、金额158.75 EUR、原版本和激活标志、禁止同编号换渠道、禁止新编号绕冻结；无效 Origin/CSRF/持有凭证在 attempt 创建前拒绝。库存、预留、订单、实收和通知全表指纹不变。渠道采用人工 adapter，真实 PayPal OAuth/Orders TLS 已由上一项独立验证，本项不能替代两者连通后的联合验收。

原 Mollie `unchanged` 和同一车 `continuous` 中英×390/1440浏览器回归另见[报告](paypal-checkout-report.json)。Go cart/payments/customer/identity/checkoutconfig/cmd-api 局部测试及 cart/payments/identity vet 已通过。

浏览器首轮 `79d16867900d` 在中文目标 URL 与当前 URL 相同的情况下，URL 等待提前完成、持久意图尚未清除，断言失败。改为显式轮询持久意图已清除，重新从头运行 `9dfdb0fa60f0` 八组通过；首轮失败报告保留，没有拼接为通过证据。

## 尚缺

当前 payment-options 和主 checkoutconfig 仍只开放 Mollie；PayPal 需后续 SDK 配置/token/钱包及卡 UI 接入，服务端 capture/authorize 和认证 GET、webhook/退款、未知创建发现与上下文取消协调。PayPal 返回订单 ID 不代表这些流程完成。没有运行完整交易链，C06 与 C01—C11 均保持开放。
