# 终止付款后返回结账

2026-11-03，固定证据：[failed-payment-retry-report.json](failed-payment-retry-report.json)。

原结果页对 canceled/expired/failed 提示可以返回结账，但 `retry_url` 从未赋值，因此实际没有入口；即使手动打开结账页，原付款绑定及创建日志仍会保留冻结，不能开始新的有效尝试。

现在按钮复用现有付款上下文协调器。点击后先按当前持有凭证和CSRF重新查询付款，只有正式返回 failed 才持久化 `payment-retry` 意图。原地区、币种及语言保持；每次重试沿同一操作UUID和版本继续。服务端原显式上下文事务对已认证终止、无人工核查标记的会话安排保护退役，即使金额不变也不再复用失效会话。原渠道适配器仍须完成核验、持久effect收据及最终事务，不能凭浏览器状态解冻。

成功后配对核对当前购物车、金额及付款选项，确认付款绑定为空才清除原付款日志并返回付款步骤。没有在返回操作中创建替代付款；用户再次确认才走新的原创建命令。丢成功响应后刷新结果页，会恢复原 `payment-retry` 操作。未完成的普通上下文保存不冒充此操作，退役重试不提供创建原付款的补偿按钮。请求期间清除上一次错误并禁用按钮，避免把旧提示当作当前响应。

没有新API、DDL、权限或支付配置。只扩展现有上下文协调器对终止付款的处理；相同金额且付款仍有效时，原绑定与CREATE版本保持。paid/authorized、needs_review、未知结果、现金事实、其他购物车和不一致绑定继续拒绝。Mollie原适配器对 canceled/expired/failed 使用认证GET完成退役、不发送HTTP DELETE；已付款/授权和未知状态保护由原适配器测试继续覆盖。隔离浏览器中的 `deletes=1` 指人工适配器的一次退役effect调用，不能称作真实远端DELETE。

## 独立验证命令

```powershell
node retail/scripts/test-context-payment-joint.mjs --scenario retry-canceled --scenario retry-expired --scenario retry-failed --scenario unchanged --scenario continuous --scenario group-restore --matrix
node retail/scripts/test-context-payment-joint.mjs --paid-order-recovery
```

最终 `d0a543a5c002` 通过24项真实 Gin/MySQL9.6/Redis/Astro/Chromium场景，107迁移，源码冻结匹配并清理自有资源：

- canceled/expired/failed × 中英 × 390/1440 共12项：从现有付款页准备一次158.75 EUR付款，再进入正式结果页，通过按钮继续。
- 每项在最终上下文收据注入SQL失败：原版本5、原checkout、1个活跃会话及两份前端日志保持，最终收据0；已持久化的退役effect及冻结供原操作恢复。
- 去除故障后同操作完成，刻意丢掉成功响应。SQL版本6、原checkout及refresh为空、活跃会话0、上下文收据1，CREATE仍1/退役调用仍1；浏览器保留原日志。
- 刷新后同UUID/版本5恢复原收据，核对当前数据后清理两份日志并返回付款步骤。原158.75 EUR和各项税费/优惠合计保持。只有再次点击付款复核才创建第二次付款，版本7、新UUID/输入版本6、唯一新活跃会话、两条原创建尝试。所有重复上下文请求的UUID与载荷一致。
- 原 unchanged、continuous、group-restore 各中英双宽度共12项同时通过，保留有效付款和连续保存/原报价补偿语义。24项库存/订单/实收/邮件等保护指纹保持，无新增订单。

另一个最终隔离环境 `4ffdc257127c` 通过上一轮已付款待成单8项回归：原121.25 EUR优惠额度冲突、双击单请求、解除夹具冲突后成单丢响应、刷新及四布局原收据恢复。目标收据/实收/交易/归属/通知/优惠登记各一条，渠道CREATE仍3，证明终止付款退役没有破坏已付款恢复。

前端275项全量、Go全包、cart/identity vet通过；最终Astro检查0 errors、0 warnings、29 hints。未重复执行选品至售后的整条业务流程，外部付款仍为人工适配器，没有真实收费、退款或发信。

运行器首轮 `25d65a3573fd` 缺少场景局部金额读取函数，第二轮 `78148b55dc0c` 因旧错误仍可见导致丢响应断言提前结束，证据保留。已补局部函数、清除旧错误并等待本轮实际响应；最终两组从冻结源码完整执行通过，失败环境亦已清理。

通过原 `scripts/start-retail.ps1` 更新本机三端及独立worker。当前Gin API SHA256为 `19b2a9713788dfca2c5d4c3024d67596e249416f5aa86ffc28f345666ccdfcd1`。服务复查和日志见报告；旧9001仍按既有迁移预览安排停用。

真实渠道终止付款再付款、PayPal专有取消/授权分支、运营待处理付款队列、生产回调、实际收件及C01—C11剩余范围保持开放。本项不替代整期结束时的完整流程验收。
