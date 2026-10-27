# 购物车、促销、税与配送配置字段设计

日期：2026-10-27。T006 第四批33表321列，累计123表1,275列，剩余70表。机器设计：[JSON](checkout-rules-field-mapping.json)、[CSV](checkout-rules-field-mapping.csv)。旧系统规则行为另见[促销/税/配送契约](promotion-tax-shipping-contract.md)；本文件不表示Gin规则引擎或MySQL已实现。

## 承接结构

| 范围 | 明确保留的源结构和语义 |
| --- | --- |
| 购物车 | cart、address、line_item、shipping_method及各税/折扣行、credit_line、payment_collection/promotion关联；未完成、已完成及在途支付分开处置 |
| 促销 | promotion、application_method、campaign、budget、budget_usage、rule/value、order/buy/target三个规则成员关系；不能压成code/value两列 |
| 税区 | tax_provider、tax_region父子关系、tax_rate、tax_rate_rule；国家provider与省规则继承、is_default/is_combinable保留 |
| 配送 | fulfillment_set、service_zone、geo_zone、shipping_option/type/rule、location→set/provider关系；shipping_profile和price_set分别由前批承接 |

保留每个原ID、时间、软删及未知合法JSON内部字段，按源复合主键而非关联表id猜唯一性。price/库存/客户/支付域仍分别处理；购物车customer_id只对应历史主体，新客户不能凭旧邮箱/旧cookie取回历史购物车。

cart_line_item.quantity源int4保持整数；其他金额主要货币单位，raw_*检查配对。cart税行和tax_rate.rate源float4，与订单税率numeric不同：先保留源float32渲染证据，再精确十进制承接；新税写入使用decimal，禁止套商品尺寸浮点转换。当前快照这些税率表为空，并不证明所有旧数据都能无损装入目标精度，后续人工边界及MySQL往返仍需验证。

promotion_application_method.value依fixed/percentage分别是金额/百分比；budget.limit/used依spend/usage/use_by_attribute/spend_by_attribute区分金额与次数，不能只因numeric就乘除100。预算并发、回滚和属性作用域仍需领域实现。code是业务代码允许191字符，不用ISO code的16字符约束截断。budget_usage.attribute_value可能是客户ID或邮箱等个人信息，只内部处理，不输出日志或公共DTO；保留历史预算使用不能恢复客户登录。

## 活动配置和边界

源约束允许budget.type=`spend_by_attribute`，而本机安装promotion运行JS的查询仅在迁移定义中找到该字面值；已有compute/register逻辑明确分支覆盖spend/usage/use_by_attribute，尚未确认spend_by_attribute的完整运行语义。不能自动按usage或spend处理；后续实现必须先补契约/测试，对未支持类型明确拒绝。当前源budget为空，保留结构不表示该能力已验收。

shipping_option_rule.value是JSON，可为标量或in/nin数组；false/0不得被真假值判空。geo_zone.postal_expression也是JSON，不能直接当唯一邮编文本或任意正则执行。MySQL地址/邮编匹配、条件排序及大小写需要按源行为单独测试；不照搬PostgreSQL GIN/表达式索引。tax_region有parent时要求province_code非NULL且provider_id为NULL，父区国家一致，不出现环；无parent可配置provider。税率优先级和含税价格标记独立。

前台切换配送时，同shipping_profile旧方法替换、其他profile保留；地址变化需重算税/运费/促销和支付版本。历史购物车金额和付款尝试不因导入批量刷新现价，更不能启动旧worker完成未完订单。

## 快照实测与在途边界

```powershell
python retail/scripts/map-checkout-rules-fields.py --check
python retail/scripts/preflight-catalog-mapping.py --domain checkout-rules --bundle logs/gin-migration/source-20261026.json
python retail/scripts/audit-checkout-snapshot.py --bundle logs/gin-migration/source-20261026.json
```

预检4,729字段值、106 raw配对、543引用无失败；[语义报告](checkout-rules-semantic-report.json)63项检查包括购物车订单关联、税区父结构、配送规则值形状、已有预算语义。固定52台账购物车分类为13个completed-history、1个未完成带付款历史、38个未完成且无关联付款集合；未见本地状态为authorized/captured/pending/pending_authorization/requires_more的未完购物车。**本地session状态不是provider远端结算证据**；不能据此承诺没有在途收款。

13个已完成购物车保持订单关联；1个带付款历史的未完购物车保存collection/session/provider引用并进入对账，不能按普通空购物车删除。38个无关联付款集合的购物车仅证明该快照内未发现关联，不恢复旧身份凭证、不默认授予新账号；历史保留期限/最终切换处置另按T065执行。本轮未访问任何支付服务、未清理购物车、未恢复旧会话。

当前有效promotion=0、tax_rate=0；这使某些语义检查没有实际行覆盖。46个人工规则样例用于规则纯函数子集，但完整资格/预算并发/buy-get/provider/多profile流程仍待实现验证；禁止以空表检查通过删除这些模块或声称促销税引擎已完成。

## 后续验收

T032/T033实现独立购物车凭证、版本、归属、精确金额、地址/地区刷新及所有规则；T017/T035落地支付意图、幂等、可靠重试和对账；T012/T018验证MySQL索引、NULL/活动唯一性、原ID关系、JSON精度和差异账本。前三批发现的交易孤儿和库存合计差异继续有效，本批通过不抵消那些阻碍项。
