# 促销、税率与配送规则承接

日期：2026-10-27。T002/T005/T007/T033 的源码核对与人工样例；尚未实现 Gin 引擎。金额合计另见 [checkout-contract](checkout-contract.md)，这里区分“选择哪些规则”与“选好规则后如何合计”。

## 可重现证据与边界

`node retail/scripts/capture-commerce-rules.cjs --check` 对实际安装的 Medusa 2.21.2 模块运行 46 组纯函数人工输入：12 组商品促销分配、3 组运费促销、5 组促销规则、10 组税率选择/候选匹配、8 组配送规则、5 组地址查询条件、3 组配送规则验证拒绝。输出 [legacy-commerce-rules.json](fixtures/legacy-commerce-rules.json) 保留输入、独立预期和精确输出；执行来源 11 个文件及另外 6 个只读审查文件有 SHA-256。

没有初始化应用容器/服务构造器，没有读取 .env、访问数据库、调用 provider 或创建订单。税率调用未装饰的同步选择方法；配送地理条件调用静态方法。**该证据仅代表当前已安装产物**，不能以相同 package 版本声称其他机器产物完全一致；正式验证须核对哈希和锁文件，并查清本地依赖定制的可复现来源。不得为复现而改写运行中的 node_modules。

源码位置以各包 dist 为根：promotion 的 `services/promotion-module.js`、`utils/{compute-actions,validations}`；utils 的 `totals/promotion/index.js`；tax 的 `services/tax-module-service.js` 与 `providers/system.js`；fulfillment 的 `services/fulfillment-module-service.js`、`utils/utils.js`；core-flows 的 `cart/workflows/{list-shipping-options-for-cart,list-shipping-options-for-cart-with-pricing,add-shipping-method-to-cart}.js`、`tax/steps/get-item-tax-lines.js`。

## 促销资格与顺序

- `listActivePromotions_` 要求 status=active；有 campaign 时 starts_at 为空或 <=now，ends_at 为空或 >now。开始包含、结束不包含，同一次查询共用 now。
- `computeActions` 收集既有 adjustment 的 code 和新提交 code，默认纳入自动促销；先生成移除旧 adjustment 的动作，再按本次请求 code/自动促销重新添加。移除 code 不能只是删前端标签。
- 查询按 application_method.value DESC；之后 buy-get 类型排序先处理，并保留共享资格/已折扣 map。不能简单依用户输入 code 顺序逐个相乘，也不能默认为所有券互斥或仅取最大券。
- 通用促销限制、campaign 按属性额度、币种及订单级 rules 在分配前校验；商品 target_rules 和 shipping target_rules 另按对应 scope 评估。资格不成立与预算超限有不同动作，前端需要明确反馈。
- rules 数组整体 AND；每条必须有 attribute 和有效字符串 values。`eq` 要求取出的每个值都在规则集合，`in` 只需任一个命中，`ne` 要求全部未命中；范围比较通过精确数字比较。不能把 `eq` 和 `in` 对数组视为相同。缺失正向属性拒绝，不应通过前端补默认值扩大资格。

## 分配、税基和额度

| 行为 | 已安装实现 |
| --- | --- |
| 商品 fixed + each | 金额上限由 min(行数量, max_quantity 或默认1) × value 及可用金额共同限制；未指定额度不是自动乘整行数量 |
| 商品 percentage + each | applicable base 同样受 max_quantity 默认1的件数范围约束；百分数 /100，不当 fixed amount |
| across | 按符合条件行的剩余金额比例分配；同一行已用促销从基数扣除；中间不先 round(2) |
| once | 按单件未税价格升序消耗 max_quantity；同价比较返回0，保留输入次序；无 max_quantity 当前配额为0 |
| 非可折扣/零基数商品 | 跳过；不能仅用 code 存在就给所有行添加折扣 |
| 含税促销 | 以 original_total 作基数，既有 map 始终累计未税已折扣；按 original_total/subtotal 在两种税基转换 |
| 运费 each/once | 每个 shipping method 一次金额；once 按运费升序、额度按 method 数消耗 |
| 运费 across | 按剩余运费比例分配；百分比直接作用各 method 剩余未税基数；不能混用商品行 max_quantity 公式 |
| spend / usage / use_by_attribute | 分别按折扣金额、使用次数、属性使用次数处理；不能统一视作券张数 |

例如 fixed each=10，subtotal200、quantity2、无 max_quantity 得折扣10；明确 max_quantity2 才得20。含税 original125/未税subtotal100，既有未税折扣90，含税券25最多再折12.5，而非再折25。预算 used90、limit100、折扣10恰好允许；used95则产生 budgetExceeded。

分配函数里的预算检查只是预检查，并不在遍历中持久累加 campaign.used。真正 `registerUsage` 在事务中按稳定顺序锁 promotion/预算行、重新读 used，再累加校验与保存；没有事务会明确拒绝。新 MySQL 要有相同的最终额度约束，并以业务操作唯一键防重；不能以计算成功或 Redis 缓存额度作为提交成功。撤销注册与失败补偿按原操作追踪，不能无限减预算。

`limit=0` 等真假值边界、buy-get 的买入/获赠资格分摊、自动促销查询预筛选及并发预算注册仍需扩展样例与隔离集成验证；不因当前没有配置或纯函数样例通过而删掉这些能力。

## 税区与税率选择

税行生成要求 force_tax_calculation 或 region.automatic_taxes，并需要 shipping address.country_code；非强制时缺国家返回空税行，强制缺国家抛错。giftcard 行被过滤。当前税 provider 是否有配置仍以源数据及运行配置核对，不能硬编码欧盟统一税率或把未配置税区视为法律免税结论。

TaxService 规范化国家/省代码为小写，查询国家与省税区；无国家父税区返回空。候选包括默认税率与匹配商品、商品类型、shipping option 的规则，按以下优先级选择第一个：

1. 省级商品/配送规则；2. 省级商品类型；3. 省级默认；4. 国家商品/配送规则；5. 国家商品类型；6. 国家默认；其他为7。

例如省默认可覆盖国家商品专用规则。被选税率 `is_combinable` 且有 parent_id 时，再取该父区最优先的一条，不是把全部候选相加。同级同分按输入稳定次序，源查询若没有确定排序需记录配置冲突，不能在迁移时擅自将 rate 最大者设为赢家。

system provider 将选中的 rates 变成商品/运费税行，保留 rate_id/rate/name/code/provider_id。税率选择与价格 is_tax_inclusive 独立；多条选中税在合计函数相加而非复利。历史订单已有税行属于成交快照，不用今天地址或税率重算覆盖。

## 配送选择与保存

- 必须有 cart.sales_channel_id、region_id、currency_code。沿 sales channel→stock locations→fulfillment sets 查询 options；来源仓库/渠道关联不能只迁移为文本名称。
- 地址条件投到 service_zone.geo_zones：country、province、city、zip 从窄到宽 OR 匹配。静态构造器只加实际存在的上级字段，例如有国家和邮编、没有省市仍会生成 zip(country,postal_expression) 与 country 两项；不是要求所有上级字段齐全才允许查询。具体数据库邮编匹配方式与大小写需在 MySQL 转换时独立验证。
- option.context 默认 is_return=false、enabled_in_store=true（字符串形式）；无 rules 放行，有规则默认全部满足。boolean value 经规范化成为字符串；嵌套字段可取值；区间比较对日期/数字区分。旧配送规则数值比较用了 JS Number，新精确金额边界必须避免超精度误判，并登记兼容差异。
- 旧 validateRule 在规范化前用真假值判空，因此 JSON boolean false 和 number 0 被错误拒绝，字符串 `"false"`/`"0"` 不受此问题影响；已固定两组拒绝样例。新 schema 应明确接受类型合法的 false/0，仅拒绝缺失值，属于纠正验证边界，不改变既有字符串规则的匹配结果。
- flat 运价走价格集合及当前 cart 的地区/币种/条件；calculated 运价向 fulfillment provider 请求，带 option data、method data、cart 与发货地/对应 shipping profile 的行。不能把演示 DHL 查询伪装成通用运费 provider。
- 公开列表为 option 附 `insufficient_inventory`：对应 location 缺层级或 available_quantity < item.quantity 时提示；unmanaged/backorder 及不需运输的 inventory component 跳过。此处未乘 required_quantity，和最终预留校验不是同一算法，不能据此放宽最终库存检查。
- 前台 `modules/checkout/components/shipping/index.tsx` 会计算 calculated option 的价格；库存不足、没有 number 价格、保存中或支付中禁用选择/继续。新 API 金额字符串需 adapter 明确适配，零运费合法，不能用 truthy 判断。
- 保存配送先校验 cart、options 可用、价格已定义，provider 校验 data；使用服务端计算金额及含税标记，不接受浏览器运价。相同 shipping_profile 的旧 methods 被替换，不同 profile 保留；随后刷新商品/税/促销并发 cart.updated。成单还需覆盖全部需配送商品的 shipping profile。

## 新 API 与验证要求

促销 apply/remove、税/配送重算都进入购物车版本及幂等契约；客户端只提交 code、option ID 和允许的 provider data，不能提交自造 adjustment/tax rate/amount。后台促销与税区、运费配置使用 Ant Form/Table/Drawer，错误与范围权限来自 Gin；记录修改版本，历史快照不被配置修改覆盖。

验收需新增：自动/手动促销叠加与移除、buy-get/上限/过期、同预算并发、地址/地区变化、税区组合/缺失、free shipping=0、calculated provider 超时/不支持、多个 shipping profile 保存、跨渠道 option 拒绝、运费价格变化导致支付版本失效。当前 46 组只覆盖纯规则子集，不声称这些集成场景已运行。
