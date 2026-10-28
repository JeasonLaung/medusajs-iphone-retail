# 税区正式结构与事务内读取

2026-10-28。T020/T032/T033 的中间实现；普通配置导入、后台税区管理和购物车重算事务尚未完成。

`0028_tax_relational.sql` 从已经固定的完整 MySQL DDL 审查中生成，承接 tax_provider、tax_region、tax_rate、tax_rate_rule 共 4 表/36 个来源字段。保留 ID、可空税率、删除状态、JSON、创建者历史引用、父区/提供者引用和原布尔约束。rate 使用已审查 DECIMAL(38,18)，读取保留精确字符串，不恢复旧登录身份。

`generate-tax-schema.py --check` 核对原审查 DDL SHA、字段计划、完整选表和依赖，复现 SQL 与 `tax/schema_plan.json`。同名表存在时，迁移 guard 在创建业务表之前拒绝，不删表、不覆盖数据。迁移器启动前检查完整 28 文件；应用启动与 `/health` 同时核对编译税区模型。`writes_enabled` 受到 FALSE 约束，表结构就绪不等于税务写命令或 checkout 已开放。

保留四项审查差异：MySQL CHECK 与级联外键组合限制，导致 tax_region.parent_id 的 CASCADE 及 provider_id 的 SET NULL 外键不能直接照搬；国家/省为完整 LONGTEXT，两个活动唯一索引也不能用前缀索引代替。计划记录原约束、原因与 hold。父区和 provider 的普通引用保留，配置写权限仍关闭；后续专用导入和写事务需要完整值唯一账本、明确锁定和父关系维护，不能把当前结构宣称为完整约束等价。

`tax.ReadSystem` 只在调用者的 Gorm SQL 事务内运行，自动提交入口拒绝。调用方负责 RR 快照/写事务锁、购物车授权、版本与输入来源；本函数无事务提交、业务写入或 HTTP 路由。根据 Automatic/Force 与配送国家决定是否计算，强制无国家拒绝，普通无国家和没有国家父税区返回空税行。国家/省按原逻辑规范为小写；国家限制两位代码，所有集合和文本有尺寸界限。

读取有效国家/省区、启用的内置 provider（数据库 ID `tp_system`，输出税行标识 `system`）、未删除税率和规则，再调用独立 Go 选择与投影引擎。保留礼品卡排除、商品/运费区别、父税区组合、null rate 为零和税率精度。其他 provider、缺失/停用 provider、重复国家/省区、错误父关系和同优先级候选冲突拒绝，不以来源未定义的查询顺序决定税率。已有纯 `Select` 仍保留给定输入顺序的原算法；SQL 适配器对配置歧义额外拒绝，这项限制需在后台配置审查中明确处理。

人工 MySQL 检查在 `identity/tax_schema_test.go`，纳入独占 `test-admin-auth.py`：迁移与重放、原表保护、SQL CHECK/FK/活动默认唯一性、实际商品/运费税行、18位小数、国家/省规范化、礼品卡、自动/强制/缺地址、缺父区、null 税率、停用/未知 provider、重复税区、同级冲突、删除税率、禁止自动提交和模型漂移。来源业务库、旧账号和外部 provider 不参与。实际运行结果以当前 `admin-auth-report.json.tax_schema_and_read` 及其源码 SHA 为准。

```text
python retail/scripts/generate-tax-schema.py --check
python retail/scripts/test-tax-selection.py
python retail/scripts/test-admin-auth.py
```

税区完整导入尚未实现，也未把固定快照 1 个 provider/7 个 tax_region 写进运行数据。尚需专用导入回执/差异与唯一性处理、真实快照比对、provider 适配、Ant 管理及权限、税行保存、价格/税/促销/配送同事务换区和完整前台验收。旧订单税行继续作为成交快照读取，不能用当前税区覆盖历史成交金额。

本批完整独占SQL回归已通过：28迁移执行/重放、税区专项及既有16组领域测试，admin-auth-report记录170项源码SHA，逐项核对无漂移；测试自有容器和二进制已清理。Go全包测试与tax/schema/server静态检查通过。真实进程及浏览器的联合报告另行验证，不以本SQL报告替代。

最终补充：实际安装的providers loader按tp_<identifier>注册，已固定loader源码SHA并修正数据库ID为tp_system，税行输出仍为system。修正后完整SQL回归重跑通过，另验证税区模型停用/恢复使健康检查503/200。真实Gin/MySQL/Redis下后台3项浏览器、商品加购/购物车恢复和Astro全部组件对照通过，自有容器/网络/程序清理完成；admin-live、astro-runtime及tax-selection报告源码SHA无漂移。服务检查9001 health/app、8000商品/购物车和9300预览均200，既有错误日志长度/时间未新增。配置导入和完整换区事务继续待办。
