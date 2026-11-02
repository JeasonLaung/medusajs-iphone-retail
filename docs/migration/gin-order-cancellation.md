# 已收款原生订单整单取消

本项补 C07/C10 的已收款、未发货原生订单取消主链路。整体 C01—C11 未完成；未捕获付款撤销、历史支付凭据和其他提供者仍有边界，不能据此宣布所有订单均可取消。

## 行为与原项目对照

参考 `medusa-store/apps/backend/src/workflows/cancel-order-with-reason.ts`、原 Admin 取消表单，以及安装的 Medusa 2.21.2 `order/workflows/cancel-order.js`、`payments/create-order-refund-credit-lines.js`：

- 必须填写取消原因，保留其他 metadata，并记录原操作者及实际完成时间。
- 已取消、已完成和草稿订单不能再次取消；所有履约包裹必须事先取消。已发货订单走退货，不把取消当作库存回收。
- 对每笔原付款计算精确的实收减已退款余额，生成退款指令；退款确认后生成退款抵扣的新订单版本，再取消订单及付款集合、释放原预留库存。
- 新增版本保留原商品/运费版本快照。现金退款仍通过原退款流水及负交易记账，不以 credit line 代替真实退款。

新协调器将网络和本地事务分开：取消申请、每笔退款及其前驱都是持久记录；外部结果不明时继续持有订单冻结。退款 `pending/queued/processing` 不算退款成功，也不释放库存。已发送过的退款只允许查询原指令；明确 `failed/canceled` 后由管理员确认新的退款尝试。

取消指令有独立领域记录，未伪造售后申请。Mollie 请求使用 `order_cancellation_id` 和原退款操作 UUID；普通退货保留原 `after_sales_request_id`。原商户 key、原付款 ID、币种和金额校验保持。官方接口依据：[创建退款](https://docs.mollie.com/reference/create-refund)、[查询退款](https://docs.mollie.com/reference/get-refund)。

## 入口与部署

Ant 订单详情新增“取消整单”，包含原因、退款状态、失败退款的新尝试及同一提交丢响应后的恢复。仅具备订单读取、付款读取、售后处理、退款、配货和履约读取权限的当前管理员可操作。仓库权限不会自动获得整单退款权限。

接口：

```text
GET  /api/admin/orders/:id/cancellation
POST /api/admin/orders/:id/cancellation
POST /api/admin/orders/:id/cancellation/refunds/:refund_operation_id/retry
```

初次提交的 JSON 为字符串字段 `operation_id`、`expected_version`、`reason`；新尝试只传新的 `operation_id`。所有写入经过现有登录、Origin、CSRF 校验和严格对象解码，不由浏览器提供退款金额、远端付款或退款状态。

显式迁移 `0106_order_cancellation.sql` 新建取消记录及默认关闭的 runtime，退款行新增可空的取消归属，并使售后归属与取消归属互斥。退款 runtime 升到 v5，旧 worker 遇到新结构会拒绝执行。没有启动 DDL。

本机专用库先迁移，然后从 `retail/backend` 执行：

```powershell
go run ./cmd/configure-local-preview --refunds --order-cancellation
```

该命令仅适用于已配置 Mollie 测试收款/退款的专用本机预览。原 `ORDER_REFUNDS_ENABLED=true` 的退款 worker 同时恢复取消终态；API 只保存指令，不在请求事务中调用支付商。使用 `start.bat` 恢复本机服务；密钥仍在忽略的 `.env` 中，不写入仓库或浏览器。

## 固定检查点验收

```powershell
python retail/scripts/test-local-customer-account.py --cancellation-from-checkpoint=790ce0fc9e71
```

恢复原已付款订单及其新账户归属，使用独立 MySQL 数据库、Redis 前缀和 19100/19200/19300 服务。实际运行 Astro、Ant、Gin、SQL 事务及通知投递器；外部退款适配器和 Resend 传输采用人工测试实现。本项没有新发起真实 Mollie 退款或付款，也没有向真实邮箱发送消息。

验证包括：

- 六并发同一取消提交和同一失败退款新尝试，各只保存一份指令；浏览器丢响应后恢复原 UUID。
- 原退款明确失败后显式新尝试；新尝试丢响应后只查询，后续 pending 保持冻结，refunded 才记账。
- 退款通知写入的晚故障回滚现金流水，但保留独立提交的已核验观察；恢复从缓存入账，不再调用渠道。
- 最终取消的晚故障回滚版本、抵扣、库存、原因和通知；六并发恢复只生成一个取消终态。
- 预留设备释放为可售、分配取消的事务检查；装箱/已发货设备拒绝释放。该分项使用人工设备记录并整体回滚，不冒充实际入库验收。
- 陈旧版本、已完成订单、仍有有效包裹、停用账户、关闭开关、外部订单和未捕获付款均拒绝；结果不明时订单改价被拒绝。
- 中英文 Admin 和 390px 客户页出现取消状态、唯一时间线及两类通知；退款/取消邮件经过正式队列、投递器和后台失败重试，冻结正文及幂等键保持。
- 终态 SQL：订单 v2、取消记录 1、退款流水 1、负交易 1、退款抵扣 1、原预留释放收据 1、两封业务邮件 sent、待处理差额 0。
- 原付款、实收、归属、商品快照、原商品/运费版本、实物库存、设备和履约指纹保持；独立资源清理并保留忽略的前后 SQL。

最终所有者、API SHA 和回归结果见 [验收记录](order-cancellation-native-report.json)。普通 `go test ./...` 不会自行运行这些 Windows 原生检查点；不能把单元测试通过当作上述联合验收。

## 未关闭的边界

- 当前入口限于本系统可证明原收款来源的订单。没有原付款/密钥证据的历史订单保持拒绝，不按邮箱绑定新账户，不要求恢复旧私钥。
- 未捕获且未撤销的付款目前拒绝整单取消；尚未接入独立撤销协调器。不能将本项描述为已完成未付款/授权付款取消。
- 原生实收取消的外部退款本次采用人工适配器；真实 Mollie 已在先前退款检查点恢复到 refunded，但该证据不能替代本项独立的真实渠道验收。PayPal 退款及更多配送提供者仍待。
- 旧检查点的商品图片缺失、旧页面提示文案及真实邮件送达等仍归 C11/C10，本项不据截图宣布完整页面或整期验收。
