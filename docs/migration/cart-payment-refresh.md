# 购物车支付刷新决策与准备

2026-10-29：新增`cart.PlanPaymentRefresh`及`PreparePaymentRefresh`，承接原refresh-payment-collection工作流的决策和私有准备阶段。它们尚不执行支付渠道调用、删除本地session或更新集合，不能将返回计划当作刷新完成。

原流程的shouldExecute取决于输入：提供cart对象时看是否有payment_collection，否则看cart_id。金额使用原raw精确比较；金额相同则只判断币种，金额不同则使用shouldExecute。需要变更且集合存在时，准备更新原集合的amount/currency并删除其有效session；不创建缺失集合。Plan实现这几个分支，保留有效会话顺序，验证ID、币种、非负精确金额及最多100会话，错误不返回部分计划。

`capture-cart-payment-refresh.cjs`从当前安装的原工作流中提取shouldExecute、should-update及两个准备transform回调，以原MathBN对15组人工输入执行：有cart/只有cart_id/无输入、无集合、相同金额、等价scale/exponent、币种变化、超出scalar小数位的差异、大数精确相等、零总额、无session等。该脚本不运行工作流、不连接应用或支付渠道；Go纯测试逐组比较结果，另测非法金额/币种/重复或超限session。

Prepare在调用方事务重查当前cart授权/version、payment schema，读取税/运费/促销后的实际金额。按固定顺序锁定cart_payment_collections、有效payment_collections及payment_sessions。链接或集合软删不参与，多有效集合拒绝，超过1000链接/100会话拒绝，不截断。scalar/raw成对解析，坏raw不回退；输入固定采用已获取的cart分支。返回目标集合ID、准确新金额/币种、需要处理的旧session IDs及决策。数据仅供内部支付协调，不可直接返回浏览器或记录日志。

## 验证

原15组回调对照及Go非法输入测试通过。隔离CartRuntime人工MySQL验证无集合不创建、币种/精确raw变化生成计划、等价金额不使session失效、有效session保留而软删session排除、多集合/坏raw/schema关闭拒绝。调用后原集合币种不变，后续同一事务仍能读到未删除session；最终整体回滚核对购物车/库存原指纹。autocommit拒绝，授权/version由既有LockForWrite执行。

cart-runtime及全量报告cart_promotion_amounts增加payment_refresh_preparation_without_mutation，记录实现、原对照采集脚本及fixture SHA。fixture需原脚本--check复核，报告SHA必须同时匹配。

## 后续必须实现

原delete-payment-sessions实际调用provider.deletePayment，删除失败会验证结果并尝试重建已经删除的会话；不能用SQL软删代替外部会话失效。后续需要显式的持久操作状态、provider适配和幂等/重试/补偿，在本地数据与外部副作用间保持可恢复状态，并在最终金额/币种一致后完成购物车命令。尚未启用任何新支付provider调用，也未声明支付刷新或checkout完成。T032/T033未验收。
