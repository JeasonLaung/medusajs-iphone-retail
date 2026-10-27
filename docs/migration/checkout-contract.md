# 购物车、金额、库存与成单契约

日期：2026-10-27。T002/T005/T007/T031—T036 的核对记录。已运行人工纯函数样例；尚未实现或验收 Gin 交易核心。

## 当前调用与真实运行证据

前台 `medusa-store/apps/storefront/src/lib/data/{cart,customer,fulfillment,payment}.ts`、`modules/common/components/cart-totals/index.tsx`；项目后端 `src/api/{payment-guards,device-guards}.ts`。本机实际安装 Medusa 2.21.2：core-flows 的 `dist/cart/workflows/{complete-cart,add-to-cart,update-line-item-in-cart}.js`、`dist/cart/{steps,utils}`；utils 的 `dist/totals/{cart,line-item,adjustment,shipping-method,tax,credit-lines}`。

已对实际安装的计算模块运行人工样例，不启动 Medusa、不加载 .env、不连接数据库或外部 API：

```powershell
node retail/scripts/capture-totals.cjs --check
node retail/scripts/capture-inventory.cjs --check
```

固定结果位于 [legacy-totals.json](fixtures/legacy-totals.json)（18 组）与 [legacy-inventory-input.json](fixtures/legacy-inventory-input.json)（10 组）。包含人工输入、预期、源代码 SHA-256；金额同时记录完整 decimal 与原 raw precision。独立断言核对各样例的两位总额/库存准备输出，不能代替完整 checkout、促销选择或支付舍入测试。

## 购物车生命周期

- 每次加购前实时读 cart 的 completed_at；失效或已完成则新建，防止缓存把已成单购物车继续用于购物。地区变化 update region_id 后刷新展示、配送和价格，不仅更改 URL。
- 新建带 region 和 locale；客户端只传 SKU ID/数量，不接受客户端成交价。数量更新、删除、选运费、地址和 promo_codes 都走服务端。
- 客户登录后当前 `transferCart` 关联当前购物车；未见“合并两个购物车所有商品”的项目实现，不能把 transfer 当作已存在完整跨设备 merge 功能。新契约区分归属转移与真正合并，不能凭迁移任务中的“合并”一词虚构旧行为。
- Mollie 初始化必须 cart.id 与当前 cookie 相同，回跳地址由服务器配置构造，保存支付尝试记忆。购物车在外部支付期间改变总额需要冻结版本或替换未付尝试，已付尝试不能静默改价。
- 成功 complete 返回 order 才清购物车并跳订单确认页；返回 cart/error 不当成功。跳转异常属于框架控制流，迁移不能吞成普通错误。
- applyPromotions 真正提交 promo_codes；礼品卡/移除礼品卡/旧 removeDiscount 是空函数，不作为已存在能力。促销移除的其他原生/可达 UI 路径仍需审查，不能据此说完全不支持。

## 金额字段与算法

价格、税、折扣为主要货币单位；Go/API 使用精确十进制字符串，device.cost_cents 独立。旧数值 API 的 helper 不能直接接字符串；Astro adapter 需明确处理。19 位展示号与所有 ID 都是字符串。

| 字段/规则 | 旧实际算法与语义 |
| --- | --- |
| 原行金额 | unit_price × quantity；含税时除以 (1+税率之和) 得未税 subtotal |
| 税率 | 多条 tax_lines.rate 相加，百分数 /100；当前非复利，不对每层税累进 |
| adjustment | 每条 amount 独立；自身 is_tax_inclusive 为 true 时先除 (1+taxRate) 得 discount_subtotal；不能按商品含税标记推断折扣标记 |
| 折扣基数 | adjustments 是整行折扣金额，不是每件；按原 quantity 分摊后用于当前数量 |
| 当前数量 | quantity - return_received_quantity - return_dismissed_quantity；仅 requested 不从当前成交数量扣除 |
| 行 tax_total | 当前未税 subtotal 减当前 discount_subtotal 后计算税 |
| 行 discount_total | discount_subtotal + 原税额 - 折后税额，不能在 cart.total 再减一次含税折扣 |
| 可退款数量 | 另扣 return_requested、received、dismissed；refundable_total_per_unit 是可退款口径，与已请求退回之前的 total 不相同 |
| 运费 | 含税/税行/adjustments 独立计算；不直接把 shipping.amount 当未税显示 |
| cart.subtotal | item_subtotal + shipping_subtotal（未扣折扣、未加税） |
| cart.total | subtotal + tax_total - discount_subtotal - credit_line_total |
| credit lines | 当前 cart 使用税率 0 的 credit 口径；与 gift card 占位不同，历史存在不能丢弃 |
| 原金额 | original_* 为未折扣口径；不是重新查现价，也不是 Apple 上市参考价 |
| 展示 | Intl 按币种格式化，计算中间结果不统一先 round(2)；外部 provider 的最终货币最小单位与订单舍入需独立明确 |

旧上下文 includeTax/automatic_taxes 与 item.is_tax_inclusive 有区别：显式 item flag 优先；不能把 automatic_taxes=true 简化成所有价格含税。`calculateTaxTotal(isTaxInclusive=true)` 返回额外税 0，但行合计会先取出税基再算已包含税；移植时不能据此漏掉含税金额中的税。

金额样例包括：100×2 未税 + 运费10，25% 税→262.50；125×2 含税 + 含税运费12.50→同值；未税整行折扣20/含税折扣25 均使对应商品总额225；数量3退回1时折扣按当前2件分配；仅申请退回1时当前总额仍337.50。细字段以固定 JSON 为核对对象。

`inclusive-repeating-division` 等样例会产生重复小数；源 raw precision 与全 decimal 不一定完全相同。MySQL DECIMAL(24,8) 不能成为所有中间运算精度，Go 内部需足够精度或有理数策略；落库/结算的明确舍入点另行验证，不能把显示同为299.97当作所有金额字段精确一致。

## 数量库存与预留

加购/更新工作流调用 confirmVariantInventory；实际下单流程 reserveInventory 才创建数量预留。不能为了迁移把所有放进购物车的商品永久占住，也不能把加购检查当作下单已锁库存。

输入准备按 SKU manage_inventory、inventory item、required_quantity、sales channel 对应仓库及 stocked-reserved；raw_* 优先。unmanaged 跳过；backorder 可绕库存覆盖，但不等于有现货。

prepareConfirmInventoryInput **只是准备数据**，库存不足仍可能返回已有 location_ids，后续 confirm/reserve 才决定成功或失败；固定样例特意保留这种输出。存在单仓全量覆盖时优先该仓，否则回退到有层级的仓库集合；reserveInventoryStep 选 location_ids[0] 创建预留，未见自动拆成各仓多条预留。新系统不能用“总量够就随意分仓”替代后续校验，跨仓拆配规则须整体核对。

每条预留 quantity=required_quantity×订单行 quantity；同 inventory item 多行合计、并发抢最后一件、跨 SKU 共享 item、取消/失败释放都应数据库事务保证。旧 Medusa 的 inventory locking 需要替换成 MySQL 行锁/条件更新和稳定锁顺序，不能只用 Redis 锁。

设备序列号档案独立，`guardDeviceInventory` 当前直接 next，并不禁止设备相关 SKU 的原生库存调整。退回/检测/入库不自动等同数量库存补回，详见 [设备命令](device-commands.md)。

## complete-cart 的实际顺序与风险

已安装代码：按 cart ID acquire lock（等待30s、TTL120s）→查询已有 order_cart 与 cart 快照→校验行与付款 session→若无订单则校验 shipping profile/库存、创建订单快照→创建业务关联、completed_at、预留、促销 usage 等→最后授权支付→记录 capture transactions→hook→释放锁→返回新订单或既有订单 ID。

- 同一 cart 重试返回既有 order，不可重复成单。新 MySQL 必须有 cart→order 唯一约束与可恢复操作状态，不能只靠时限锁。
- 付款 session 可处理状态为 pending/requires_more/authorized/captured/pending_authorization；provider 必须在 region 启用。状态可处理不等于实收。
- **原生 validateCartPaymentsStep 没有校验 session.amount 等于 cart.total**，complete-cart 注释也明确此点。新系统必须绑定购物车版本、币种和精确金额，已付不匹配进入恢复/对账，不照搬漏检；不能在 validate hook 改合计后继续使用旧支付金额。
- 授权在创建订单/预留等之后，失败有补偿。若已捕获付款后后续失败，旧补偿可能退款，但会检查是否另一尝试已成功成单以避免退掉成功订单。新实现需持久补偿指令，不在通用异常捕获中直接无条件退款。
- order.placed 事件可能在最终授权之前发出；邮件/设备分配不得只凭事件名认定已付款，必须检查事实状态，迁移 outbox 的发布时间和消费者条件显式定义。
- total<=0 且 credit_line_total>=0 时校验函数返回空 session；实际 complete-cart 后续仍取 paymentSessions[0].id，存在需隔离运行确认的边界，不能仅凭函数注释声称零元结账已验证可用。

## 配送、税费、促销尚待核对部分

已确认前台查 `/store/shipping-options?cart_id`，计算型 option 另 POST calculate，加入所选 option 后刷新购物车。complete 时需要配送的行必须有 shipping method，且商品 shipping profile 被所选 options 覆盖。DHL shipping-demo 保留演示边界。

还需审查：service zone/国家/邮编规则、flat/calculated 运价、税 provider/优先级与地区继承、promotion 资格/堆叠/预算/usage 和金额分配；样例中的 adjustment 不是已完成促销规则引擎。历史成交税/折扣/运费快照必须原样继承，不以现规则覆盖。

## 新 API 与验收规格（待统一 schemas）

新 `/api/store/carts`、cart detail/update、items create/update/delete、shipping-options/calculate/select、promotion apply/remove、payment-attempt/create/resume、complete；所有写操作验证会话/匿名 possession、Origin/CSRF 与幂等。公共 ID 不能单独授予私有 cart 权限；购物车合计由 Gin 计算，不信浏览器 subtotal/discount/tax。

CT01 已完成购物车不复用；CT02 region 改变重算价格/税/配送；CT03 登录转移不窃取其他客户购物车；CT04 加购不预留、下单并发不超卖；CT05 18 组精确金额及10组库存准备和拒绝边界；CT06 促销预算并发/过期不误用；CT07 shipping profile 覆盖与无配送选项拒绝；CT08 支付版本/金额/币种不匹配拒绝；CT09 同 cart 唯一成单、崩溃恢复和补偿；CT10 全退/申请退回/部分退款区分；CT11 零元/credit 边界显式结果；CT12 不按同邮箱赋予旧客户业务。新实现仍待验证。
