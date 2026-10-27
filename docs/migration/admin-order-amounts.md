# 订单当前版本金额与 Ant 展示

2026-10-28。Gin GET `/api/admin/orders/{id}/amounts` 在新员工当前 admin 会话后，
使用同一只读 REPEATABLE READ 事务装配订单当前版本的财务数据，调用独立
[Go 金额核心](go-totals.md)。没有 Medusa 运行时或旧库代理，不触发 provider。

## 来源装配

已核对安装的 order 2.21.2 `utils/base-repository-find.js` 与 `transform-order.js`：
商品版本行、运费版本、商品/运费 adjustments、credit 及 summary 均要求订单当前版本；
税行依附成交商品/配送快照，不能附加不存在的税行版本条件。
已删订单、版本行、快照、税行、折扣、credit 与汇总分别过滤，不从现目录查价或税率。

成交价格的普通数值和 raw 采用原格式化器各自独立的 NULL 回退：
COALESCE(i.unit_price,l.unit_price) 与 COALESCE(i.raw_unit_price,l.raw_unit_price)。
最终计算 raw 优先；因此普通单价已 NULL 而 raw 仍有值时，不能错误清掉原 raw。
输出 effective unit_price/quantity 明确使用同一来源；原订单基础详情接口的数据库字段
展示保持原有行为。所有金额均为字符串，没有 JS Number 或浮点乘加。

按有效成交快照逐项加载原数量及履约/发货/申请退回/收到/驳回/核销数量，
按当前版本加载折扣。运费版本须对应有效运费快照；运费折扣模型本来没有
is_tax_inclusive 字段，沿原计算器缺省 false，不凭配送含税标记推断它。
币种 decimal_digits 读取 commerce_currencies 的有效正式定义，缺失时拒绝猜测。

汇总只投影 transaction_total 与 raw_transaction_total，计算 pending_difference；
没有有效 summary 或 totals 为 SQL/JSON NULL 时返回 summary:null。缺失 transaction_total
按原规则取零；未知汇总字段不进入响应，原 JSON 不修改。重复顶层键、无效 raw
结构/值或重复当前 summary 均拒绝，不用 JSON 后值静默覆盖金融事实。
记录交易额与待处理差额不证明已收款，也不替代 payment/capture/refund 状态。

## 响应和边界

返回 order_id/version/currency_code、合计、按版本行 ID 对应的成交行金额、配送快照
名称/金额/合计和明确摘要。metadata、data、地址、客户主体、provider/外部付款引用、
原 summary JSON、raw envelope 不公开。未知 query 为 400；缺失/软删订单 404；
依赖不完整、模型漂移、超限、计算拒绝统一 503，不透出 SQL、业务行或部分成功金额。

最多 1000 商品行、100 运费行、1000 credit；每个父行最多 100 税行/折扣。
读取总数与 JOIN 数量不等时拒绝截断；相同当前商品快照或运费快照重复绑定也拒绝。
不会修复历史差异、补假父对象、改变历史订单或启用订单写能力。

## Ant 页面

独立金额区沿用 Ant Space、Typography、Descriptions、Table、Alert、Spin、Button，
明确列商品小计、运费小计、未税折扣、折后税额、抵扣、当前合计及存在时的记录摘要。
配送列表展示原成交名称和计算合计；精确长数字可换行，小屏表格内横向滚动。
请求取消、加载、失败重试和 401 会话失效复用现有方式；金额失败仍保留基础详情。
当金额响应版本与当前详情不同，拒绝把两个版本混在同一页，提示刷新详情。
无图表，不生成虚构统计；后续图表统一使用 ECharts。

## 证据

- [实际人工 MySQL](admin-auth-report.json) 的 order_read：18 文件执行、当前版本折扣/
  配送/credit 排除旧版和已删记录；大额 18 位小数、独立 raw 回退、空订单及缺失对象。
  重复 raw/summary、已删配送、已删币种、无效税 raw 和重复当前 summary 均失败；
  查询前后 11 表指纹相同，修正人工故障后可恢复。私密标识未进入响应。
- [固定业务快照](catalog-mysql-rehearsal-report.json)：私有源经独立 Node 原计算器装配预期，
  JSON number 使用 source 文本保留超过 JS 精度的位数；13 个有效历史订单的
  Gin 财务响应与原计算器完整 JSON 投影一致，771 个合计/成交行/运费字段比较通过。
  汇总、版本、原数值、名称和 ID 也完整比较，预期只在 stdin/内存及临时 MySQL tmpfs，
  不写 Git 或普通 stdout。完整业务读取指纹不变。
- [真实浏览器](admin-live-report.json)：实际 Gin/MySQL/Redis 和人工账号，Ant 金额、
  精确大额、配送名称、中英文 390/768/1440 宽度截图；非法 query 与退出后拒绝。
- [人工 HTTP UI](admin-ui-report.json)：金额独立失败重试、缺失摘要不伪造、版本冲突
  拒绝混合显示；原后台回归与 build/type check 共 14 项。
- [纯金额证据](go-totals-report.json)：47 组/1,644 字段一致，29 非法输入整体拒绝。
  Go 全包测试/vet 通过；这不替代上述显式启用的 SQL 与浏览器演练。

固定 293 条订单引用差异、全域 375 项引用缺口、库存 4 项 hold 均未修改。
完整支付/履约详情与动作、成单/退款/订单变更事务、权限对象范围、正式全域导入协调、
价格/税费/促销选择、provider 结算及 Astro 仍待完成。订单与结账领域未验收。
未连接部署服务器或修改原开发业务数据，旧服务继续可用。
