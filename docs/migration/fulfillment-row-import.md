# 履约关系结构与历史导入

2026-10-28。本次落实 Gin 的历史履约证据结构和导入，不授予发货、取消、标签生成、provider 或库存命令权限。React 后台继续优先使用 Ant Design，图表统一 ECharts；本次尚未新增履约页面。

## 表与字段

`0021_fulfillment_relational.sql` 从已审查的来源字段计划及关系 DDL 生成 7 张表、72 个来源字段：fulfillment、fulfillment_address、fulfillment_item、fulfillment_label、shipping_option_rule、location_fulfillment_provider、location_fulfillment_set。逐列保留包裹状态时间、仓库/供应商/配送选项关联、原始 provider JSON、地址、明细、原始数量、标签与规则；仓库 provider/set 关联使用来源完整复合主键，原 id 字段同时保留。

数量使用 `DECIMAL(38,18)` 并保留 `raw_quantity`；SKU、条形码、物流单号按字符串保存。SQL NULL 与 JSON null 分别保留。物理外键、JSON 和布尔约束及八种规则运算符在 MySQL 中执行。预存同名表会阻止迁移，保留原数据。编译模型与运行标记进入启动和健康检查，`writes_enabled` 受 SQL 约束保持关闭。

模型摘要与生成文件摘要见 [schema_plan.json](../../retail/backend/internal/fulfillment/schema_plan.json)。生成脚本均支持 `--check`，检查审查来源与编译产物一致性。

## 事务导入

`0022_fulfillment_import_evidence.sql` 保存逐行来源/目标/快照摘要、完整身份、首次执行标识与逐字段缺失关联记录。`fulfillmentimport.Import` 必须接收全部七个表键（空表也必须存在），校验完整字段与完整复合身份，按物理外键顺序和身份稳定顺序写入；不接收任意目标表名。

调用方持有事务，任何错误或后续领域失败必须整体回滚，组件不会自行提交。同一模型行锁串行化历史批次，回执和目标校验使用当前读，旧 RR 快照也不能隐藏已提交回执。重放逐列核对来源、目标、SQL NULL 和精确 DECIMAL；不以已有 id 直接忽略内容差异。任何错误返回零结果，不报告局部成功。

语义父域尚不可用或父记录缺失时，保存原引用并登记 `parent_domain_unavailable` / `missing_parent`。记录禁止自动解除，不补造订单、仓库或库存，不把导入成功视为发货授权。先前领域导入时观察到的差异不会因本次导入自动删除或重写；正式全域协调器和人工差异处理仍待实现。

## 已验证范围

[人工 MySQL / Redis 报告](admin-auth-report.json) 的 `fulfillment_history` 验证完整 22 文件迁移、7 表/72 字段、9 行及 11 条缺失引用。覆盖事务回滚、最后阶段触发器失败回滚、8 并发一次新写入/七次重放、旧 RR 快照重放、完整复合身份、超出 JS 安全整数的精确数量、前导零 SKU/物流单号、物理外键/JSON/布尔/枚举拒绝、目标小数漂移拒绝、禁用模型和保留已有表。未创建账号，未改动库存，未调用 provider。

[固定快照整合演练](catalog-mysql-rehearsal-report.json) 在独占且有资源上限的 MySQL 中组合商品、库存、历史客户、支付、订单与履约；来源连接、生产目标和 provider 均未使用。实际完整来源验证 159 表/27,140 行，本领域导入并精确重放 167 行：4 包裹、89 地址、4 明细、0 标签、4 规则、64 仓库/provider 关联及 2 仓库/set 关联。1 条来源缺失明细关联保留登记；此前订单导入观察到的 271 条差异继续保留，完整来源 375 项缺失不变。回滚验证同样覆盖 167 行，读取与重放前后完整业务指纹一致。标签在本快照为空，其精度验证来自人工样本，不能视为真实标签数据验收。

[真实后台浏览器回归](admin-live-report.json) 使用实际 Gin + MySQL + Redis + Vite，验证既有 Ant 商品、库存、订单及支付界面，新增的履约迁移随启动应用；不是新增履约 UI 或发货流程验收。

## 仍需完成

[履约私有查询与 Ant 明细已新增](admin-order-fulfillments.md)；完整用户/仓库权限、IMEI 分配、部分履约、原子库存联动、发货/取消幂等命令、物流 provider 实际对账与回调、正式全域导入协调器和 Astro 仍在后续范围内。订单、履约及整体迁移均未完成验收。
