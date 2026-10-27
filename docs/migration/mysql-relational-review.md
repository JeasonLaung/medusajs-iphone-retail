# 全字段关系结构审查稿

日期：2026-10-27。`mysql-relational-review.sql`覆盖六批字段设计的159张目标表、1,691个源字段。它位于文档目录，**不是backend/migrations中的正式迁移**，不能部署或导入生产数据。所有未解决的约束都有逐项记录，建表成功不表示迁移完成。

```powershell
python retail/scripts/build-mysql-ddl.py --check
python retail/scripts/test-mysql-types.py
```

生成器不读.env或业务行、不连数据库，严格使用六批显式字段映射、源constraint/index元数据及34表排除清单；每个字段/表集合必须完整互斥覆盖。未知类型、check表达式、外键动作或标识符失败，不以宽泛fallback吞掉。生成SQL与输入都有规范换行SHA256，`--check`检测漂移。

## 实际产物

| 范围 | 实际结果 |
| --- | --- |
| 表/源字段 | 159 / 1,691，独立目标表，源ID及列名重命名按映射 |
| 主键 | 159项；复合业务主键保留，不假定所有表以id单列唯一 |
| 源CHECK | 47项，明确枚举/非负/税区跨字段/布尔单例；另加boolean 0/1及精确JSON文本JSON_VALID检查 |
| 外键 | 86项在全部建表后ALTER，按映射转换父子字段及明确动作 |
| 源约束暂缓 | 4项：2项PayPal完整值唯一，2项税区CHECK/FK动作冲突 |
| 被排除源表的约束 | 53项逐条记录排除，不恢复旧登录或迁移框架账本 |
| 源索引 | 759项全登记，159项主键已覆盖；460项待转换，其余140项属于不导入表 |

SQL不带IF NOT EXISTS，避免已有不兼容表被静默接受；不带源触发器、nextval、默认值或ON UPDATE时间副作用。所有业务导入值必须显式提供，可NULL与必填保持源约定。新命令默认值/版本/幂等/outbox/新身份是另一套目标设计，不能通过这份源字段DDL自动生成。

文本统一utf8mb4_0900_bin，身份按显式VARCHAR长度；精确数值DECIMAL(38,18)，UTC instant承接DATETIME(6)；原文JSON LONGTEXT+CHECK，受保护信封按字段映射使用BLOB或文本。由于严格模式仍可能舍入，导入器仍须在SQL前校验精度/范围，见[类型执行证据](mysql-type-contract.md)。

## 实际MySQL执行

`test-mysql-types.py`在已有本机MySQL8.4.11镜像的禁网临时容器中创建独立relational_fixture库，核对审查稿hash后执行全部SQL，查information_schema确认159张表、1,691列、全部InnoDB。原45项人工类型/约束检查一并执行，总48项通过。仅人工测试数据，未导入固定历史快照；1核/768MiB、无端口、临时内存盘，结束核对所有权并清理。

初次执行真实报错3823：tax_region.provider_id在跨字段CHECK中，同时要求FK ON DELETE SET NULL；parent_id同理与ON DELETE CASCADE冲突。生成器现在明确hold这两条FK，保留原CHECK，不假装转换已等价完成。后续需要显式完整父节点/供应者命令和一致性方案，或审查可等价的约束重构；在解决前这份SQL不具备完整外键保护。

## 必须继续完成

460项源索引包括部分唯一、条件唯一、表达式及普通查询索引，均不可静默忽略。超宽键不能通过前缀唯一、截断或仅哈希相等冒充完整身份；PayPal事件唯一还关联持久回调去重，必须落实。全部约束/索引在JSON中可追踪，不因“空库能建”标完成。

真实快照已有孤儿引用、软删除关系和库存预留差异仍有效；不能通过FOREIGN_KEY_CHECKS=0导入后称一致，也不能自动补造或删除业务行。正式执行器的校验和、迁移互斥、失败恢复、运行账本、受保护配置重加密、新账号与授权、完整交易服务及React/Astro均未由本稿实现。T006/T011/T012/T018不据此验收。
