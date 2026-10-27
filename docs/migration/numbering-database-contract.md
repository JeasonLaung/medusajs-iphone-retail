# 编号触发器、序列及接续规则

T005/T006/T012/T013 的当前数据库证据。`source-numbering-database.json` 通过只读系统目录查询采集2个项目非内部触发器、对应2个函数的实际定义、8个序列参数及所属列；不执行函数/nextval/setval。当前序列last_value/is_called与单例计数值只写入Git忽略的logs/gin-migration/numbering-positions.json，不输出具体位置。报告 [numbering-database-report.json](numbering-database-report.json) 只有聚合检查和来源哈希。

## 当前生效触发器

| 表与触发器 | 生效事件 | 必须迁移的实际规则 |
| --- | --- | --- |
| order / order_timestamp_number | BEFORE INSERT，每行，enabled=O | 原子UPDATE单例order_number_clock，tick=max(last_tick+1,floor(epoch(clock_timestamp())×100000))；上海时区YYYYMMDDHH24MISS+5位tick余数，写custom_display_id |
| product_variant / product_variant_erp_sku | BEFORE INSERT 或 UPDATE OF sku，每行，enabled=O | 空/NULL/全空白生成ERP+(last_id+1+10000)并推进单例；清空合法旧ERP SKU时保留旧值；显式合法ERP号将计数抬升，不回退 |

订单`display_id`仍是另一条整数sequence，`custom_display_id`才由项目触发器生成19位业务号；前台/后台消费区别必须保留。原trigger对INSERT无条件覆盖custom_display_id，不能在目标历史导入时执行对应新号分配，否则会改写旧号和推进时钟。导入历史与新命令必须使用不同的受控写路径，不能靠客户端随意开关绕过新号约束。

当前order_number_clock已是10微秒tick；旧迁移曾乘10的升级不能在Gin导入再次运行。新命令用整数除余生成秒/5位尾数；旧函数仅对整数秒转double再to_timestamp，不能把整19位号经JS Number/float保存。

SKU函数识别`^ERP[0-9]{5,18}$`且数字部分>10000为合法已分配身份；修改空白的旧合法号保持原号、不推进计数。普通自定义SKU原样保留；手工ERP号包含前导零时，保存文本不应丢零，但计数占位按数字部分。计数初始化扫描包括软删除行，不能以当前可见商品最大值重新从头分配。显式ERP分支使用GREATEST，绝不降低已占位值。

若SKU计数行缺失，自动分配的RETURNING INTO STRICT会失败；显式ERP占位UPDATE没有STRICT，不能以源流程“没报错”推断持久计数存在。新Gin要求单例已初始化并受事务保护，缺失、达到int64上界或加10000溢出均明确失败。新操作的稳定幂等/版本检查仍需避免重放创建多次推进。

## 序列分类与导入种子

| 源sequence | 目标处置 |
| --- | --- |
| order_display_id_seq | 历史整数展示号另保留；新的持久分配器接续，不取代custom_display_id时钟 |
| order_change_action_ordering_seq | 保留动作顺序及分配位置，不能按每订单从1重排 |
| order_claim_display_id_seq | 申诉展示号持久分配接续 |
| order_exchange_display_id_seq | 换货展示号持久分配接续 |
| return_display_id_seq | 退回展示号持久分配接续 |
| link_module_migrations_id_seq、mikro_orm_migrations_id_seq、script_migrations_id_seq | 3个旧运行迁移序列不导入，不伪造新MySQL迁移账本 |

当前8个序列均increment=1、cache=1、非循环；定义包含实际数据类型/上下限及所属列。序列next取决于is_called：false时下一次为last_value，true时为last_value+increment。若目标存“最后已分配”，种子至少为max(源is_called?last:last-increment，所有导入行历史最大值)；目标下一次再加increment，必须检查上界。不要无条件last_value+1，空序列会跳过首号；也不要只用MAX，因为回滚/删除/预留号会令源位置高于存量。

计数表与sequence的事务语义不同：PostgreSQL nextval不随事务回滚退号，表行UPDATE则随事务回滚。Gin持久分配器要保证不重复、历史保留及业务审计；不得承诺绝对无缺号，也不能通过倒退源位置“补号”。未来目标allocator的提交/回滚语义必须明确并测试，不假定MySQL auto_increment天然等价。

## 已运行检查

29项：8个序列定义，5个业务序列的下一位置高于固定快照历史最大值，2个当前单例不低于固定快照，10个人工tick的实际SQL格式表达式，4个is_called独立预期。无阻断发现；3个旧迁移序列明确hold不导入。

人工SQL从实际已采集order函数提取格式表达式，SELECT人工CTE的tick并对照Python独立整数/UTC+8计算，覆盖同tick、时钟回退、秒进位及大时间值。tick分配的GREATEST使用人工输入，未执行实际UPDATE、INSERT、触发器；不证明并发、锁、回滚或完整SKU行为。SKU条件与分支目前为实际函数源码证据，不能因读取定义而当执行通过。

```powershell
python retail/scripts/capture-numbering-database.py --positions logs/gin-migration/numbering-positions.json --check
python retail/scripts/audit-numbering-state.py --bundle logs/gin-migration/source-20261026.json --positions logs/gin-migration/numbering-positions.json
```

捕获首次去掉--check；--check校验定义不漂移，并重新观测当前位置到忽略目录。序列读取非事务一致，即使在REPEATABLE READ里，其他正常写入也可能推进；当前位置比对的是较旧固定快照，非正式切换种子。切换前须在源写冻结后重新采集位置、全部导入行及文件/外部任务状态。此阶段不操作部署服务器。

剩余：隔离数据库执行完整触发器与SKU边界/并发/溢出，MySQL分配器和事务/失败恢复，正式写冻结种子与全域对账；T001—T007及后续实现门槛仍未整体通过。新前台保持现有订单号、SKU文案和交互，不能以迁移为由重编号或改变历史展示。
