# Go快照主键与跨表关联校验

日期：2026-10-27。属于T006/T018的实现组件，不是完整导入器或新业务API。旧目录导入草稿未替换。

## 固定规则

build-import-plans.py从六批字段映射和源约束元数据生成159张表的完整主键及224条关联规则：223项字段关联加1项仅在物理外键声明的order_line_item.totals_id→order_item.id。其中88条带有源物理外键名称；包含DDL审查稿因MySQL CHECK/级联冲突而hold的2条税区外键。字段关联与物理外键必须指向相同源表和源列，不得以目标表命名猜测源列。

每个父关联目标目前均为完整单列源主键，生成器逐项证明；复合外键、非完整主键目标、未映射表/列、未知动作或重复同列外键均拒绝生成，不能偷偷取复合键前缀。159个源主键包括复合连接表、currency.code、region_country.iso_2和两个布尔单例。身份/代码按原文精确匹配，不trim、不改大小写、不作Unicode归一化。复合键以类型标记及字节长度编码，避免拼接或分隔符碰撞。

## 流程与资源

`rowimport.VerifyRelationships(reader, expectedSHA256, RelationLimits)`复用v2完整流校验，收集主键与必要引用值，直到所有159张表、COUNT/footer及外部完整文件SHA256通过后才解析关联并返回证据。表顺序不决定是否能关联：向后引用和循环引用均按完整快照查找。循环存在只证明关联对象存在，不代表分类树/税区树的业务无环约束通过。

主键重复（包括一行软删除）、非法或NULL主键、非法引用类型、结构或hash失败均返回空证据。关联字段SQL NULL按字段nullable验证；非NULL空字符串作为实际键查找。包含软删除的父行是有效历史关联，同时统计resolved_to_deleted_rows。真正缺失一律返回ErrReferences及已经完整验证的快照/聚合证据：physical_missing、missing_on_deleted_rows及history_policy_missing分别计量，不能把历史保留策略当作已通过关系验收。只有ErrReferences允许返回诊断；其他失败不提供部分验收结果。

调用方必须提供正的MaxRows和KeyBytes；前者限制总行数，后者限制保留的编码键/值字节总量，不等同于Go堆上限。映射固定224条规则，额外的map和待解析项随行数增长；大快照应选适当进程限制或后续磁盘staging，不使用无界默认。超限返回ErrRelationLimit。当前只保留身份/引用/软删除状态，不保存整行payload或secret；错误和输出不包含源ID或引用值。函数无SQL、无文件写入、不改变快照。

这里不执行全字段Convert、raw金额对账、CHECK、解密、唯一登记、新账号归属授权或业务写入。需要正式导入器将各项检查与受保护转换、事务、恢复账本组合，不能用单项成功跳过其他检查。

## 验证

Go测试覆盖全部159表主键及224规则、88物理外键名称；复合键拼接/NULL字节碰撞、软删主键重复、向后/循环关联、空字符串/大小写/尾空格/Unicode差异、软删父关联、历史缺失仍失败、资源上限和错误整文件hash。人工v2孤儿流只有完整性通过后才返回ErrReferences诊断，截断/额外行/hash不符不泄漏部分结果。

隔离PostgreSQL脚本在完整159表/3条product_tag人工流上运行该封装，验证主键和流处理；这3行没有非空关联，不能计作224规则的数据库业务覆盖。Python生成器10组测试包括CHECK及外键目标冲突、复合外键/未知动作/父表、非完整父主键和nullable主键拒绝。

## 固定真实快照的离线结果

`audit-relationships.py --bundle logs/gin-migration/source-20261026.json`仅接受Git忽略且SHA256与既有基线相同的快照，调用Go同一关联组件。v1仅用于已证明是文本/布尔/时间的主键、关联及deleted_at列；JSON字段不猜测SQL NULL，没有把v1改写成v2，也未连接业务数据库。

结果见[relationship-snapshot-report.json](relationship-snapshot-report.json)：159张映射表27,140行（其余排除/重建表不送入），43,496非空关联，43,121可解析，2,549个NULL引用，375个未解析值，物理外键缺失0。375项恰与先前库存/设备119项及交易256项缺口一致，11类缺失全部保留；其中119项属于已删除行的明确历史保留策略，另12次缺失位于软删除的交易连接行，均不自动放行。报告包含规则/字段名称、聚合次数和实现hash，不包含业务行。

审计程序退出成功仅表示审计完整执行；报告`relations_accepted=false`明确表示关系未验收。既有缺口仍须逐域历史结构设计、关联证据及对账，不删除历史、不补造实体、不按邮箱把历史授予新账号。

```powershell
python retail/scripts/build-import-plans.py --check
python retail/scripts/test-import-plan-checks.py
python retail/scripts/audit-relationships.py --bundle logs/gin-migration/source-20261026.json
python retail/scripts/test-cell-record-postgres.py
```
