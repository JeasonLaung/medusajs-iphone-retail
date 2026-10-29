# 促销、活动与预算正式结构

2026-10-29：版本化迁移0031从已核对的完整DDL/字段计划生成10张促销域表，承接75个来源字段：promotions、application_methods、campaigns、campaign_budgets、budget_usages、rules、rule_values及order/buy/target三类规则关联。不触碰其他数据库、旧登录账号、真实优惠券或订单。新Gin启动和health校验编译内嵌的schema SHA与读取开关；启动不自动建表。

`python retail/scripts/generate-promotion-schema.py --check`核对审查DDL、字段范围、依赖父域、所有表及显式枚举约束；生成0031与promotion/schema_plan.json。金额兼容列保留DECIMAL(38,18)，raw金额使用有JSON合法性约束的原始文本，保留更长小数和精度来源；规则值及属性值使用完整文本，不截断、不按前缀比较。保留源ID、时间、软删除、复合主键、有效唯一性和物理外键。预算类型枚举包含spend_by_attribute只用于忠实保留配置，不等于运行计算支持。

原campaign_identifier完整文本唯一性、budget_usage的(attribute_value,budget_id)有效完整文本唯一性两项仍为显式hold：不使用前缀索引冒充完整唯一。后续导入与预算事务须在受控锁下做完整值核对，解决语义唯一性后才可开放写入。通用writes_enabled在数据库约束中固定FALSE；没有通过CRUD绕过预算语义。

## 隔离SQL验证

`python retail/scripts/test-admin-auth.py`的PromotionSchema suite在新建、无外部网络、单核且限内存的专属MySQL/Redis资源内执行，不连接既有业务数据库。证据以最新admin-auth-report的promotion_schema及源码SHA为准；报告包含该项且SHA一致才算通过。

验证31个迁移首次/重放、10表关系、超安全整数与18位标量/超18位raw文本、512字符属性原文、规则前导零、枚举/JSON/布尔/FK拒绝、有效优惠码唯一性与软删除复用、规则删除关联级联、禁用/漂移模型拒绝和写开关不可开启。对已有同名表先升级到0030后插入人工行，再尝试0031，要求拒绝并保留原行，且不留下已启用的部分促销域。

原registerUsage还包含按稳定ID顺序锁promotion与budget、锁后刷新读和按属性使用量重查；原revertUsage与共享预算的细节必须分别承接，不能把金额预算和次数预算混为一个计数器。当前仅完成正式结构与可读取状态校验，尚未完成活动配置导入/读取投影、预算锁内登记/撤销、持久幂等回执、完整购物车命令和管理页面。T032/T033仍未验收。
