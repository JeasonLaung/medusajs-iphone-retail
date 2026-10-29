# 付款返回核实与原结果页

2026-10-29。首次创建之后，本次接入私有Gin付款状态核实、稳定返回地址及原Astro结果页。付款成单闭环仍未验收。

## 服务器事实与授权

`POST /api/store/carts/:id/payment-status`只接受空JSON对象，拒绝URL查询、状态/金额/提供商/远程ID等字段。当前possession、Origin、CSRF、商店及SQL归属缺一不可，单凭URL购物车ID无权查看。每凭证独立限流90次，API及413错误均private/no-store、不延长cookie。服务器从当前冻结的attempt和不可变加密创建结果取得远程locator，不接受浏览器payment_id。没有确定创建结果时保留requires_review，不重复POST。

短SQL事务重验当前代次、冻结、购物车版本与实际金额、唯一集合归属和session、请求摘要，再在事务外执行已注册adapter的渠道GET。核对远程ID、metadata中cart/session、币种、金额、状态/action/session映射后，将完整观察作为AES-GCM保护的独立证据追加保存。回调URL不能证明付款；paid和authorized是不同事实。观察写入使用已授权外部操作的上下文，原请求取消或后续归属撤销也保留事实，但后续应用与响应仍须重验当前授权。

0045为attempt增加observed_status/needs_review并创建追加观察表。应用状态与证据保存分开，后续事务失败不会擦除已核实事实。已付不会被迟到pending/open降级；已付与终止失败矛盾时保留付款事实并标记requires_review。仅授权不算现金，支付session沿用原映射为authorized但attempt保留实际provider_status。已captured不降级。成功查询既不创建订单，也不预留库存、清购物车或发送邮件。

适配联调发现新cart UUID带连字符，旧Mollie校验只接受下划线。已将其限定字符集扩展到连字符并增加实际createBody回归；仍保持长度/固定前缀及服务端回跳约束。复用原有InvokeSessionObservation边界提取完整观察，该函数此前在未提交0037工作中已存在；本次因购物闭环需要单独提交这个无数据库依赖的公共适配边界，不包含0037收件箱和worker等改动。

## 前台原行为

正式`/{countryCode}/payment-result`复用原PaymentResult组件、CSS、logo、i18n及最多12次、5秒间隔串行轮询。Next Link改原生链接，读取改为独立Gin购物车凭证/CSRF与固定private bridge。未付款/仅授权/paid_pending_order显示原等待态；失败显示原失败文案；查询故障按原节奏重试后报错；非当前购物车直接invalid且不发付款查询。当前尚未成单，前台不会因paid事实伪造confirmed或订单跳转；终止后解冻/重付未接，因此暂不暴露重付链接。

保留`GET /api/mollie-return`固定入口，仅从当前cookie读取购物车及地址国家/locale，303到对应原结果页，不进行渠道查询或付款确认。URL cart_id必须与当前购物车匹配；地址/邮箱不进入island props/HTML。结果页面及回跳private/no-store、no-referrer/noindex，不依赖Medusa。创建支付按钮、API真实渠道配置及订单成功分支仍保持未开放。

## 验证与剩余

- 独立提交快照44迁移、不含0037：Go全包通过；真实Gin/MySQL/Redis人工隔离验证pending重复读取、仅授权、8并发paid、迟到pending不降级、金额不符拒绝且无记录、provider故障不当作事实、观察加密、后续apply失败保留证据、查询后撤销归属保留证据但不披露、相互矛盾状态需审核、严格HTTP和凭证撤销。渠道使用隔离adapter，不是实际Mollie网络/消费；已有Mollie HTTP transport纯测试覆盖GET与真实格式字段校验。见cart-payment-status-runtime-report.json。
- 前台116单元测试、Astro正式构建通过。Chrome正式Astro配合合成API覆盖原地址/配送回归、稳定回跳、paid等待订单、authorized不作现金、失败状态、外部cart拒绝且不出站，中英390/1440无JS错误/横向溢出。已查看中文移动截图；证据见astro-payment-result-report.json。未作原页同数据像素对比，浏览器和SQL专项分别验证，不当作真实浏览器渠道联合验收。
- MySQL单核768MiB、Redis单核128MiB，独占容器无公开端口/外网，运行后已清理。旧开发服务继续可用；旧Next日志已有transformAlgorithm报错仍在，不在此改动范围。

下一步必须完成当前已付事实到唯一订单、精确快照/舍入、共享库存预留、促销用量、必要通知及仓库配货，再接通付款发起按钮和订单成功结果；同时接取消/过期后的持久恢复、真实渠道配置与webhook，不能把本次结果页当作完整购物闭环。
