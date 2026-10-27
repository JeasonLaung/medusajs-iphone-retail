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
| 目标整行转换 | 新Go转换器执行159表/1,691列、59组raw配对及47条源CHECK，普通字段严格转换、受保护值hold；人工MySQL标签整行与唯一登记同事务验证；真实只读v2完整流27,140行已验证，支付42整行转换通过 | 全域v2值转换、源约束/索引漂移指纹、其他受保护域、完整导入执行器/恢复/差异对账；未替换旧目录草稿 |
| 跨表快照校验 | 独立Go封装校验159表主键与224关联/88源外键，真实v2完整流hash后审计43,496项，保留375项已知缺失，物理外键缺失0 | 关系明确未验收；历史缺口结构、正式导入组合、超大数据磁盘staging及全域v2导入演练 |
| 旧注册邮件历史 | 专用Go入口清理已知认证载荷；真实2行/8字段离线通过；新增SQL处置账本及同事务写入/精确重放检查，人工MySQL回滚、冲突、8路并发通过；普通业务邮件hold | 真实行持久化、完整导入组合、worker执行、其他来源认证能力审查尚缺 |
| 长文本完整唯一登记 | 新Go组件及独立SQL在人工MySQL验证碰撞、回滚、8路并发及源商品选项同事务写入；28源键固定适配计划（27普通、1受保护专用比较）；见[执行证据](../docs/migration/mysql-unique-key-registry.md) | 受保护HMAC重建/轮换及全部领域写入/导入接入仍缺，普通联系人入口拒绝；源索引继续hold |
| 业务秘密信封 | Go AES-256-GCM版本/key ID/精确上下文绑定组件，人工SQL持久化及轮换回滚通过；12商户字符串转换入口与四域真实旧函数的16个人工样本互操作、人工MySQL整行密封/回滚通过；3条真实配置/6字段在私有stdin路径由Go转换并与Node预期相等，仅临时信封 | 受保护唯一比较已人工验证；真实新密文持久化、保护JSON审查、密钥保管/恢复、读取授权尚缺 |
| 购物车与合计 | 18 组人工金额、10 组库存准备及 46 组促销/税率/配送规则样例可重现，原生成单顺序有记录 | 完整促销/税/运费引擎、Gin 实现及并发恢复验证 |
| 支付context历史资料 | 显式投影移除旧客户地址簿/账号绑定及默认标记，保留经核对的交易历史；21个真实context临时信封往返、人工LONGTEXT存储通过；完整行组合见下行 | 真实持久化、provider对账和新账号归属/在途恢复未实现 |
| 支付provider历史data | [专用Go组件](../docs/migration/payment-data-review.md)逐层审查手动/Mollie载荷，42个真实对象临时密封往返、16个Mollie精确金额/引用匹配、42次错行拒绝；完整行组合见下行 | 正式SQL导入、远端对账及支付运行时尚缺；不启用生产手动付款 |
| 支付历史完整行 | [专用入口](../docs/migration/payment-history-conversion.md)组合data/context/metadata与强制操作hold；[同事务SQL组件](../docs/migration/payment-history-import.md)在人工MySQL验证原密文重放、轮换读取、冲突、回滚和8路并发；[真实隔离演练](../docs/migration/payment-mysql-rehearsal.md)完成23集合/42支付/42hold的回滚、提交、重放及加密读取，临时库已清理 | worker执行、正式全域导入/恢复、持久密钥管理、远端对账及读取授权仍缺 |
| PayPal商户配置 | [专用转换](../docs/migration/paypal-config-conversion.md)补明文/v1/v2及live/sandbox嵌套秘密解密投影、别名/活动镜像冲突拒绝；原TS/安装JS/Go互操作9样本/27字段含18嵌套秘密，人工MySQL整行回滚/读取通过；真实v2两配置内存转换通过 | 其他设置section/JSON、配置副作用hold、真实SQL、key恢复和Gin provider尚缺；快照无真实非空凭据 |
| React Admin | Ant Design / ECharts 组件及 23 个页面文件、6 个 widgets 承接有设计 | 外壳、权限及业务页面均未实现 |
| 生产替换 | 未进行 | 全量对账、回归及未来切换计划 |

当前没有任何业务域可据此声明已完成迁移。草稿不能连接旧系统数据库作为新后端，也不能代理 Medusa 冒充迁移实现。

## 基线与验证

详见 [业务基线](../docs/migration/business-baseline.md)、[设备命令](../docs/migration/device-commands.md)、[回收与邮件](../docs/migration/buyback-and-mail.md)、[数据映射](../docs/migration/data-mapping.md) 和 [前台基线](../docs/migration/frontend-baseline.md)。原始业务快照及截图仅留在 Git 忽略目录，不提交身份凭据或业务行数据。

新增契约见 [购物车与合计](../docs/migration/checkout-contract.md)、[API 约定](../docs/migration/api-conventions.md) 和 [后台组件](../docs/migration/admin-ui-contract.md)。样例仅用人工输入，不代表真实支付或新应用验收。
