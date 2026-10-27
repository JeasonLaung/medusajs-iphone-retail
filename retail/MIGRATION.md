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
| 新后台登录 | [Gin/MySQL/Redis联合路径](../docs/migration/admin-auth-runtime.md)：一次性挑战、精确Origin、CSRF、双维限流、会话轮换/退出；新账号非零撤销代数逐请求复核；独占临时库验证停用/角色/密码恢复后旧会话不复活、旧/客户cookie拒绝、依赖关闭503及九文件schema执行/重复 | 客户认证、Ant页面、完整权限/范围、账号编辑及撤销事务、真实浏览器/TLS/服务崩溃仍缺；不能用于生产 |
| Redis会话组件 | [独立组件](../docs/migration/redis-session-store.md)：后台/客户及应用namespace隔离、12小时绝对会话、10分钟一次性CSRF登录挑战、15分钟8次原子限流；独占禁网临时Redis验证16路消费仅1次、24路限流仅8次及过期/撤销/坏记录/连接关闭拒绝 | 随后已接入[后台联合路径](../docs/migration/admin-auth-runtime.md)；客户认证、完整权限、浏览器、真实宕机/TLS/进程生命周期仍缺 |
| Gin HTTP边界 | [请求追踪与私有诊断](../docs/migration/gin-http-boundary.md)：替换默认URL/panic/SQL日志，服务端请求ID、统一错误、404/405、1MiB与分块413、后台禁止缓存；真实Gin路由人工请求验证通过 | 真实数据库HTTP、完整错误表、CSRF/Redis会话、权限、依赖配置和领域DTO仍缺，现有payload目录草稿尚不兼容159表 |
| 显式schema执行器 | [独立组件](../docs/migration/schema-migration-runner.md)替换按分号切分草稿，逐语句inflight/完成记录、全计划校验和/顺序核对、专用连接互斥及失败拒绝盲重试；九文件空库/重复联合演练通过，前述八文件人工MySQL执行/重复/失败对账恢复/锁通过；实际DDL等待中终止子进程后重试拒绝不确定记录，后续语句不执行 | 0001仍为未验收草稿，159表正式迁移、断电/MySQL崩溃/真实恢复、结构差异比对和对账工具仍缺 |
| 目标整行转换 | 新Go转换器执行159表/1,691列、59组raw配对及47条源CHECK；[全量v2值检查](../docs/migration/value-coverage.md)验证27,140行/220,235字段，普通完整转换27,090行，50行需专用规则，普通值/raw配对/CHECK失败0；支付及商户专用转换另有真实SQL证据 | 源约束/索引漂移指纹、其他保护域、完整导入执行器/恢复/差异对账；61空表无真实样本；旧v1写入入口已退役，新全域协调器仍缺 |
| 目录配置事务写入 | [新组件](../docs/migration/catalog-row-import.md)固定36表布局、复合主键/字段改名、完整来源/目标账本及7类长文本唯一登记；人工MySQL验证tag/分类关联/price回滚、重放、金额末位差异和八路并发；[真实目录演练](../docs/migration/catalog-mysql-rehearsal.md)在开启外键的完整DDL下验证36表+9父依赖共23,559行的回滚/提交/精确重放、21,837账本及348完整唯一声明 | 父领域正式账本、空表真实样本、JSON业务语义、v2写入执行器及正式schema仍缺；旧命令现已退役，不等于目录领域完成 |
| v2离线命令 | [预检与退役说明](../docs/migration/v2-preflight.md)：新命令组合完整流、全部字段和引用诊断，真实159表27,140行通过结构/普通值检查，375项引用缺口返回2；错误输入不输出部分报告；旧v1命令及库函数均先拒绝 | 正式全域写入/恢复协调器、正式schema及领域验收仍缺；诊断不启用任何写入 |
| 跨表快照校验 | 独立Go封装校验159表主键与224关联/88源外键，真实v2完整流hash后审计43,496项，保留375项已知缺失，物理外键缺失0 | 关系明确未验收；历史缺口结构、正式导入组合、超大数据磁盘staging及全域v2导入演练 |
| 旧注册邮件历史 | 专用Go入口清理已知认证载荷；真实2行/8字段离线通过；新增SQL处置账本及同事务写入/精确重放检查，人工MySQL回滚、冲突、8路并发通过；普通业务邮件hold | 真实行持久化、完整导入组合、worker执行、其他来源认证能力审查尚缺 |
| 长文本完整唯一登记 | 新Go组件及独立SQL在人工MySQL验证碰撞、回滚、8路并发及源商品选项同事务写入；28源键固定适配计划（27普通、1受保护专用比较）；见[执行证据](../docs/migration/mysql-unique-key-registry.md) | 受保护HMAC重建/轮换及全部领域写入/导入接入仍缺，普通联系人入口拒绝；源索引继续hold |
| 业务秘密信封 | Go AES-256-GCM版本/key ID/精确上下文绑定组件，人工SQL持久化及轮换回滚通过；12商户字符串转换入口与四域真实旧函数的16个人工样本互操作、人工MySQL整行密封/回滚通过；3条真实配置/6字段在私有stdin路径由Go转换并与Node预期相等，仅临时信封 | 受保护唯一比较已人工验证；真实新密文持久化、保护JSON审查、密钥保管/恢复、读取授权尚缺 |
| 购物车与合计 | 18 组人工金额、10 组库存准备及 46 组促销/税率/配送规则样例可重现，原生成单顺序有记录 | 完整促销/税/运费引擎、Gin 实现及并发恢复验证 |
| 历史付款主体 | [专用转换](../docs/migration/account-holder-conversion.md)保留system provider空资料及历史外部引用；[原子SQL组件](../docs/migration/account-holder-import.md)人工并发/冲突通过；[真实SQL演练](../docs/migration/account-holder-mysql-rehearsal.md)验证1主体/1实际provider/1 hold/1唯一声明回滚、提交、重放及读取，临时库已清理，不恢复旧客户绑定 | 正式全域组合/恢复、持久密钥、其他provider载荷、读取授权及worker执行仍缺 |
| 支付context历史资料 | 显式投影移除旧客户地址簿/账号绑定及默认标记，保留经核对的交易历史；21个真实context临时信封往返、人工LONGTEXT存储通过；完整行组合见下行 | 真实持久化、provider对账和新账号归属/在途恢复未实现 |
| 支付provider历史data | [专用Go组件](../docs/migration/payment-data-review.md)逐层审查手动/Mollie载荷，42个真实对象临时密封往返、16个Mollie精确金额/引用匹配、42次错行拒绝；完整行组合见下行 | 正式SQL导入、远端对账及支付运行时尚缺；不启用生产手动付款 |
| 支付历史完整行 | [专用入口](../docs/migration/payment-history-conversion.md)组合data/context/metadata与强制操作hold；[同事务SQL组件](../docs/migration/payment-history-import.md)在人工MySQL验证原密文重放、轮换读取、冲突、回滚和8路并发；[真实隔离演练](../docs/migration/payment-mysql-rehearsal.md)完成23集合/42支付/42hold的回滚、提交、重放及加密读取，临时库已清理 | worker执行、正式全域导入/恢复、持久密钥管理、远端对账及读取授权仍缺 |
| PayPal商户配置 | [专用转换](../docs/migration/paypal-config-conversion.md)补明文/v1/v2及live/sandbox嵌套秘密解密投影、别名/活动镜像冲突拒绝；原TS/安装JS/Go互操作9样本/27字段含18嵌套秘密，人工MySQL整行回滚/读取通过；真实v2两配置内存及临时SQL验证通过 | 其他设置section/JSON、key恢复和Gin provider尚缺；快照无真实非空凭据 |
| 商户配置原子导入 | [专用组件](../docs/migration/merchant-config-import.md)把七表配置、固定操作hold账本和邮箱/SEO key完整唯一登记纳入同一调用者事务；人工MySQL验证回滚、重放、轮换读取、冲突及八路并发；[真实隔离演练](../docs/migration/merchant-mysql-rehearsal.md)验证5配置/5 hold/1唯一登记及8字段独立预期读回、3 SQL NULL | 完整导入组合/恢复、持久密钥与读取授权、worker强制执行hold和Gin配置接口尚缺；临时库已清理，两个SEO表无真实样本 |
| React Admin | [Ant登录/账号页面](../docs/migration/admin-login-ui.md)新增独立React应用，Form/Layout/Menu/Drawer/Descriptions，直连Gin协议；类型及build通过，7项人工HTTP浏览器测试及9张中英三宽截图；23页面/6widgets仍仅承接设计，图表固定ECharts | 真实浏览器到Gin/MySQL/Redis串联、完整外壳/页签及权限、全部业务页和图表仍缺；bundle体积待拆分 |
| 商品基础信息编辑 | [Ant抽屉与Gin命令](../docs/migration/admin-product-edit.md)：0011专用能力、版本冲突、操作重放及同事务审计；实际MySQL八路并发/故障回滚与实际浏览器持久化、中英三宽验证 | 创建、选项/变体/关联、完整价格库存、完整RBAC/对象范围及Astro仍缺；通用目录写入保持关闭 |
| 变体价格配置 | [Ant/Gin查询](../docs/migration/admin-variant-prices.md)：共享集合、十进制字符串、阶梯数量、规则与价目表；人工MySQL边界及真实1558变体价格与SQL逐项验证，实际浏览器抽屉分页/中英三宽 | 配置展示不等于结算引擎；含税偏好、完整条件选价、价格写命令和库存仍缺 |
| 变体库存与预留明细 | [0012/Ant/Gin](../docs/migration/admin-variant-inventory.md)：2表27列、模型校验、精确库存余额及预留差异；固定样本927库存层/99预留及1558变体查询、实际MySQL/浏览器验证 | [0013逐行账本与4项差异证据](../docs/migration/inventory-row-import.md)已补充；[0014专用库存量编辑](../docs/migration/admin-stock-edit.md)已补充；全域导入协调器、订单解析/孤儿问题账本、预留/扣减/履约、完整仓库权限仍缺；通用写入关闭 |
| 生产替换 | 未进行 | 全量对账、回归及未来切换计划 |

当前没有任何业务域可据此声明已完成迁移。草稿不能连接旧系统数据库作为新后端，也不能代理 Medusa 冒充迁移实现。

## 基线与验证

详见 [业务基线](../docs/migration/business-baseline.md)、[设备命令](../docs/migration/device-commands.md)、[回收与邮件](../docs/migration/buyback-and-mail.md)、[数据映射](../docs/migration/data-mapping.md) 和 [前台基线](../docs/migration/frontend-baseline.md)。原始业务快照及截图仅留在 Git 忽略目录，不提交身份凭据或业务行数据。

新增契约见 [购物车与合计](../docs/migration/checkout-contract.md)、[API 约定](../docs/migration/api-conventions.md) 和 [后台组件](../docs/migration/admin-ui-contract.md)。样例仅用人工输入，不代表真实支付或新应用验收。
# 2026-10-28：真实浏览器认证补充

已通过独占Vite代理到实际Gin二进制、人工MySQL/Redis的浏览器登录闭环；见
[实证](../docs/migration/admin-live-browser.md)。开发HTTP下的Cookie、Origin/CSRF、
刷新及退出/旧会话拒绝已覆盖。生产TLS、客户权限和完整业务仍未验收。
后台优先Ant Design组件，全部图表使用ECharts；不制作虚构业务统计。

2026-10-28：[商品列表](../docs/migration/admin-product-list.md)接入Ant筛选/表格和
Gin明确字段投影，URL分页、稳定排序、软删及私密payload隔离有人工浏览器证据。
详情写入、完整结构提升、权限、价格库存和Astro仍待完成。

2026-10-28：[后台详情/变体](../docs/migration/admin-product-detail.md)改为明确关系
投影并接入Ant详情页。完整审查版结构用于隔离测试，正式schema升级仍未完成；
详情编辑、价格/库存、权限和Astro继续待办。

2026-10-28：[0010目录迁移](../docs/migration/catalog-schema-migration.md)正式接入
显式计划，45表374列只读结构；旧草稿有数据时改名前拒绝，空表保留改名。
启动校验完整10文件账本及目录模型，未实现的编辑入口明确503；全量仍未验收。

2026-10-28 库存导入补充：0013 与 inventoryimport 为 2 表 27 列增加逐行来源/目标账本；固定快照 927 库存层、99 预留共 1026 行的回滚/提交/精确重放通过，4 条不可自动释放的差异记录保留原事实。见[边界与证据](../docs/migration/inventory-row-import.md)。全域协调器、历史订单关联、库存写入与 hold 执行仍未完成。

2026-10-28 库存量编辑补充：[0014 与 Ant/Gin 专用命令](../docs/migration/admin-stock-edit.md)设置库存/在途的精确绝对数量；预留与设备独立保留。版本、持久操作重放、同事务审计和当前员工/能力复核；减少库存时拒绝历史 hold、实时预留合计差异或原负余额，补充库存不清除 hold。实际 MySQL 并发及回滚、真实浏览器和中英三宽验证见独立报告。尚缺完整库存写流程、权限/仓库范围、对账处置和 Astro，整个领域未验收。

2026-10-28 预留事务补充：[0015 内部参与组件](../docs/migration/inventory-reservation-transactions.md)在调用方同一 MySQL 事务内创建/释放预留、精确修改 reserved 并保存回执与审计；稳定位置锁序、共享库存需求合计、持久差异、重复/跨行释放保护。能力默认关闭，没有外部路由；订单状态/权限/渠道/行归属仍须领域接入。实际人工 MySQL 并发、旧快照重放、owner 后续失败回滚等有报告；checkout、履约/取消恢复、完整库存与 Astro 尚未完成。

2026-10-28 订单结构补充：[0016 与 orders.Ready](../docs/migration/order-schema-migration.md)正式承接 29 表/390 列，保留版本、成交快照及索赔/换货/退回。补原生枚举的 MySQL CHECK，守护同名表与历史主体无登录权限，启动/健康检查要求模型一致和写关闭。实际 MySQL 证据见联合报告；订单导入、权限/Ant 页面和交易流程仍未实现，不代表成单接入完成。

2026-10-28 订单导入补充：[0017 普通行与差异账本](../docs/migration/order-row-import.md)承接 28 表/378 字段，调用方事务内恢复循环关联、保存精确回执与不可自动解除的缺失引用记录；真实人工 MySQL 并发/回滚与固定快照 336 行演练见报告。受保护客户 5 行仍排除，正式全量协调器、订单读写/Ant 页面和交易流程尚缺，整个订单领域未验收。

2026-10-28 历史客户补充：[0018 专用主体转换与 SQL](../docs/migration/customer-history-conversion.md)保留业务字段，排除旧账号绑定，普通入口拒绝 customer；未知 metadata 不导入，已审查空基线密封。固定 5 行/60 字段与人工 MySQL 原子/并发/轮换验证见报告。正式全域协调器、新客户认证/历史归属、密钥恢复、订单查询/Ant 页面及交易链路仍缺。

2026-10-28 订单基础查询补充：[Gin 与 Ant 订单列表/详情](../docs/migration/admin-orders.md)支持严格筛选分页、当前版本成交快照和精确字符串数量/单价，缺失快照拒绝不完整详情。人工 MySQL、固定 13 个订单/14 条当前成交行及真实浏览器中英三宽验证见报告；读取不改变业务指纹。293 条订单引用差异继续保留。财务合计、支付/履约信息、订单写流程、完整权限、全域导入和 Astro 仍缺，订单领域未验收。

2026-10-28 金额核心补充：[Go 独立精确计算](../docs/migration/go-totals.md)承接含税/折扣/运费、退回与可退款、credit 和 pending_difference；47 组/1,644 十进制字段与原工具逐位一致，29 非法输入整体拒绝及并发/输入不变通过。无 Medusa 运行时或浮点运算。仍需实际订单/购物车数据装配、历史合计核对、Ant/Astro、价格税费促销选择与支付结算；整个订单/结账领域未验收。

2026-10-28 订单金额补充：[Gin 当前版本金额与 Ant 合计](../docs/migration/admin-order-amounts.md)按原实际版本/快照关系装配商品、税行、折扣、运费、credit/summary，保持 raw 独立回退和精确字符串。人工 MySQL 依赖/版本/精度、固定 13 订单/771 字段原计算器比较及真实浏览器验证见报告。完整业务指纹不变，历史差异保留；支付/履约动作、成单/退款事务、完整权限、正式全域导入和 Astro 继续待办，订单领域未验收。

2026-10-28 支付结构补充：[0019 正式支付证据结构](../docs/migration/payment-schema-migration.md)承接 8 表/83 来源列、状态/JSON/外键与活动会话唯一性；编译模型进入启动/健康检查，通用写能力关闭，同名表拒绝保留。人工 SQL 约束及正式 19 文件下真实 23 集合/42 支付/42 hold 隔离演练见报告。支付操作、provider 对账、主体导入整合、正式全域协调器及 Astro 尚缺，支付领域未验收。

2026-10-28 支付导入补充：[0020 与八表同事务导入](../docs/migration/payment-row-import.md)组合 5 普通表/46 字段与 3 专用保护策略表/37 字段，强制完整表键、完整复合主键回执、原密文重放和不可自动解除的逐字段关联 hold。人工 MySQL 并发/旧快照/失败回滚/密钥轮换通过，真实 95 行、42 支付 hold 与 1 主体 hold 已接入商品库存订单隔离演练；新建目标中 22 条付款集合父关系可解析，订单差异为 271 条，完整来源 375 项缺失仍保留。未恢复旧账号或开放 provider 调用；全域协调器、对账、交易权限及 Astro 尚缺，支付领域未验收。
