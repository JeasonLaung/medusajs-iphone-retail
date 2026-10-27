# 业务迁移基线：第一轮核对

日期：2026-10-27。T001、T002 进行中。本轮只读检查源码，不改变业务数据、不启动新服务、不访问部署服务器。

## 已确认的范围与证据

自动追踪表见 [源码证据追踪表](source-traceability.md)，可机读数据见 [source-manifest.json](source-manifest.json)。生成器从仓库根目录运行 `python retail/scripts/inventory-business.py`；只读取源码，不读取 `.env`、凭据或数据库快照。

扫描结果：67 个自定义路由文件、86 个 HTTP 方法导出、12 个自定义模块、23 个自定义 Admin 页面、6 个 widgets、29 个顶层工作流、3 个定时任务和 4 个事件订阅。前台检测到 36 处原生 SDK 调用候选。每个自定义路由已关联承接任务，并记录直接工作流导入；权限统一标记待 T004 审查，不能由 URL 或 TypeScript 请求类型推定已安全授权。

静态匹配不是调用图：SDK 泛型、别名、封装、动态路径、插件和 Medusa 内置 API 可能不在候选中；注释/未使用函数可能产生候选。未被检测到不意味着没有使用，检测到也不意味着线上已启用。源文件 SHA-256 用于后续发现变化；证据仍需人工核对。

## 原生能力与插件的承接表

下列证据路径相对仓库根目录。配置存在、代码消费与线上实际启用分开判断，尚不声称已验证商户配置或全部后台操作。

| 业务 | 当前证据 | 新系统必须承接的行为 | 任务 / 当前判断 |
| --- | --- | --- | --- |
| 商品、分类、集合、区域 | `storefront/src/lib/data/{products,categories,collections,regions}.ts` | 商品详情、字段关联、区域价格、分类层级与列表分页；软删除及公开范围不能丢失 | T020、T021、T026；确认存在消费 |
| 销售渠道与来源仓库 | `backend/src/workflows/device-source.ts` | 创建来源时创建仓库并关联默认销售渠道；缺少默认渠道会拒绝创建 | T020、T031、T042；不能删除渠道模型只保留来源名称 |
| 原生库存和预留 | `backend/src/workflows/device-operations.ts`、`hooks/device-cart-stock.ts` | 数量库存、预留创建/更新、库存调整及履约联动；设备号码不限制结账数量 | T031、T043、T044；确认核心工作流依赖 |
| 购物车生命周期 | `storefront/src/lib/data/cart.ts` | 创建、取回、地区更新、商品数量、地址、配送、完成与已完成购物车恢复 | T032—T034、T039；确认存在消费 |
| 客户认证、邮箱验证及地址 | `storefront/src/lib/data/customer.ts`、`backend/src/api/registration-guards.ts` | 注册、验证、登录、资料、地址、购物车归属转移；使用新账号，旧账号和密码不迁移 | T014、T030、T032；确认存在消费 |
| 订单与所有权转移 | `storefront/src/lib/data/orders.ts` | 列表/详情、归属与转移请求接受/拒绝；历史标识保留，新账号不能凭同邮箱取得旧记录 | T034、T039；确认存在消费 |
| 配送价格 | `storefront/src/lib/data/fulfillment.ts`、`cart.ts` | 配送选项及动态 calculate；地址、地区或行项目变化后重新计算 | T033；确认存在消费，实际服务区/运费规则待核对 |
| 促销 | `storefront/src/lib/data/cart.ts` 的 `applyPromotions` / `submitPromotionForm` | 提交 promo_codes、错误反馈及合计刷新 | T033；确认代码接入，实际活动、条件和折扣叠加待核对 |
| 税、条件价格与价格表 | `storefront/src/lib/data/products.ts`、`cart.ts`、Medusa 原生价格/购物车能力 | 币种、税包含方式、价格条件及精确舍入需固定样例 | T021、T033；实际规则待配置及数据核对，不能默认零税/统一价格 |
| Mollie | `backend/medusa-config.ts`、`src/modules/mollie`、`src/workflows/resume-mollie-session.ts` | 托管支付、已有会话恢复、地区 provider 验证、回调与旧浏览器返回 | T035、T036；配置与实现存在，不代表真实商户已核实 |
| PayPal 与 PayPal Card | `backend/medusa-config.ts`、`storefront/src/lib/data/payment.ts` | 插件 config、onboarding 依赖、普通 PayPal 与卡支付；历史交易及退款兼容 | T035、T036、T047；插件接口也需替代，不能只搬 Mollie |
| 手工付款限制 | `backend/src/api/payment-guards.ts`、`middlewares.ts` | 生产禁止 system provider、mark-as-paid、相关授权/扣款及购物车完成路径；开发允许不等于生产允许 | T035、T036；需纳入负面用例 |
| 履约与发货 | `backend/src/workflows/device-operations.ts`、`src/api/admin/orders/[id]/fulfillments` | 创建履约、发货、取消、分批数量与设备分配一致 | T037、T044；有原生 workflow 调用 |
| 原生退货/换机/退款 | `backend/src/workflows/device-operations.ts`、`device-refund.ts`、`src/api/device-guards.ts` | 退货申请/收货确认、换机请求、实收核验、退款；原生入口也受设备规则限制 | T045—T047；不能只迁移自定义售后表 |
| 原生后台库存与订单覆盖 | `backend/src/api/admin/{orders,inventory-items,stock-locations}` | 在原生响应上做仓库/供应商范围限制和字段裁剪；部分方法委托框架实现 | T015、T031、T038；需追踪委托的底层行为 |
| Analytics | `backend/medusa-config.ts`、`src/lib/admin-analytics.ts`、`src/admin/i18n/json/analytics-en.json` | 订单/销售趋势、收货国家、订单状态、热销、缺货/低库存、客户及群组统计；中文、图表和错误反馈 | T027；配置与界面集成存在，统计 API 与金额/客户口径待审查 |
| 后台编译时定制 | `backend/src/lib/admin-{device-fulfillment,warehouse-menu,inventory-sorting,analytics}.ts` | 履约选择、受限菜单、库存排序与统计展示需要显式迁移到 React 后台 | T023、T027、T038、T048；不能仅重写 23 个自定义页面 |
| 本地公开/私有文件 | `backend/medusa-config.ts` 的 file-local provider、`src/workflows/device-photos.ts` | 公共图片与受保护照片/附件分开迁移；不沿用所有文件公开静态挂载 | T016；配置与工作流存在 |
| 锁、任务及事件 | 工作流 core-flows 锁、`src/jobs`、`src/subscribers` | MySQL 事务及持久化操作/outbox 承担事实和可靠重试；Redis 临时状态与失效策略独立定义 | T017、T019；不能把 workflow 补偿替换成无持久化 goroutine |

上表 `backend/` 指 `medusa-store/apps/backend/`，`storefront/` 指 `medusa-store/apps/storefront/`。它是第一轮行为清单，不是 Medusa 全部 API 规范。

## 必须单独拆动作的接口

| 入口 / 验证器 | 已发现的动作与分支 | 后续要求 |
| --- | --- | --- |
| `/admin/devices` / `src/api/admin/devices/validators.ts` | 更新、启用、批量可售、删除、报废、转退回、来源、库存接管、入库、供量、配货、解配、履约、发货、取消履约、收回、检测、维修返还等 | 每个 action 的角色、来源/仓库、状态、幂等和库存影响分别写入 T004/T005/T007 |
| `/admin/mail/commands` / `src/api/admin/mail/validators.ts` | 邮箱、草稿、发送、联系人、退订、活动、模板；消息 operation 有 read/unread/star/unstar/trash/restore/archive/delete/retry，活动有 queue/pause/resume/retry | 不能只列“邮件 CRUD”；分别覆盖持久化队列、收发状态、附件及重试 |
| 回收请求 / `src/api/buyback-validators.ts` | 客户提交照片/寄出/接受或拒绝终价/取消；后台照片审核/允许寄出/收到/设备关联/终价/付款/退回等 | 12 个业务状态与各动作的合法边必须转写为固定用例 |
| `/admin/seo/commands` / `src/api/seo-validators.ts` | save/delete/scan/run/indexnow/google-test，跨 settings/page-metas/crawler-sources/providers/citation-monitors | 分清读写、外部调用、长任务与凭据，不把它当一个保存表单 |

## 已识别的演示与占位

- `/admin/custom` 和 `/store/custom` 的 GET 当前只返回 200，是模板占位，不是业务 API。先登记为占位，最终保留或清理由接口兼容清单决定。
- `cart.ts` 的 `applyGiftCard`、`removeGiftCard`、`removeDiscount` 函数体是注释占位，不能据此声称礼品卡已实现。`applyPromotions` 则有实际更新购物车调用，二者不能混为一谈。
- DHL shipping-demo 需保持既有演示边界，不能当作已接入生产物流。
- Stripe 依赖存在不证明已启用该支付方式；以 provider 配置、调用与商户核验为准。

## 本轮发现的清单缺口及处理

1. 原任务缺少独立 Analytics 任务，已补 T027：审查插件路由/聚合逻辑，明确销售额、退款、国家、时间区间、客户与历史主体口径，并迁移 React 展示。
2. 用户已决定不迁移旧账号。因此“客户总数/新老客户/群组统计”不能直接混合新登录账号和历史交易主体；T006/T027 需定义历史统计口径，保留必要历史快照，不恢复旧账号身份。
3. 原生后台促销、税区、价格表、草稿单、订单编辑、客户群组、API key、区域/渠道等仍待核对实际使用与约束。没有自定义源码或静态调用不能作为删除依据。
4. 自定义接口虽已全部有任务去向，调用方、真实授权、全部副作用和数据范围尚未逐个审计，因此 T001 仍为进行中；T002 也未完成。

## 下一批具体工作

- T001/T004/T005：已展开 [22 个设备命令](device-commands.md) 及 [回收与邮件](buyback-and-mail.md)；继续补齐剩余接口的调用方、权限与副作用。
- T002：已提取 [Analytics 口径](analytics-contract.md) 与 [支付第一轮基线](payment-contract.md)；继续核对插件实际编译产物及原生交易规则，不改运行中的依赖产物。
- T003：已有 [78 张公开/未登录页面截图索引](frontend-baseline.md)；继续补动态商品与交互、隔离登录状态，当前开发账户仅用于只读检查，不为了截图改变真实订单、设备、库存或支付。
- T006：已有 [193 张表的去向及转换要求](data-mapping.md)，继续逐字段映射和关联校验；不把全表元数据清单当作全量导入完成。
- 补充领域证据：[授权矩阵](authorization-matrix.md)、[商品关系及页面消费](catalog-contract.md)、[主商城搜索](search-contract.md)、[SEO/流量/引用监测](seo-contract.md)。已核对范围和剩余缺口见 [基线索引](README.md)。
- 上述基线与契约形成后再继续审查/实现 `retail/backend` 草稿，当前没有把编译通过当业务验收。
