# MySQL类型与身份约束隔离验证

日期：2026-10-27。T006/T012设计落地证据，尚不是目标全量DDL或导入器验收。

## 可重复执行

```powershell
python retail/scripts/test-mysql-types.py
```

使用已存在本机mysql:8.4镜像，实际执行版本8.4.11；49项类型/条件唯一检查加5项全表结构检查，共54项结果见 `mysql-type-fixture-report.json`。临时容器network=none、不发布端口、限制1核/768MiB内存/512MiB数据tmpfs，所有表和数据均为人工；随机root密码仅用于本次初始化，未打印或写入报告。finally核对随机所有权标签后清理容器及卷；已清理。没有连接现有MySQL/PostgreSQL/Redis、源业务快照或部署服务器。

每次SQL连接设UTC与STRICT_ALL_TABLES、NO_ZERO_DATE、NO_ZERO_IN_DATE等明确模式。除人工最小表及真实SQL约束外，另在隔离空库执行[159表审查DDL](mysql-relational-review.md)，核对表数、列数和InnoDB。审查稿有显式约束/索引hold，不是完整目标迁移；不运行新Gin、不导入真实历史数据。

## 类型与约束结果及迁移影响

| 范围 | 实际执行结果 | 目标要求 |
| --- | --- | --- |
| 精确数值 | DECIMAL(38,18)保留38位输入及极小负值；整数部分超20位报1264 | 词法/整数位/小数位/范围进入SQL前验证；业务金额与cost_cents单位分开 |
| 超额小数精度 | 严格模式下0.1234567890123456789写成0.123456789012345679 | 不能把写入成功当无损；超精度拒绝，不能偷偷舍入历史金额/数量 |
| JSON文本 | LONGTEXT+JSON_VALID保留大整数、1.2300词法、重复键及原UTF-8字节；无效JSON报3819 | 原文存储与公开DTO分开；Go使用RawMessage/UseNumber，不先经float64解析 |
| 原生JSON对照 | 同一人工文本重序列化；超64位整数为DOUBLE，1.2300为1.23，重复键只剩最后值 | MySQL原生JSON不能替代精确原文存档；内部检索字段单独定义 |
| NULL | JSON文本null与SQL NULL可区分；boolean false保留 | 不能以空值/truthy判断替换false/0/null/未提供 |
| ID | utf8mb4_0900_bin为NO PAD；ID/id/id尾空格可并存，Unicode/emoji字节不丢；超191字符报1406 | 历史ID保留原文；业务新ID输入可更严格，但不能trim/lower旧ID；外键列同类型/排序规则 |
| 时间 | DATETIME(6)保留微秒；零日期、无效日历日期、必填NULL拒绝 | 源instant先转UTC；缺值不补epoch/now；每个连接设UTC |
| 额外时间精度 | .1234567写入.123457，严格模式仍舍入 | 导入前明确拒绝或按源已知精度规范化；不能靠DATETIME自动处理 |
| 会话时区 | 把session改+08后DATETIME读取仍是原墙上时间 | DATETIME本身不执行UTC instant转换；应用必须先统一UTC |
| 活动唯一键 | 生成列IF(deleted_at IS NULL,value,NULL)阻止活动重复，软删除保留文本并允许新活动行，冲突恢复报1062 | 显式生成键/注册表设计；恢复也须冲突检查，不能删除历史值 |
| 错误复合键方案 | UNIQUE(value,deleted_at)允许两个deleted_at=NULL的同值活动行 | 不得用该方案冒充PostgreSQL部分唯一索引 |
| 长键上限 | 512个四字节字符的完整单值唯一索引可用；两个512字符utf8mb4字段复合唯一键报1071 | 不能用前缀索引/直接截短替代源完整唯一性；超宽身份需事务化完整值注册与碰撞复核 |
| option条件唯一 | nonexclusive未删title全局唯一，两个exclusive同名可并存；exclusive改成shared冲突被拒绝 | 保留is_exclusive与软删除条件，不能把所有标题强制唯一 |
| option value | 191字符父ID+512字符value的完整复合唯一索引可建；NULL父多同值保留，非NULL活动重复拒绝 | 不用空字符串替代可NULL父关系；恢复冲突检查 |
| 外键 | 孤儿父ID、大小写不同父ID插入报1452；有子项父硬删除报1451 | 保留关联语义，已发现历史孤儿须对账而不能为了导入关闭外键后当作通过 |
| 单例计数 | BOOLEAN主键+CHECK(id=1)拒绝false及重复true，last_id>=0有效；更新随事务回滚 | 业务计数与序列分别设计，不能依靠普通boolean列自然实现单例 |

精确文本保留重复JSON键是历史证据保留，并不要求公开API接受歧义输入。新命令仍需明确输入schema、重复键策略和签名规范化；公开响应按字段白名单，不直接把归档payload透传。

## 尚缺的真实交付

完整源约束/索引到159表目标DDL转换、最长复合身份的注册方案、精确导入器、隔离真实快照导入及全部差异对账；编号分配并发/回滚策略、新写入和失败恢复；Gin授权与前后端业务。45项不能证明所有金额/JSON/字段已可导入，也不证明已有Gin草稿DDL满足要求。现有交易孤儿引用、库存预留差异、软删除关系等既有报告继续有效。T006/T012保持进行中。
