# PayPal 扣款／授权持久命令

2026-11-02 C06 接续。新增 `POST /api/store/carts/:id/paypal-settlement`，继承购物车凭证、Origin、CSRF、限流与严格空 JSON 校验。订单、商户、金额、币种及 CAPTURE/AUTHORIZE 意图均从服务端原创建收据及配置恢复，客户端不能指定。

0104 仅新增 `cart_paypal_settlements`，先事务提交唯一执行占位，再调用渠道。稳定36字符请求编号绑定 provider、collection、session、远程订单及意图；同订单并发和重试共用原记录。执行中返回 processing；响应丢失标记 uncertain，随后只查询恢复，不重新发送扣款／授权 POST。查询仍无交易明细时保持 requires_review。已核实结果加密保存，并通过原观察流水更新购物车状态，未新增订单、库存或实收账目。

渠道适配器先 OAuth→GET 复核订单和精确金额，仅对已批准且没有交易明细的订单调用 capture/authorize。共用八秒期限、固定主机、拒绝跳转、禁用请求体自动重放；完整授权或扣款证据直接返回。对 POST 返回继续核对原身份及交易明细，单凭顶层 COMPLETED 不算成功。

## 验证

`go -C retail/backend test ./...`、cart/payments/identity 的 `go vet` 通过。PayPal 专项121条 pass 事件（含父测试）通过：钱包／卡、授权／扣款、pending／完成、错误结果及丢响应边界。渠道 HTTP 测试使用受控传输，既有 OAuth／查询测试覆盖本地 TLS；没有真实 PayPal 请求。

`node retail/scripts/test-context-payment-joint.mjs --paypal-checkout` 在一次性 MySQL 9.6（原生外键）／Redis 和真实 Gin HTTP 中验证四个既有创建场景，以及四个钱包扣款／卡授权正常与丢响应场景。每车六路重放仍仅执行一次；加密收据、权限拒绝及订单／库存／实收／通知指纹通过。渠道为人工适配器，未执行 Astro 或真实 PayPal。

`--scenario continuous --matrix` 对原 Mollie 连续保存／替换／恢复流程通过实际 Gin／Astro／MySQL／Redis 浏览器中英×390/1440四组回归。两次验收均核对冻结源码并清理专属资源，见[报告](paypal-settlement-report.json)。未跑完整业务链。

本机 Docker MySQL 在完整逻辑备份后显式升级：首次 applied=1/skipped=103，重复 applied=0/skipped=104。备份保存在忽略的 `logs/retail-native/`，未声称已演练恢复。`start.bat` 已更新原生服务，9100/9200/9300检查通过，付款／退款／外发邮件仍关闭。

## 剩余

原 SDK 钱包／卡界面、client 配置／token、webhook、未知创建发现、取消／退款及 PayPal 专用成单记账继续待办。当前主配置未注册 PayPal；已有 Mollie 成单入口继续拒绝消费 PayPal 证据。C06 和 C01—C11 未完成。
