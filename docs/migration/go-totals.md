# 独立 Go 金额计算核心

2026-10-28。`retail/backend/internal/totals` 以 Go 标准库 math/big 计算商品、
运费、税费、折扣、抵扣及数量生命周期合计；没有 Medusa 运行时、JavaScript、
数据库、Redis 或外部调用。输入是领域层提供的可信成交/购物车快照，不直接接收
浏览器成交价，不选择价格、税率、促销，也不授予历史订单归属。

来源为本机固定 Medusa utils 2.21.2，含项目实际安装的原有修正。
原有 18 组样例保持不变；[新增 29 组边界](fixtures/legacy-totals-boundaries.json)
记录十个源码/算术依赖文件校验和，检查实际 bignumber.js 的除法配置。
捕获器只运行人工纯函数，不加载应用、环境文件、数据库或业务数据。

## 计算与精度

- 普通输入和 raw 值均使用字符串；有效 raw 优先，不经过 float64。
  raw precision 保留为输入来源约束，不作为再次截断 raw.value 的精度。
- 加减乘精确保存所有中间位；除法沿用原实现的 20 位小数、五入且中点远离零。
  这不是统一保留 20 个有效数字，也不是数据库 DECIMAL(38,18) 的存储量化，
  更不是支付提供商的结算舍入点。输出是完整有限小数字符串。
- 商品显式含税标记优先于地区/config 默认；每条 adjustment 独立按自身含税标记计算。
  商品的整行折扣按原数量分摊，按当前数量生效。每条税率分别除 100 后计算税额，
  不以先合并税率再量化替代，以保留极小税率的原有舍入差别。
- 当前数量扣 received/dismissed，requested 单列；可退款数量再扣 requested。
  fulfilled/shipped/requested/received/dismissed/written-off 按原完整数量净单价派生；
  仅提供相应数量字段时输出派生金额。原逻辑中可退款含税判断与继承标记的区别保留。
- cart total = subtotal + 折后 tax_total - discount_subtotal - credit_line_total；
  discount_total 已包括折扣税，不能再次从上述 tax_total 中整项扣除。
- 原金额、运费折扣和抵扣按原算法分别计算。币种小数位必须由调用者提供正式定义，
  不从未知币种猜测。仅 credit 合计与 summary.pending_difference 使用原有的
  abs(amount) <= 一个最小币种单位归零规则，包括 EUR 0.01、JPY 1、KWD 0.001。
  该规则不应用于所有总额、退款或外部付款金额匹配。
- 保留原有负数结果，如超额折扣或历史退回超过数量，不擅自截零或修正事实。
  新交易是否允许这些输入由领域命令验证，不由这个纯计算器授权。

## 拒绝与资源边界

缺失币种定义、非法/非有限数、无效 raw、超限指数/长度、重复非空行 ID、
除零或超限行数等返回统一 ErrInput 及零 Result，输入保持不变。
每次最多 1000 商品行、100 运费行、1000 抵扣行，每行最多 100 税行和 100 折扣；
输入最多 128 个十进制数字、64 位小数，科学计数指数在 -64 至 64 内。
这些是有界计算接口约束，不是凭空修改数据库源值。
原逻辑零数量且请求 refundable 产生非有限结果的输入明确拒绝，不输出部分合计。

## 验证

```powershell
node retail/scripts/capture-totals.cjs --check
node retail/scripts/capture-totals-boundaries.cjs --check
python retail/scripts/test-totals.py
```

[报告](go-totals-report.json)绑定实现与样例校验和：47 组、1,644 个合计/商品/
运费/折扣/summary 十进制字段逐位比较，包含完整 38 位金额、raw 覆盖、重复小数、
多税率、混合税标记、数量生命周期、正负容差边界、科学计数及输入不变。
另有 29 种整体拒绝、独立算术舍入/精确乘法断言及八个并发调用各 25 次和结果隔离。
Go 全包测试及 vet 通过；需要 opt-in 的数据库测试没有在这次纯计算验证中执行。

仍须接入订单/购物车的实际版本、行税与折扣、运费/credit/summary 查询，
使用真实历史业务样本核对，补 Ant 完整合计和 Astro，以及定价/税率/促销选择、
支付提供商最小单位结算、成单/退款事务与恢复。此核心不代表订单或结账领域验收。
未修改原开发数据或部署服务器，旧服务保持可用。

2026-10-28 后续接入：[订单实际金额与 Ant 展示](admin-order-amounts.md)已实现当前版本财务投影；固定 13 个历史订单与原计算器完整比较通过。此查询不确认已收款，也没有启用 checkout 或订单写流程。
