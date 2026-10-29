# 促销预算撤销事务阶段

2026-10-29：`promotion.RevertUsage`与RegisterUsage复用同一个内部changeUsage，保持促销→活动→预算→属性行的锁序、三秒子context、当前锁定读和全部预检查。它是调用方事务中的算术/持久阶段，不提交、不独立幂等、不提供HTTP。只能传入从原可信登记操作解析的正金额，不能用浏览器金额或直接把重复请求交给该入口。

## 计量与保留

spend合计同promotion所有动作金额后从预算扣除；usage按不同promotion扣一次，同活动多个promotion合计扣除；有limit的promotion.used按不同promotion扣一次。所有计数和金额都保持不小于零，撤销不受“当前已经达到额度”阻挡，避免额度用满后不能释放。精确raw优先，使用同一十进制解析/显式18位兼容标量截断，不经过浮点。

use_by_attribute用完整UTF8二进制属性值在budget父锁下重查，减去不同promotion数；耗尽时软删除当前属性行，保留历史raw和用量证据，重新登记使用新ID。找不到有效属性记录时不创建记录。全局used仍按原分支减并以零封底。相同属性值多条活动记录仍视为配置冲突而拒绝，不任意挑选。

原行为参考当前安装promotion-module.js的revertUsage与revertCampaignBudgetUsageByAttribute_：spend按金额、usage按code、属性记录扣减/耗尽删除、零封底，以及只查询当前有效促销。新实现增加登记与撤销共同锁定/锁后重读，修复并发丢更新；共享活动多code的扣减也按本次不同promotion合计，与登记阶段修复的漏计相对应。源delete改为软删除保留历史，当前用量与额度语义保持。

## 实库证据

`python retail/scripts/test-admin-auth.py`的PromotionUsage suite同时验证登记和撤销，证据以最新admin-auth-report.promotion_usage与源码SHA为准。新增覆盖：达到promotion额度后撤销、同promotion多动作合计扣金额但只减一次计数、零封底、同活动不同promotion次数合计、16并发登记/撤销最终余额不变、属性递减/耗尽软删/重建新ID、旧RR快照后的属性当前锁读、超18位raw逆向扣减、后续阶段失败/预算SQL故障同时回滚属性软删与父预算、autocommit拒绝。该人工隔离MySQL测试不读写真实订单或支付。可使用`python retail/scripts/test-admin-auth.py --promotion-usage-only`只运行同一预算套件，产生独立promotion-usage-report.json；该报告仅证明预算阶段，不替代完整联合报告。测试运行器逐套件输出名称，超时后的资源清理与所有权校验保持一致。联合套件曾在商品价格阶段触发768MiB容器上限；将专属测试MySQL的table_open_cache限制128、table_definition_cache限制400、缓存分区限制1后重新验证，CPU/内存限额保持原值。

## 接入限制

当前登记和撤销都重新校验促销active、活动起止和软删除状态。原revert查询也是活动筛选，但其静默忽略旧code会漏释放；新阶段明确报错，避免误报已撤销。活动已结束、改了预算/属性或软删后，必须依赖后续持久登记台账保存的原目标和delta释放，不能把当前配置当历史事实；这一能力尚未完成。

零封底不是重复撤销保护。调用方仍须在同一个持久操作回执和锁下证明登记存在且从未撤销；现阶段直接重复调用可能释放其他操作占用的额度，禁止对外开放。没有实现回执的全局唯一性、历史登记快照、撤销一次性状态转换或完整结账编排。T032/T033/T035继续未验收，通用促销写能力仍关闭。
