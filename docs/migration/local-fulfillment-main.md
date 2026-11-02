# Windows 配货／装包／发货主流程

2026-11-02 接续真实 Mollie 测试购物订单。主预览中的履约运行开关仍为关闭，所购 SKU 没有可用设备；仅订单详情可读不能代表可发货。本批增加本机显式履约配置，并在独立数据副本实际操作原 Ant 页面。

## 实际验证

将当前专用开发库以 single-transaction、GTID关闭的逻辑快照恢复到唯一 `retail_fulfillment_52d85a4b083d` 数据库。API19100和Admin19200使用该副本以及独立 Redis 前缀；主9100/9200/9300保持运行，未启动共享数据库的第二个后端。测试副本使用同一笔已由真实Mollie测试渠道付款的新订单 `order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d`，不伪造实收。

仅在副本预置一台 `ISOLATED-FULFILLMENT-001` 可售设备，绑定订单SKU、原订单预留仓库和测试来源。最初未填IMEI时，真实分配命令400拒绝且未保存；补齐测试IMEI后，经真实React Admin登录、设备配货、保存预配货、创建配货包裹、装包扣库存、包裹操作及整包发货完成。物流号为 `ISOLATED-TRACK-001`，没有调用承运商预约或寄出真实包裹。

实际SQL核对设备和分配均shipped，包裹 `ful_d4c7518b-8e4c-4564-b371-2414e048bb01`，一条发货命令收据，订单 fulfilled/shipped数量各1，标签物流号一致。目标库存从50/预留1变为49/预留0。装包扣一次数量库存，发货登记未再次扣减。主库相同订单仍为fulfilled/shipped各0、库存50/预留1。

运行截图、API日志、浏览器结果和快照保存在忽略的 `logs/retail-native/fulfillment-52d85a4b083d/`；报告见[可提交证据](local-fulfillment-main-report.json)。测试后停止两个专属进程、删除副本数据库、按精确前缀SCAN/DEL清除8条专属Redis键，不使用FLUSH，不改主Redis会话。

## 可操作预览

`configure-local-preview --fulfillment` 在现有专用 development/loopback/MySQL库限制下核对版本，再显式开启设备identity/commands及allocation/packing/parcel命令。事务配置可重复执行，不自动添加设备、改订单或扣库存。`.env` 的 `LOCAL_PREVIEW_FULFILLMENT_ENABLED=true` 让start.bat每次应用同一配置；新Admin原页面无需重新设计。

本机已重复运行配置并通过devices/orders局部测试及命令go vet。测试数据只存在于已清理副本；主预览需要由实际业务登记有效设备后才能配货，不把数量库存伪造成设备。

## 仍未完成

本轮实际验证平台Admin配货到手动发货登记。当前真实Mollie测试购物为游客订单，未冒充已验证客户或按邮箱继承订单；客户账号查询/物流通知联合、仓库员工独立初次配货、真实承运商及邮件收件仍需继续主流程验证。C07及C01—C11保持未完成。
