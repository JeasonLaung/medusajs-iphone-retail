# 目录配置整行事务导入组件

2026-10-27。新增 `internal/catalogimport.Import` 作为后续完整 v2 目录导入器的 SQL 写入层。覆盖既有 catalog-field-mapping 清单的36种目录、价格、区域、渠道和商店配置表，固定 SQL 布局由严格字段计划及完整唯一键计划生成。它不是现有 `cmd/import-medusa` 的替换版本；旧命令仍是未验收的v1目录草稿，不能用于正式迁移。

调用者提供已验证完整流的快照SHA256、完整源行和隔离目标事务。组件先调用严格整行 Convert，拒绝缺列/多列、非法值、raw金额配对/CHECK不一致，随后核对生成布局中的目标表和每一列。未知表和保护/账号/支付领域不进入该入口。

主键依据固定源计划，支持单ID、币种/国家代码及复合主键。主键字符串不拼接或截断：以版本字节、每个组件的UTF-8长度及完整值编码，摘要仅用于账本索引，账本同时保存完整身份并逐字节比较，摘要相撞不会当成同一主体。category_id、tag_id等目标改名使用固定映射；复合主键读取不错误假设存在id。

`0008_migration_catalog_rows.sql` 记录源表、完整身份/身份摘要、完整快照摘要、源行/目标行摘要、首次尝试随机值及policy版本。源行摘要排序字段，含长度前缀、精确JSON字节与SQL NULL标志；JSON词法变更属于来源变更。账本先写入再加行锁核对，已有来源、快照、身份或目标摘要不同直接冲突。

七类长文本源唯一键使用完整登记：pricing_preferences、pricing_price_rules、catalog_options、catalog_option_values、catalog_tags、catalog_product_types、fulfillment_shipping_profiles。目标字段投影保留SQL NULL和软删除/参与条件，登记与账本、业务行使用同一事务；其他29表不误调用不存在的登记scope，原生目标索引继续由实际schema执行。

只在首次尝试插入完整业务行，精确重放逐列核对、不更新或补写丢失目标。金额/数量等DECIMAL字段在比较参数上显式 `CAST(? AS DECIMAL(38,18))`，避免字符串参数比较被隐式转为DOUBLE而掩盖末位差异。JSON用原文LONGTEXT，不改成原生JSON或重编码，SQL NULL使用NULL安全比较；不按当前价格重算历史字段。

所有错误要求调用者回滚整个事务，包括已经写入的账本/唯一声明。没有自动commit、源库连接、HTTP/provider、文件复制、授权或应用启动DDL。成功只是完整行暂存，完整流/footer/hash、引用/历史缺口、文件和领域审查仍是提交前提，不因某行成功就默认整个快照可接受。

布局输入摘要和逐列类型/主键对齐由普通Go测试核对；生成命令可重现：

```powershell
python retail/scripts/generate-catalog-import-plan.py --check
```

人工MySQL 8.4.11验证使用既有单核768MiB禁网临时容器，复制实际审查版tag、分类关联和price表结构（LIKE不复制外键，因此不冒充跨表验证）。覆盖长值整行/账本/唯一声明回滚、重复JSON键/大整数/词法/SQL NULL保留、精确重放、复合主键及字段改名、DECIMAL第18位变化被拒绝、来源/快照/目标冲突、完整唯一键冲突、缺目标不补写、八路同源重放、业务SQL失败回滚及未知领域拒绝。结果见 [mysql-type-fixture-report.json](mysql-type-fixture-report.json) 的TestCatalogImportMySQL。其余33表的真实SQL与完整关联尚未验证，不能把36项布局覆盖说成36表迁移验收。

本次新增0008使显式执行器的空库/重复测试覆盖八个迁移文件；0001仍不符合完整159表目标设计。尚缺审查版schema提升、完整目录源数据与父依赖导入、普通JSON内资料/业务语义审查、真实目录SQL演练、完整导入运行账本/恢复、旧v1命令替换及Gin/React/Astro商品闭环。375项全局引用缺口仍未验收。T018/T020/T021不因此完成。
