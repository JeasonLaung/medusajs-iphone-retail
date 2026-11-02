# 本机退款开关与独立worker启动

2026-11-02，真实沙箱退货退款此前已在独立副本完成，但主Windows预览的退款gate和进程配置仍关闭，start.bat没有启动退款worker。本批接入原有 `process-order-refunds`，不新写退款执行逻辑、不在API内派发、不重新走完整购物。

`configure-local-preview --refunds` 复用原本机严格边界：development、TCP 127.0.0.1:13307、eurorefurb_local_trial、Mollie test key、本机无外部webhook模式。要求原checkout/keyring/退款adapter可用且ORDER_REFUNDS_ENABLED=true；事务锁定order_refund_runtime，核对版本4后启用。缺失表或错误版本失败回滚，不迁移DDL、不创建指令、不改退款金额。独立编译命令实际验证production、live key、错误数据库、未显式启用退款四种配置均拒绝。

本机忽略.env增加 `LOCAL_PREVIEW_REFUNDS_ENABLED=true`、`ORDER_REFUNDS_ENABLED=true`，示例默认false。start.bat对应原生脚本显式调用配置命令，构建、隐藏启动独立 `order-refund-worker.exe`；记录PID和独立日志。补款与退款复用同一个启停函数，按记录PID核实精确.exe路径，先停止旧worker再构建启动，PID被其他可执行文件占用时拒绝终止。实际连续启动两次，两个旧PID退出，各只有一个新worker存活；不按全局Node或worker进程名批量停止。

主库启用前退款指令0、补款指令0；启用并启动worker后仍各0，客户订单归属0，库存50/预留1，未制造售后、退款、设备或测试资金流。邮件worker继续关闭。管理员按正式一次性challenge登录后的实际只读售后接口200；主库不存在的售后退款读取仍按原语义409，不能把它当成已有退款可操作的证据。主环境没有为测试UI创建真实售后。

固定完成状态恢复检查：

```powershell
python retail/scripts/test-local-customer-account.py --verify-completed-refund=d9bf3fae2419
```

此命令要求原报告为通过且real_mollie_sandbox_refund/refund_completed，恢复其 `refund-final.sql` 到随机独立数据库并临时授权，编译当前生产worker，以 --once 运行。核对原789 EUR退款refunded观察已应用、唯一负向789交易、唯一本人退款通知、原退货申请完成及客户归属各1。运行前后orders/order_items/payments/payment_captures/payment_refunds/order_transactions/order_refund_operations/inventory_levels/device_assignments/customer_account_notices十表指纹一致；保留新验证文件与worker摘要，清理副本/授权/容器临时文件。

本批该命令通过，证明完成状态worker重启不重复记账；它没有重新创建或退款，不代表再次完成一笔真实退款。原pending→refunded的真实渠道查询与唯一记账证据仍见 [原退款主流程](local-refund-main.md)。本批证据见 [启动、门禁拒绝与完成状态恢复报告](local-refund-runtime-report.json)。

主health9100、Admin9200及受影响Astro账户订单/商店路由9300均200，近期原生服务日志正常，无临时19100/19200/19300监听。旧9001/8000按已迁移运行方式保持关闭。本机此后从实际售后提交的退款指令会由独立worker按现有规则执行Mollie沙箱退款；pending只表示处理中，须等待认证查询/refunded及本地应用，界面不能先记成功。

本批不关闭C06/C09/C01—C11。未知结果、渠道失败、新尝试、生产回调、真实邮件收件和其余交付继续按清单推进；独立PayPal尚未开放，主测试key不提交Git。
