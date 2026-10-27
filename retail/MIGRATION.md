# 迁移实现状态

更新：2026-10-27。正式任务与验收要求见 [任务清单](../docs/gin-react-astro-tasks.md)。

## 已确认

目标为独立 Gin + MySQL + Redis、React / Ant Design 管理后台（图表统一 ECharts）、Astro + React 前台。前台保留原页面、URL、样式和交互。旧登录账号、认证材料和个人授权不迁移，历史交易主体保留业务证据，不能按邮箱自动赋权。当前不操作服务器。

## 当前交付与限制

| 范围 | 实际状态 | 尚缺 |
| --- | --- | --- |
| 源码及数据盘点 | 67 路由文件 / 86 方法；193表1,961列均有处置设计：159表1,691列映射、34表270列排除/重建；快照预检/语义差异有报告 | 全部约束/默认值落地、1条预留合计差异、119条设备/预留软删引用及交易162行256孤儿引用对账、MySQL验证、原生及插件契约、剩余权限与副作用 |
| 设备 / 回收 / 邮件 | 已提取动作、状态与固定用例规格 | 新系统实现及行为验证 |
| 前台 | 32 种页面 URL + 7 个 handler 有源码清单；14 个公开/未登录状态，84 张本机截图 | 全部路由状态和交互；Astro 尚未实现 |
| Gin / SQL / 导入器 | 先前写入的未验收草稿；Go 包编译通过 | 隔离 MySQL 演练、事务及并发测试、DTO/授权审查、完整业务 |
| 长文本完整唯一登记 | 新Go组件及独立SQL在人工MySQL验证碰撞、回滚、8路并发及源商品选项同事务写入；28源键固定适配计划（27普通、1受保护拒绝）；见[执行证据](../docs/migration/mysql-unique-key-registry.md) | 受保护token比较及全部领域写入/导入接入仍缺，邮件联系人整组拒绝；源索引继续hold |
| 购物车与合计 | 18 组人工金额、10 组库存准备及 46 组促销/税率/配送规则样例可重现，原生成单顺序有记录 | 完整促销/税/运费引擎、Gin 实现及并发恢复验证 |
| React Admin | Ant Design / ECharts 组件及 23 个页面文件、6 个 widgets 承接有设计 | 外壳、权限及业务页面均未实现 |
| 生产替换 | 未进行 | 全量对账、回归及未来切换计划 |

当前没有任何业务域可据此声明已完成迁移。草稿不能连接旧系统数据库作为新后端，也不能代理 Medusa 冒充迁移实现。

## 基线与验证

详见 [业务基线](../docs/migration/business-baseline.md)、[设备命令](../docs/migration/device-commands.md)、[回收与邮件](../docs/migration/buyback-and-mail.md)、[数据映射](../docs/migration/data-mapping.md) 和 [前台基线](../docs/migration/frontend-baseline.md)。原始业务快照及截图仅留在 Git 忽略目录，不提交身份凭据或业务行数据。

新增契约见 [购物车与合计](../docs/migration/checkout-contract.md)、[API 约定](../docs/migration/api-conventions.md) 和 [后台组件](../docs/migration/admin-ui-contract.md)。样例仅用人工输入，不代表真实支付或新应用验收。
