# 当前促销配置事务读取

2026-10-29：`promotion.ReadActive`从正式MySQL关系读取内部候选配置，要求调用方事务与可信时刻，不创建事务、不提交、不消费额度、不提供HTTP。请求包含明确优惠码与是否纳入自动促销，最多100个输入码，按原大小写精确匹配且不trim、不转大写；重复码合并，未知/停用/软删除码缺席。活动开始包含、结束不包含，活动软删除不再匹配。同一调用统一使用At。

## 配置与规则

返回源ID、code/type、自动与含税标志、次数限制/已用、可空campaign、可空应用方式、可空预算及订单规则。应用方式保留fixed/percentage、items/order/shipping_methods、可空allocation/currency/件数参数及精确raw优先金额。没有有效method时返回nil，由后续原编排分支跳过，不能伪造each或零价方法。候选按精确应用金额降序、同值按源ID稳定返回；buy-get优先级仍在后续计算编排完成。

活动预算投影保留spend/usage/use_by_attribute/spend_by_attribute原类型、精确used、可空limit/currency/attribute。spend_by_attribute只保留配置，不能据此启用计算；预算登记入口继续拒绝。读取不加载promotion_budget_usages，不包含客户属性使用值、旧身份凭证或来源metadata。返回值是内部配置，仍不可直接充当公开DTO。

分别从order/buy/target关联装配规则，数据库value文本以JSON字符串传给现有MatchRules，前导零和完整文本不丢失。每个作用域复制独立规则值，修改一份不影响另一份。先读原链接，再要求每个链接有活动rule和非空活动values；软删rule或全部值失效时整体报错，避免INNER JOIN悄悄删除限制后按“无规则”放行。操作符、路径、范围阈值、raw、数量等非法配置均拒绝，不返回部分候选。

设定促销/应用方式/预算各1000，规则1000，每类链接5000、全部规则值10000、单规则1000值及文本/路径长度上限。查询取上限+1以明确拒绝超限，而非截断权限或优惠范围。所有读取沿用调用方快照；之后登记/结账仍必须锁后校验当前事实。

## 证据

`verifyPromotionActiveRead`作为专属PromotionSchema人工实库suite的一部分运行。`python retail/scripts/test-admin-auth.py`完整报告中的promotion_read与implementation SHA为权威证据；只在报告含该项且SHA匹配时算通过。验证原码大小写、自动/手动合并、时间边界/软删除、raw超18位、null限制、三类规则和前导零、复制隔离、缺方法、失效限制/空值/非法范围/raw/负件数拒绝、未知预算类型保持、关系溢出拒绝、schema开关/autocommit、RR快照并发更新一致性。该suite使用只读事务调用读取函数，不消耗预算，不访问真实订单/支付。

尚缺真实购物车可信上下文组装、order/item/target资格与买赠排序组合、预览预算反馈、调整项保存、整个购物车版本/幂等命令与Astro接入。读取有效候选并不代表促销可应用或checkout已完成；T032/T033未验收。
