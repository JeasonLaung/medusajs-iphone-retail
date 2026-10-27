# 迁移实现状态

更新：2026-10-27。正式任务与验收要求见 [任务清单](../docs/gin-react-astro-tasks.md)。

## 已确认

目标为独立 Gin + MySQL + Redis、React / Ant Design 管理后台（图表统一 ECharts）、Astro + React 前台。前台保留原页面、URL、样式和交互。旧登录账号、认证材料和个人授权不迁移，历史交易主体保留业务证据，不能按邮箱自动赋权。当前不操作服务器。

## 当前交付与限制

| 范围 | 实际状态 | 尚缺 |
| --- | --- | --- |
| 源码及数据盘点 | 67 路由文件 / 86 方法、193 表 / 1,961 列元数据有清单 | 完整字段映射、原生及插件契约、剩余权限与副作用 |
| 设备 / 回收 / 邮件 | 已提取动作、状态与固定用例规格 | 新系统实现及行为验证 |
| 前台 | 32 种页面 URL + 7 个 handler 有源码清单；14 个公开/未登录状态，84 张本机截图 | 全部路由状态和交互；Astro 尚未实现 |
| Gin / SQL / 导入器 | 先前写入的未验收草稿；Go 包编译通过 | 隔离 MySQL 演练、事务及并发测试、DTO/授权审查、完整业务 |
| React Admin | 已确认 Ant Design 与 ECharts 选型 | 外壳、权限及业务页面均未实现 |
| 生产替换 | 未进行 | 全量对账、回归及未来切换计划 |

当前没有任何业务域可据此声明已完成迁移。草稿不能连接旧系统数据库作为新后端，也不能代理 Medusa 冒充迁移实现。

## 基线与验证

详见 [业务基线](../docs/migration/business-baseline.md)、[设备命令](../docs/migration/device-commands.md)、[回收与邮件](../docs/migration/buyback-and-mail.md)、[数据映射](../docs/migration/data-mapping.md) 和 [前台基线](../docs/migration/frontend-baseline.md)。原始业务快照及截图仅留在 Git 忽略目录，不提交身份凭据或业务行数据。
