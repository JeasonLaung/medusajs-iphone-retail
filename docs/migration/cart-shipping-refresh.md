# 保存配送方法的运费刷新

2026-10-29：`RefreshManualShippingMethods`在调用方事务中复核当前持有者和版本，锁定至多100条活动方法，按当前购物车地址、地区、币种和服务器manual注册名单重新报价。有效方法保留ID、shipping_option_id、data、metadata和历史关系，仅更新名称、精确raw金额/兼容标量、含税标记及更新时间。缺价、地址不匹配、provider无效和多方法下不再需要的profile方法软删除。零金额是有效报价。

行为参考实际安装`cart/workflows/refresh-cart-shipping-methods.js`：原源码以活动方法数大于1作为清理孤立profile的门槛。单方法即使profile不再被当前商品使用，只要该选项仍有效且有价便保留。本阶段仅为该单方法从当前有效选项解析额外profile，仍受原渠道/地址/provider/价格约束；不改变普通报价或严格配送覆盖入口。无方法时不生成方法，也不因刷新创建新ID。

`node retail/scripts/capture-shipping-refresh.cjs --check`从实际安装workflow提取纯转换函数执行6组人工输入，固定源SHA、完整输入和输出；不启动原应用、不调用provider或数据库。验证零价、单方法孤立profile保留、多方法孤立profile删除、选项/价格缺失、空方法及输入不变，见`fixtures/shipping-refresh.json`。它不代表完整workflow或SQL通过。

人工MySQL证据以`admin-auth-report.cart_shipping_refresh`和实现SHA为准：保存地区变化重选价、方法ID/data保留、含税偏好、零价、缺价/地址不合格移除、多方法清孤立组及单方法保护、后续阶段失败与中途更新故障完整回滚、空方法无写和autocommit拒绝。报告包含该项且SHA匹配才构成通过证据。

当前只承接manual固定运价；动态provider、合计规则上下文、翻译、自定义hooks仍缺。与原流程一样，价格刷新不等于最终库存预留；完整命令仍须协调库存、价格、税、促销、支付失效、版本及持久幂等回执。调用方遇到任何错误必须回滚整个事务。本阶段不提交、不增版本、不调用真实物流、不提供HTTP或checkout。T032/T033未验收。
