# Analytics 迁移核对

日期：2026-10-27。关联 T002/T027。检查本地安装的 `@agilo/medusa-analytics-plugin@1.4.0` 实现及本项目编译转换；仅阅读依赖产物，未修改。

## 旧接口证据

包内 `.medusa/server/src/api/admin/agilo-analytics/` 有三个 GET：orders、products、customers，均不计入仓库的 67 个自定义路由。原代码见该目录下对应 `route.js`；日期帮助函数在 `utils/orders`，界面变换在本项目 `src/lib/admin-analytics.ts`。

| 接口 | 参数与输出范围 | 已发现的真实计算口径 |
| --- | --- | --- |
| orders | preset=this-month/last-month/last-3-months/custom，自定义需 date_from/date_to；当前/前期、销售/订单趋势、国家及状态 | created_at UTC 日界；仅排除 draft，因此包含 canceled；总额来自 order.total，未在该接口另减 refund；国家用 shipping_address.country_code，否则 unknown |
| products | date_from/date_to；热销、低库存/缺货 | 热销按订单行 quantity，排除 draft；缺省阈值 5，查询 stocked_quantity，而非扣预留后的可用量；具体跨仓合并和分页须继续核对 |
| customers | date_from/date_to；客户分布、群组销售、排行榜 | 排除 draft/canceled；按 customer.id 去重；新客户根据其全部订单是否均在起始日以后判断；同客户多个群组可能每组计入销售额，因此群组总和未必等于总销售额 |

orders/customers 将多币种按店铺默认币种折算（无配置默认 EUR），使用 Frankfurter 最新汇率并缓存至 Europe/Berlin 下一次 16:00，**不是成交日汇率**。时间分组超过 30 天按周、超过 120 天按月；具体端点范围和前期日期由帮助函数生成。新接口必须记录币种、日期范围及汇率来源，缓存故障不能静默将金额当作同币种。

上述为旧实现证据，不是建议将其叫作“净实收”。迁移初始按明确标签保留业务统计语义，另用测试固定取消订单、退款、缺汇率、未知国家、没有前期数据等输出；不在没有业务确认时把销售口径偷偷改为支付实收或退款净额。

## 新账号与历史数据

用户已确认不迁移旧登录账号。历史订单的原客户标识、必要统计分组/业务快照可以保留在历史主体映射，不创建可登录客户。统计应明确历史交易主体与新注册用户的来源；不能按相同邮箱自动合并、给新用户授予历史订单或把旧 customers 记录整体作为新账号导入。

客户总数、活跃购买客户数、新注册数是不同指标。当前插件 customers 基于期间购买者，不应仅用全表新注册用户数替代。历史群组归属应在 T006 确定快照映射，无法恢复时明确未分组，不编造归属。

## React UI 要求

按用户指示使用 Ant Design 的日期筛选、Tabs、统计数字、表格、Skeleton/Empty/Alert 等合适组件；所有曲线、柱状和分布图使用 ECharts。沿用中英词条、国家名称及错误反馈，处理容器 resize、实例 dispose、加载/空/错误状态。图表尺寸变化不改变查询范围；表格/图表共享确定的响应 DTO。

后端统一权限和仓库/来源范围；未授权范围不能经聚合统计泄漏。Redis 只保存带明确参数及授权范围的缓存，不能缓存不带范围的全局结果后给受限账号返回。

## 继续核对及验收

- 阅读插件日期帮助函数、所有分页/默认 take 和订单行版本，确认大数据量不会静默漏行。
- 固定跨时区月底、闰日、周分桶、前期为零、全空数据、取消订单与退款样例。
- 固定多币种与缺汇率样例，后端用精确 decimal，展示层只负责格式化。
- 用库存 stocked=5/reserved=5 与多仓重复 SKU 样例，明确缺货/低库存标签对应库存指标。
- 验证历史主体与新账号分离、同邮箱不合并、群组重复计数有准确标签。
- 在新 Gin 查询和 ECharts 界面都有结果之前，T027 保持未完成。
