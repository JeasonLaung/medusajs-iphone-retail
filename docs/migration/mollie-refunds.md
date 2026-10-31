# Mollie 退款接续

本节承接新购物订单维修/换机连续联合之后的实际渠道退款缺口。当前只完成并验证退款适配器；退款业务事务、权限/额度占用、HTTP/worker、Admin 操作和客户完整退货退款联合仍未实现，C06/C09、P10 及全站范围均不标完成。不操作真实付款或外部退款。

## 核对来源

- [创建退款](https://docs.mollie.com/reference/create-refund)：对原 payment 的退款集合 POST，接受金额、说明及 metadata；201 返回独立退款对象。
- [退款状态与丢响应](https://docs.mollie.com/docs/refunds)：queued/pending/processing 与 refunded 分开；连接超时不能认为未创建。状态不等同订单或原支付状态。
- [幂等限制](https://docs.mollie.com/reference/api-idempotency)：当前幂等缓存为一小时，且绑定所用凭据；超时或换凭据后相同键也不能当作永久防重。不能仅凭 UUID 允许未知结果再次 POST。
- [单笔查询](https://docs.mollie.com/reference/get-refund)、[原付款退款列表](https://docs.mollie.com/reference/list-refunds)用于回读和丢响应发现。
- [官方 SDK 的退款对象](https://github.com/mollie/mollie-api-java/blob/main/docs/models/components/ListEntityRefund.md)核对 resource/id/mode/paymentId/metadata/amount/status/createdAt；[官方 SDK 退款示例](https://github.com/mollie/mollie-api-php/blob/main/docs/recipes/payments/refund-payment.md)核对 paid 与 amountRemaining。
- 原项目 `medusa-store/apps/backend/src/workflows/device-refund.ts`：处理中 return、全部选中设备已收回、付款属于本订单、退款正额且不超剩余实收。原 note 恢复不能作为新系统跨付款/金额指令的匹配证据；其浮点容差不继承。

## 已实现的适配器

`payments.MollieCreateAdapter` 新增 `CreateRefund`、`ObserveRefund`、`DiscoverRefund`；`CreateAdapters.Refunder` 只选原显式配置 provider/商户，没有自动注册退款路由或后台任务，也没有数据库写入。

私有 `RefundRequest` 携带原保护请求 SessionRequest、原远端付款、唯一 UUIDv4、订单、售后申请与正额字符串。退款金额使用原两位结算格式，但不允许隐式舍入退款金额，不经过浮点。原请求中完整私有 context/cart/回跳等不会复制进退款请求；仅发送金额/币种、售后说明及稳定操作/订单/申请 metadata。每次创建前先以原商户 GET 核对原支付的 ID、paid、session/cart、mode、原金额/币种及可退余额；缺失或不足余额拒绝 POST。

HTTP 仅固定 `https://api.mollie.com/v2/payments/{id}/refunds`，禁止重定向，限制正文和总调用时间，稳定 UUID 用作 Idempotency-Key。新退款 POST 显式清除 GetBody：Go net/http 在复用连接断开时可能自动重放带幂等头的可重放请求体，恢复必须由持久协调器决定。原付款创建已有同样保护。本机真实 TLS 假渠道先复现两次 POST，再验证修复后只接收一次；没有调用外部 Mollie。

单笔和发现查询独立验证原 paid 支付，不要求尚有未退余额，因此全退后的原结果仍可恢复。发现至多4页×25条，唯一 UUID 候选再独立 GET，核对 refund ID/parent、mode、原样金额/币种、完整 metadata 与创建时间。返回的链接只提供严格受限分页 cursor，重新构造固定地址；外域、其他付款、重复/循环/空分页、缺失字段、歧义或不完整扫描均保持待核对。找不到结果也不允许再次 POST。

返回的 `RefundObservation` 只有远端退款/付款 ID、原渠道状态、精确金额/币种及时间，没有原始响应、metadata 或秘密。queued/pending/processing/failed/canceled 都不能按 refunded 记本地退款。所有错误或 panic 返回统一待核对错误，不暴露渠道响应；HTTP 非201也不伪装退款成功。

## 当前验证

`mollie-refund-adapter-report.json` 由本次实际 `go test -json ... -run TestMollieRefund -count=1` 结果生成，90个叶用例通过，包括金额超界/零/负/精度、商户/模式/原支付/币种/metadata拒绝、六种独立状态、损坏/重复/超大JSON、网络/HTTP/panic、已创建后丢响应以GET找回、两页唯一候选、歧义/不完整历史/外部链接拒绝。真实本机 TLS 验证原付款与退款在已接收请求后断开响应时各只发一次 POST，以及307不跟随。

Go全包测试和 payments vet通过。没有新迁移、没有变更既有UI/样式或生产运行配置；上述90例不是SQL/浏览器退货退款链路验收。

## 紧接的业务接入（仍未实现）

1. 新的显式版本迁移/default-off退款运行配置与不可变操作记录，保存原 actor/order/request/local payment、原保护请求/摘要、精确额度、稳定UUID和外部状态/引用。旧付款不可凭相似金额/邮箱推断远端关联；新单应沿 cart_completion_receipts.payment_id、cart_checkout_attempts、mollie_payment_references 和原效果保护信封核对。
2. 事务中按当前权限、店铺与原已收回申请/付款归属校验，并串行锁住原付款的全部实收、已退和所有未决退款占用；不同 UUID 不能重复退款同一售后申请。原金额/指令不可修改；重放不能改 payment_id 或金额。持久提交认领后才在事务外发一次POST，超时/崩溃后只GET恢复，不能凭租约过期再发POST。
3. 待核对/排队/处理中保持占用；失败/取消只凭权威证据释放。成功观察持久化后，原唯一 payment_refunds、负向 order_transactions、集合退款金额/原精度与订单汇总在同一事务应用；末尾故障可恢复且不重发渠道请求。保留原支付ID与全部历史版本，不能只改状态/退款note。处理中退款还须保护售后审核终态，避免在外部动作未决时撤销申请。
4. 将命令/恢复接入实际Admin售后页，沿用既有Ant提示与不可变重试；客户本人实际查询退款金额/状态及通知。worker须能在浏览器关闭后继续恢复；当前职员撤权不得授权新退款，也不能令已发生的外部退款永远漏账。
5. 新购物已发货订单→本人退货→扫描收回/检测→实际退款操作→queued/pending后refunded→唯一退款流水/汇总→完成申请→客户退款及通知，中英手机/桌面和跨账号拒绝。并发/额度竞争、两次部分退款、响应丢失、本地末尾故障、重启及跨订单/提供方/权限拒绝都需真实隔离MySQL/Redis验证。

真实渠道沙箱/收件、非零换机差额在线收款、仓库专用售后及C01–C11其余验收仍需继续；不能以当前适配器测试取代这些范围。
