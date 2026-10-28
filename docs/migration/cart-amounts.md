# 购物车持久金额与私有读取

2026-10-28。`internal/cart/amounts.go` 将真实关系表中的行项目、税行、折扣和配送快照装配到现有 Go 精确计算器。新增 `GET /api/store/carts/:id/amounts`，与精简 cart detail 分开，保持旧客户端详情契约。

## 授权和快照

复用同源检查、独立 HttpOnly possession 和 Redis 解析，然后在 MySQL 只读 REPEATABLE READ 事务中校验当前店铺、代数、active、版本、地区/币种和匿名归属；原 customer ID、邮箱、Admin 登录或裸 cart ID 不授予访问。未配置/关闭 `cart.access` 继续拒绝。响应 private,no-store，包含请求编号，不签发或续期 cookie，不写订单、库存预留或业务表。

输出 cart_id、字符串 version/currency_code、明确 totals、行 ID/可空保存 variant_id/数量/单价/含税标记、配送 ID/名称/金额/含税标记及各行 totals。只读取保存价格与快照，不重新查当前 SKU 覆盖历史金额；原 SKU 不存在仍保留保存的标识。metadata、raw JSON、provider payload、客户及地址不返回。

`pricing_recalculated=false`、`checkout=false` 明确其为持久金额投影，不是当前选价/税率/促销/配送重新计算，也不是付款授权。完整加购命令、共享库存校验、变地区/数量、实时规则引擎及原页面接入仍未实现。本功能不把 T032/T033 标为完成。

## 精确计算

`totals.StoredNumber` 严格读取显式 scalar/raw 对：raw 优先，拒绝重复/未知字段、非规范数值、非法 precision、坏编码及过长 JSON，坏 raw 不回退 SQL scalar。不使用 float，不把 DECIMAL(38,18) 当作全部中间精度；有效 raw 值可保留更多小数。购物车价格、运费及折扣的非负约束同时检查有效 raw，不能通过 JSON 绕过源列约束。

商品每行显式含税标记，混合折扣含税标记独立，税率逐行累加；配送折扣源模型没有含税标记，沿用已审查的不含税折扣语义。cart 税行源表没有 raw_rate，读取 rate；价格和折扣严格读取 raw。当前 cart 结构没有 credit line 表，不虚构历史 credit。币种小数位取真实 currency 目录，不猜默认2位。

仅选本 cart 的未软删行/配送及其未软删关系。超过1000商品行、100配送、任一父行100税/折扣时整个读取失败；不分页截断后返回小计。单个金额响应上限由客户端单独放宽至4MiB，原创建/详情仍为64KiB；数量/金额/版本字符串不经Number，所有字段严格校验，重复行或多余私有字段拒绝。只有401/404视为不可访问，503不变成空车。

## 验证入口

- Go 全部测试及 vet；数值解码覆盖坏输入与精度边界，不改变已有47组计算对照。
- `python retail/scripts/test-admin-auth.py --cart-runtime-only` 与全套入口均运行真实 Gin/MySQL/Redis 金额检查，报告 `cart_amounts` 及源码SHA。人工100×2、25%税、商品折扣20、含税运费12.5/折扣2得到235；超SQL小数精度案例严格读回 `22517998136852467.5000000000000000000025`。
- 实际 SQL 行/税数量超限整体503；raw损坏/负价/缺币种拒绝。对购物车、行、关系、归属/回执及库存预留前后做指纹，读取不改变它们。
- 行读取后由另一连接提交税率25→50，进行中的读取保留原快照，下一读取观察新税率；不靠定时等待或人工组装替代真实SQL隔离。
- `test-cart-client.py` 运行30项单元、Astro类型/build及原人工HTTP协议浏览器；新增金额测试包括200行超过64KiB、数值型金额/重复行/额外字段拒绝与4MiB上限。
- `test-admin-live.py` 在真实浏览器加载实际客户端，经正式Gin读取新空车全字符串金额，陌生及丢失possession读取拒绝；后台原浏览器与Astro原组件回归仍由联合入口执行。成功证据以 [专项报告](cart-runtime-report.json)、[数据库联合](admin-auth-report.json)、[浏览器联合](admin-live-report.json)、[Astro](astro-runtime-report.json) 和 [客户端](cart-client-report.json) 的当前SHA为准。

现有业务库和旧账号不参与测试，没有开通支付或生产购买流程。只读金额不取代实时购物车命令和完整结账验收。

联合回归曾在操作区人工测试页的首个 HTML 请求超时，购物车真实浏览器与SQL回查已通过，但该次整组结果仍判失败。操作区测试改为先以60秒上限完成SSR页面编译准备并核验200/页面标题，再启动浏览器；保留15秒浏览器导航限制及全部交互/截图断言，报告单独记录准备耗时。这是测试环境准备的隔离，不将其作为生产冷启动性能验收，也不宣称已证明所有超时原因。

最终联合入口重跑通过：2项真实浏览器及SQL回查、30项Astro单元、类型/build、40对展示和24对操作区截图、依赖审计0问题。该次SSR准备5009ms，浏览器导航仍15000ms；五份报告的148/59/8/143/170个源码SHA分别逐项匹配。两个owner标签下容器/网络均为0；原health、Admin、商品页、购物车页完整读取均200，stderr未出现本次新错误。
