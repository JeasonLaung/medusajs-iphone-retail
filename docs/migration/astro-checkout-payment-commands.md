# Astro 首次付款请求与恢复记录

承接 Gin `POST /api/store/carts/:id/payment-attempts`。本项完成私有传输和浏览器命令组件，后续[原付款及复核页](astro-checkout-payment-page.md)已接按钮；本项组件证据不代表购物付款已联合验收。

`CartClient.startPayment` 只提交版本、Idempotency-Key 和 CSRF，金额及支付渠道由服务端确定。响应必须绑定原购物车、原操作和冻结后的精确版本；版本按 BigInt 比较。ready 仅接受 HTTPS Mollie 主域或子域、无用户密码且标准端口的链接；processing/requires_review 必须没有付款链接。网络或响应损坏属于结果未知。

`createPaymentCommands` 在请求前可靠保存原 cartID/version/operationID。刷新、超时和重复点击沿用原编号与冻结前版本，不因读到新版本而生成新操作。未知结果、ready、requires_review 均保留记录，跳转失败不会另开付款。记录不保存地址、邮箱、密钥、CSRF 或付款链接。未完成的购物车/地址/配送请求、损坏记录、其他购物车及存储失败阻止发起；正式浏览器命令使用现有同一 Web Lock。

记录的最终清理和付款步骤UI已在后续页面交付接入；[Gin正式配置与私有可用支付方式读取](gin-checkout-configuration.md)已在后续实现并验证。PayPal及PayPal Card入口仍需承接。不能将本项描述为真实扣款、支付沙箱或完整购物闭环验证。

验证：前台133项Node测试通过，Astro检查295文件0错误0警告。新增7项覆盖私有请求、响应绑定与危险跳转、未知结果原操作重试、刷新恢复、其他未决写入、损坏记录和存储失败。固定API桥只转发购物车Cookie、Origin/CSRF和操作编号。此次未修改页面、样式或后端，未调用真实支付服务。
