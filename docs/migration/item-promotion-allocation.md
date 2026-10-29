# 商品优惠分摊算术

2026-10-29：`totals.AllocateItems` 复用现有精确 decimal 层，为单条已完成资格筛选的商品/订单促销计算商品分摊。支持 fixed/percentage 与 each/once/across，保留输入行顺序、subtotal、original_total、整数数量、is_discountable 和共享未税已折扣余额。不改变调用者输入，返回本次金额、含税标志和新的共享余额 map。

## 原计算顺序

- 非可折扣、零 subtotal 行不参与分摊。once 按未税单件价格稳定升序排序，而不是按整行或含税价格排序；正折扣成功后消耗该行可用件数，即使剩余金额导致折扣小于名义额度，也按原数量消耗。
- each 默认最大件数1。先以单件价格乘上限限制可折扣基数；fixed 再以 value 乘 min(行数量, 上限) 限制优惠。percentage 作用于受限基数。once 缺省额度0，并按剩余额度复用 each 算术。
- across 分母是所有符合条件行的整行基数减已有折扣；max_quantity 只限制各行分子，0按原 truthy 分支视作未指定。保留负剩余对分母的影响。按比例除法后再乘优惠值，中间不按货币小数舍入。
- fixed across 超过行基数时，保留原额外两次除法的修正步骤，再执行最终 min。直接替换为 min 会改变低位小数。所有除法继续复用20位、半远离零的规则。
- 含税促销使用 original_total 作基数，将共享未税折扣按 original_total/subtotal 换到含税基数；本次折扣再按 subtotal/original_total 换回未税金额累加。连续含税与不含税优惠共享同一未税余额。
- epsilon 必须由可信服务器显式提供，不能从币种两位舍入推断或让浏览器设置。本轮人工来源明确配置为0.0001；可用基数或折扣不大于门槛则无动作。SQL数量域继续限制为正int4，非法金额/额度/重复ID/超限输入返回错误且不产生部分结果。

## 验证

`node retail/scripts/capture-item-allocation.cjs --check` 在独立 Node 进程中调用实际安装的 `getComputedActionsForItems`，明确设置该进程的 MEDUSA_EPSILON，不初始化旧应用、数据库或provider。固定33组独立预期、共享未税余额，以及 line-items、sort-by-price、usage、promotion/index、big-number、math 六个来源文件 SHA，见 `fixtures/item-allocation.json`。预期余额的加法使用独立BigInt十进制相加，不用JS浮点或原分摊函数计算预期。

Go逐项比对全部金额、含税状态和共享余额，并检查输入不变、返回map不共享、非法输入无部分结果及三次混合税基优惠耗尽余额。覆盖固定/百分比、三种分摊、默认/显式/零件数、稳定同价、先前折扣、禁用折扣、空/零价、epsilon边界、1/3和超安全整数。特别核对30×round20(100/300)=9.9999999999999999999，fixed across超限修正后的低位精度。Go全量测试与totals/promotion静态检查通过。

## 未完成范围

此入口不选择活动、code、自动促销或target规则，不处理campaign预算、buy-get、移除旧adjustment、促销排序、持久化或购物车版本。fixture里的促销都没有预算和target rules。资格筛选必须结合独立 `promotion.MatchRules` 与服务器真实上下文；预算拒绝后的额度/共享状态、完整活动流水需在组合编排中另验。商品/运费的算术通过不代表完整优惠流程或checkout已可用；T032/T033未验收。
