# 事务内促销刷新编排

2026-10-29：`cart.RefreshPromotions`把现有阶段接成一个内部调用：

1. ReadPromotionCodes：当前购物车授权/版本、add/replace/remove及旧有效调整项code。
2. ReadPromotionContext：服务端保存的商品/规格/运费/地址/地区/订单金额等私有规则事实。
3. promotion.ReadActive：本次code和自动促销、当前服务器时间的campaign有效范围、方法及三类规则。
4. ReadAttributeUsages：同一事务的精确邮箱/客户属性预览事实，不允许浏览器提供用量。
5. ComputeCandidates：原优先级、资格、分配及预算反馈逻辑。
6. ReplacePromotionAdjustments：验证动作来源绑定，替换coded调整项和cart_promotions，保留null-code手工项。

输入只有codes/action意图和调用方固定的服务器时间At；浏览器不能提供折扣金额、自动促销集合、规则或属性用量。零At、错误版本/授权、配置或上下文失败均返回零结果；函数不提交事务。现有阶段的领域限制继续适用，特别是新客户/groups和计算raw字段覆盖尚未验收。

该调用是完整购物车命令中的一个阶段，要求调用方在价格/税/配送刷新后执行，之后还需完成支付集合协调、最终金额、版本和持久幂等回执，任何后续失败必须整体回滚。它不自行处理HTTP/CSRF、不启用新命令能力、不更新版本、不执行支付provider、不预留库存，也不登记促销或预算usage。购物车预览和实际checkout的预算登记继续分离。

## 实际数据库组合验证

隔离CartRuntime人工MySQL直接调用RefreshPromotions，配置手动10%优惠MAIN、自动固定优惠5 AUTO和达到使用次数上限的LIMITED。验证MAIN/AUTO共同保存、LIMITED反馈、原手工商品20及运费2调整项保留；商品当前含税金额由225变为193.75。

无新code重复刷新重新计算，不叠加旧coded金额；移除MAIN后AUTO仍生效（商品218.75）；移除AUTO仍由默认自动促销选择重新应用；replace/空replace沿用自动语义；移除后重新添加MAIN恢复原cart_promotions链接ID。未知add失败、inactive已配置code通过存在性阶段但不产生优惠，清空旧coded项回到手工调整后的225。

再将MAIN绑定customer_email属性预算：无邮箱拒绝；保存邮箱后读取真实用量并计算到200；campaign在At恰好结束时优惠清除；恢复有效期后重新添加可计算；实际属性用量达到1时反馈campaign_budget_exceeded并清除MAIN。整个过程促销used、预算used、属性used均未增加，promotion_usage_operations为空。payment schema关闭拒绝；autocommit、零时间、错误版本/凭证拒绝。最后模拟后续命令失败，整体回滚，购物车/库存指纹不变。

报告cart_promotion_refresh记录组合范围及public_http_or_checkout_integrated=false，相关源码SHA必须与报告匹配。原各阶段行为对照、金额/目标和保存的故障回滚用例继续执行；本组合证据不冒充原完整Medusa工作流、公开HTTP或真实checkout验收。

仍需完成完整规则覆盖审计与新客户登录/group关联、支付刷新、持久幂等版本命令、购物车成单/库存预留/支付渠道协调及Astro交互接入。T032/T033未验收。
