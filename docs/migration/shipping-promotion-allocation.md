# 运费优惠分摊算术

2026-10-29：`totals.AllocateShipping` 复用现有精确 decimal 层，为单条已经完成资格筛选的促销计算运费分摊。输入包含 fixed/percentage、each/once/across、可选件数额度、按原顺序的方法未税 subtotal 和共享既有未税折扣；返回本次金额与新的共享折扣 map，不改变输入。

每项 fixed 按每个方法一次金额；percentage 作用于该方法扣除已有优惠后的金额。once 按原 subtotal 升序稳定排序，同价保留输入次序，只有产生正折扣的方法消耗一个额度；缺省额度为零。across fixed 先以全部剩余金额为分母计算比例，再乘优惠值；percentage 直接作用各方法剩余金额。单项折扣不超过可用金额，不给非正金额生成动作。所有减法保留精度，除法按原 MathBN 配置在20位小数处半远离零；不先按货币两位舍入。

特别保留原 across 分母中的负剩余值：若旧折扣大于当前 subtotal，先保留其差额参与合计，再在单项正金额判断时跳过；不能先对每项 clamp(0) 改变分配。零总额/负总额不分摊。未知方法的既有共享 map 条目保留，调用者负责资格和上下文完整性。

`node retail/scripts/capture-shipping-allocation.cjs --check` 执行实际安装的 promotion `getComputedActionsForShippingMethods`，固定20组独立预期和共享余额、来源 shipping-methods/sort-by-price/usage 三个文件的SHA，见 `fixtures/shipping-allocation.json`。所有人工促销无 campaign budget、无 target rules；不启动旧应用或访问数据库。Go对照全部20组，并验证输入与输出隔离、三轮连续优惠封顶、非法枚举/额度/金额/重复ID/超限输入无部分结果。`go test ./...` 和 `go vet ./internal/totals ./internal/promotion` 通过。

这里只是算术阶段，没有活动查询、通用/target资格调用、campaign预算检查/注册、优惠顺序与移除动作、税费重算或SQL写入。调用方不能直接把这些金额作为已授权优惠保存；必须先完成资格、预算及整车事务。范围内的人工数据不覆盖预算拒绝后额度消耗与继续分摊行为，后续组合编排需另行核验。商品促销、buy-get和完整购物车命令仍待完成，T032/T033未验收。
