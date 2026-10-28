# 税务配置事务导入

2026-10-28，T020/T032/T033 的中间交付，完整迁移未验收。

`taximport.Import` 使用调用者拥有的 SQL 事务导入 tax_provider、tax_region、tax_rate、tax_rate_rule 四表的36个来源字段。0029迁移保存来源、目标、快照摘要、完整身份和不可释放的缺失引用记录。调用者先验证完整快照，并在任何导入或后续领域失败时回滚整个事务；组件不提交事务，不访问来源数据库，不执行 provider。

导入锁定税务模型记录并核对读取已启用、运行时写入仍关闭。税区国家和省使用完整值唯一性账本，不能用文本前缀唯一索引代替。父区先留空，完成插入后恢复并逐列比对，子区 ID 排在父区前也能导入。SQL null、JSON null、删除状态和 DECIMAL(38,18) 分别保留。重放要求相同来源、快照、完整身份和精确目标；漂移拒绝，不覆盖现有记录。

人工 MySQL 专项通过：整批回滚、末尾引用记录失败回滚、重复国家和省拒绝、8路并发仅一批新写入、旧 RR 快照重放、父关系恢复、18位税率、来源与目标漂移拒绝、停用模型拒绝和不可释放 hold。证据见 admin-auth-report.json 的 tax_import 与源码摘要。

固定快照演练通过：159表/27140行经过流式验证，税务部分为1个 provider、7个税区、0条税率、0条规则；先回滚确认无残留，再提交8行并精确重放，税务引用 hold 为0。税表、回执及 hold 纳入联合只读指纹。证据见 catalog-mysql-rehearsal-report.json。实际快照没有税率，精确小数及规则的覆盖来自人工SQL用例，不能用实际快照宣称覆盖。

复现：

```text
python retail/scripts/generate-tax-import-plan.py --check
python retail/scripts/test-admin-auth.py
python retail/scripts/rehearse-catalog.py --snapshot <受保护的完整固定快照>
```

这不等于开放税务后台写入、第三方 provider 或结账。父区/提供者物理外键的原审查差异仍保留，导入证据不代表运行时删除/更新语义完成。Ant管理及权限、购物车税行保存、价格/优惠/配送同事务换区和正式页面继续待办。旧成交税行仍作为历史证据，不能被当前税率覆盖。没有迁移旧登录账号。
