# 配送候选关系查询

2026-10-28，`fulfillment.ReadCandidates`在调用方一致事务内连接销售渠道、渠道仓库、有效仓库、仓库配送集合、配送集合、服务区域、配送选项及类型，按稳定ID顺序返回内部候选。支持一个集合关联多个仓库，全部保留，避免把任意一个仓库当成唯一发货地。缺失/软删除关系均排除；禁用渠道也拒绝提供候选。该渠道禁用约束沿用新店铺可售边界，旧workflow本身只查询关联，属于显式收紧。

地址条件保留原从zip/city/province/country构造OR的行为：只有当前粒度字段存在才生成分支，上级国家/省/市也只加入非空值；空地址不加地理过滤，候选可以用于尚未填写地址的目录观察，不能据此允许保存/成单。所有文本使用完整值和二进制排序规则，不按索引前缀比较、不自动大小写归一或删除前导零。zip使用MySQL JSON_TYPE=STRING与二进制JSON_UNQUOTE精确相等，JSON对象/数组不作正则或范围条件执行。

原行为证据：`node retail/scripts/capture-shipping-geo-query.cjs --check`以实际已安装Medusa模型、buildQuery及MikroORM PostgreSQL查询生成器构造5组SQL，`connect:false`且不初始化应用、不访问数据库/provider；固定源码SHA与原参数在[shipping-geo-query.json](fixtures/shipping-geo-query.json)。它证明原SQL参数为JSON字符串，不等于PostgreSQL执行验收或MySQL资格验收。

加载当前有效选项规则后调用Go MatchRules，默认is_return="false"、enabled_in_store="true"，由协调器显式参数覆盖，额外上下文不能覆盖这两个最终值；支持自定义嵌套上下文。坏配置拒绝整批。最多1000条集合/仓库关联、1000候选、10000规则；超限失败，不截断为不完整可用目录。上下文最多1MiB，地址字段各4KiB。要求履约及地理两个编译模型可读，不在启动建表，不调用provider或写业务数据。

人工MySQL证据纳入admin-auth-report.shipping_candidates：地址粒度OR、缺省祖先、大小写/前导零、JSON对象不当表达式、多仓库、默认和return/enabled上下文、嵌套规则、删除关系/区域/规则、禁用渠道/集合、坏规则、事务与schema边界。具体通过范围以最新报告与实现SHA为准。

**尚非实际配送报价**：返回当前provider启用证据（禁用/缺失不默认为可用），不调用provider、不检查仓库/provider关联、商品shipping profile覆盖或库存，也不拼接固定/计算运价、不保存配送方法。没有新增HTTP或公开data/metadata。购物车持有凭证/版本/店铺归属、完整provider与库存/价格条件、保存和重算必须由后续协调器负责；历史数据导入、前台和完整checkout未因此验收。
