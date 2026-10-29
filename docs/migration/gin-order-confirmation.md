# 新订单私有确认读取

2026-10-30，C06。`GET /api/store/orders/:id/confirmation` 已提供本次匿名成单订单的私有确认数据。当前仅实现数据读取；Astro确认模板、付款结果页跳转及新客户长期订单归属仍待接入，不能把本批当作完成了确认页面。

身份来自当前Redis购物车possession和同一SQL事务内的拥有权、完成购物车、成单收据、订单关系校验。必须匹配店铺/渠道、有效代际、完成版本和原付款操作，且订单未删除、非草稿、尚未绑定客户。订单ID是路由参数而不是访问凭证。相同邮箱不会关联其他新订单或导入旧历史；未来新客户归属是另一明确业务操作。

响应仅投影原页面所需的订单编号/创建时间/状态/邮箱、收货和账单地址副本、商品展示快照/选项、精确订单金额与配送方式、已记录支付的方式/金额/时间。付款必须匹配成单收据的支付与捕获、订单集合关系，并存在实收时间；不读取或解密渠道data。商品显示从订单快照读取，不因当前商品下架消失。不返回任意metadata、远端付款ID、会话/捕获ID、密文、raw金额、旧客户身份或其他私有业务字段。

请求需要有效购物车凭证，跨站Origin/Fetch-Site、查询参数拒绝；响应private/no-store、no-referrer，不设置cookie或延长会话，独立限流。读取受现有cart.access/订单schema及completion运行能力控制；关闭能力后明确503，不以空订单掩盖依赖失败。

## 验证

独立提交源码快照执行完整 `go test ./...` 和 `python retail/scripts/test-admin-auth.py --order-snapshot-only`。47份正式迁移，不包含本地未提交0037。真实Gin/MySQL/Redis单核隔离容器，无映射端口/外网，按本次所有权清理；支付仅隔离适配器，不涉及真实渠道。

验证新单商品/地址/编号/158.75 EUR精确总额与实收；当前SKU下架后确认内容不变；相同邮箱的其他订单拒绝；拥有权停用、代际变化、Redis凭证撤销、订单删除分别拒绝，运行能力关闭503。检查无渠道payload/metadata/raw/凭证泄漏，禁止缓存及无cookie续期。原订单读取、成单并发/失败恢复、库存/优惠/实收回归同时通过。

报告 [cart-order-snapshot-runtime-report.json](cart-order-snapshot-runtime-report.json) 的 `order_confirmation` 附参与源码哈希。旧开发服务保持运行；原Next日志的transformAlgorithm异常仍存在，本批未涉及旧前台代码。
