# Gin 正式启动支付配置与私有可用支付方式

`cmd/api` 已支持显式 `CHECKOUT_PAYMENT_ENABLED=true`，使用独立 `PAYMENT_KEY_ACTIVE`/`PAYMENT_KEYRING_JSON`、`MOLLIE_API_KEY`、`STOREFRONT_ORIGIN`、`MOLLIE_BACKEND_ORIGIN` 和商店绑定注册 hosted Mollie 适配器。默认不注册；关闭时不读取支付密钥。启动只构建适配器，不请求付款、不写数据库、不激活 SQL 开关。说明及无真实密钥的占位配置见 backend/.env.example。

前台返回地址固定为所配置商店源的 `/api/mollie-return`，回调源为所配置的 API 源。拒绝用户密码、路径、查询、片段及远程HTTP；生产环境两者均须HTTPS。错误为固定文案，不含原始配置。付款加密密钥必须持久保存并与需读取的付款信封匹配，不与客户或管理员会话密钥混用。旧支付/订单通知信封仍引用的密钥不能删除。

私有 `GET /api/store/carts/:id/payment-options` 在一个只读事务核对当前商店/渠道/地区/凭证代次、checkout SQL开关以及已注册代码。返回 `{cart_id,version,providers:[{id}],attempt_started}`，仅当前地区绑定且启用的 hosted Mollie 可出现。停用或解绑返回空列表；已开始付款返回 `attempt_started=true` 和空列表，不能诱导另开付款。版本保持字符串。此读取不冻结购物车、创建付款或保留库存。

HTTP 仍使用现有 HttpOnly possession、Origin、限流与 private no-store 边界，不续期Cookie，不输出密钥、远端支付标识、付款链接或客户信息。Astro私有桥及CartClient已接读取并严格校验购物车、版本和响应字段；不通过公开目录请求转发私有Cookie。

验证：独立源码快照不包含未提交0037支付通知代码，47项SQL迁移；`go test ./...`通过，真实隔离MySQL/Redis的OrderRead及CartRuntime通过。新增用例覆盖配置开关、密钥/源拒绝、地区绑定/渠道启停、错误代次、SQL禁用、冻结购物车、Origin/凭证和隐私响应。前台134项测试通过，Astro295文件0错误0警告。

后续[Astro付款及复核步骤](astro-checkout-payment-page.md)已接按钮、记录清理和冻结后页面恢复，并通过合成API浏览器验证；PayPal和PayPal Card未由此开关注册。通知URL沿用现有适配器契约，但HEAD中Mollie通知收件与后台消费仍未启用，不能将生成URL视为通知闭环。原有0037通知工作区改动本次未提交。当前只验证隔离适配器及数据，未调用真实支付沙箱，未声称完成全站购物闭环。SQL checkout/completion及库存、订单编号等运行条件需单独准备，本配置不会自动开启它们。
