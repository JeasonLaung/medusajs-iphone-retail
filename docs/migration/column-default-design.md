# 列默认值转换设计

T006/T012 的元数据证据和逐列设计，尚非 MySQL DDL 或已实现命令。`capture-column-defaults.py` 用 REPEATABLE READ READ ONLY 查询 information_schema 的列默认值、identity/generated 标记，不读取业务行；与已有字段清单逐项核对193表1,961列一致。机器证据为 source-column-defaults.json，目标去向为 column-default-design.json。

源603列有默认表达式，没有identity列或generated列。这与自增号无关：源使用nextval默认的sequence另行处理。字段去向设计覆盖159业务表1,691列和34排除/重建表270列；不为排除登录/运行时表创建新业务默认值。

| 目标处置 | 列数 | 规则 |
| --- | --- | --- |
| 无源默认 | 1,172 | 导入须字段存在，NULL按映射判断；非空新写入必须显式提供，不补空串/零 |
| UTC时间 | 300 | 保留导入时间；新命令使用一次权威UTC时钟，DATETIME(6)；updated_at显式更新，不用ON UPDATE覆盖历史或重放时间 |
| 精确文本/枚举 | 93 | 保留大小写与空文本；状态默认不替代合法状态迁移检查 |
| 布尔 | 57 | 明确新写默认；enabled=true不允许在导入演练启动外部任务 |
| 精确数字文本 | 39 | 原lexeme与单位保留，拒绝float转换；raw/value双列默认必须一致 |
| 精确JSON文本 | 22 | 保留有效JSON文本，不双重编码，不使用隐式native JSON数值转换 |
| 持久编号分配 | 5 | 保留旧展示号/顺序；新写用持久计数锁，不用MAX+1或直接auto_increment替代 |
| 布尔单例主键 | 2 | order_number_clock.id、sku_number_counter.id必须true且只有一行；不是随机ID/按用户多行计数 |
| 空文本数组 | 1 | notification_provider.channels旧'{}'::text[]是空数组，新精确JSON为[]，不是字符串花括号或NULL |
| 不导入 | 270 | 无目标列，不恢复旧登录、个人授权或运行任务 |

所有条目的candidate_new_write_value仅是已识别候选；领域命令可要求字段显式提供，不能机械生成完整默认INSERT。JSON如raw金额零的precision声明须结合金额映射，不能用schema默认产生和主数值不一致的raw值。NOW()与CURRENT_TIMESTAMP在PostgreSQL表示事务时间，新业务须明确同一命令时点，不把旧数据重写成导入时间。

## 编号特别处理

源有 order.display_id、order_change_action.ordering、order_claim.display_id、order_exchange.display_id、return.display_id 五个业务nextval。订单还有项目自定义编号触发器和order_number_clock，不是仅靠序列：仍需采集触发器定义、各sequence当前位置及写入路径，和19位上海时间编号契约一起核对。不能因display_id声明nextval就抛弃项目编号逻辑。

两个单例计数器的布尔true主键保留，必须结合CHECK和唯一约束，事务中锁定原行并推进。新ID生成、旧ID保留、原展示号、ordering序列和SKU业务编号是不同事项。当前没有在MySQL创建或调用这些计数器，也未改变源sequence位置。

## 可重现验证

```powershell
python retail/scripts/capture-column-defaults.py --check
python retail/scripts/map-column-defaults.py --check
python retail/scripts/test-column-defaults.py
```

捕获脚本核对完整列集合；设计脚本核对六批映射与非导入集合恰好互斥覆盖全部列，未知表达式/新identity/generated拒绝。10项人工测试涵盖无默认不补零、排除表、布尔单例、精确大数、JSON数字lexeme、SQL文本引号、空数组、sequence、未知函数/强制类型和generated。测试不证明MySQL执行行为。

报告只包含元数据默认表达式，源配置默认值不等于实际秘密值。未查询业务行或凭据，未执行任何源默认表达式；nextval只作为文本捕获，未消耗编号。字段清单与源元数据漂移需审查再更新，不能自动放行新增列。

仍缺完整触发器/函数、sequence位置和并发规则、全部约束/索引的目标DDL、MySQL空库/导入/新写入演练，以及现有库存/孤儿差异对账。T006/T012继续未验收；此前Gin草稿DDL不能因这批设计自动转为可用迁移。
