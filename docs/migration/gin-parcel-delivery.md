# 包裹送达登记与客户查询

2026-11-03，补C07主流程缺失的后台“确认送达”。原项目安装版本的 `@medusajs/core-flows` 2.21.2 `order/workflows/mark-order-fulfillment-as-delivered.js` 在标记包裹送达后登记订单行送达数量；原客户进度读取fulfillments.delivered_at。Gin此前只有ship/cancel，无法从页面产生这条事实。本批在现有手动设备包裹操作里接deliver，不依赖Medusa运行时、不改页面布局或主题，无新DDL。

## 行为和边界

`GET/POST /api/admin/orders/:id/fulfillments/:fulfillment_id/operations` 复用原私有会话、Origin/CSRF、分配/履约权限、runtime门禁、全局设备/库存锁、完整版本摘要及不可变操作收据。deliver的tracking_number/tracking_url必须为空，不能覆盖已记录物流。入口只接当前已发货、未取消、设备及分配仍shipped的manual_manual设备包裹；设备数量必须匹配包裹商品与订单行，送达累计不能超过已发货数量。

同事务增加当前成交行delivered_quantity及精确raw镜像，写入fulfillments.delivered_at、订单更新时间，推进设备revision并写原包裹收据；`delivery.created`站内通知和业务邮件也在该事务入队。任何晚失败全部回滚。付款、捕获/退款、库存/预留、设备及分配状态、原物流、订单价格和摘要均保持；没有承运商调用或自动完成整张订单。

Ant包裹抽屉对已发货包裹默认选择“确认送达”，确认后禁用重复操作。已装包包裹沿用原发货/取消；两者在已发货包裹上禁用。未知响应保留原命令并锁定输入，重试读取同一收据。客户现有中英文订单进度和物流组件直接读取唯一送达事实，无需新状态模拟。仓库员工的原shipment入口仍只接受ship；新增操作在全店订单管理员入口。其他提供者、非设备包裹/库存套件不在原生设备包裹支持范围内。

## 固定验收子流程

```powershell
# 从已付款、已实际入库并发货的持久检查点开始，隔离库与Redis
python retail/scripts/test-local-customer-account.py --delivery-from-checkpoint=a27835e587f6
```

运行前使用当前源码编译受运行器路径约束的API，可通过ACCOUNT_TEST_API_BINARY指定忽略日志目录内的独立exe。不重跑注册、购物、付款、入库或发货。检查点迁移至105，关闭付款创建/上下文及实际发信worker；仅外部Resend传输使用example.invalid限定的模拟器。应用、浏览器、数据库和Redis均为真实原生链路。

最终owner `fc8a8658f2c5` 的[公开报告](parcel-delivery-native-report.json)与原始 `logs/retail-native/account-fc8a8658f2c5/` 记录：

- 当前已发货包裹、原已验证客户及自动订单归属原样恢复。Go专用夹具验证旧版本/错误订单/重写物流拒绝，收据末尾故障使时间、数量、通知和库存全表指纹回滚；超量送达、已取消包裹、已退回设备、停用账号及关闭门禁拒绝。
- 实际Ant点击确认并发送六个同UUID请求，唯一fresh及五次replay；丢弃首次响应后以原UUID/版本/内容恢复。随后新的送达、发货、取消指令均409，不能叠加数量或撤销送达。中英文后台均显示完成并禁用按钮。
- 实际Astro英/中390px订单页各显示唯一timeline-delivered及DeliveredAt，原物流labels完全相同、无横向溢出，唯一delivery.created通知，浏览器异常0。
- 正式业务邮件投递器六并发只一次渠道POST，拒绝后实际后台重试并丢弃响应，按同指令恢复；冻结正文/幂等键保持，最终唯一sent/投递收据和一条重试审计。这里不代表真实Resend送达，也没有重跑既有邮件安全矩阵。
- SQL确认送达包裹1、送达数量1、送达邮件及投递收据各1、重试审计1；设备revision只增加一次。付款/库存/设备/物流及除送达计数和更新时间外的订单投影指纹不变，主库客户/售后计数不变。私有最终SQL保留，独立数据库/授权、Redis及进程清理通过。

首次夹具尝试以应用凭证建触发器受限，已改为运行器通过已有root辅助配置只影响固定测试UUID的故障触发器；另三轮为测试语言初始化、Ant中文按钮空格及恢复英文逻辑定位错误。失败报告和快照保留，未放宽生产校验、修改付款/渠道状态或重跑完整购物链。最终成功场景覆盖此前所有阶段。

Go全包测试、相关vet、Admin TypeScript、Python编译和JS语法通过；真实数据库证据来自本轮专用夹具，不冒称全部23套SQL重跑。实际中英文后台及客户截图已检查送达内容和可读性；旧快照的订单商品图片仍有资源缺失，留在C11历史文件/数据组合验收，不将本轮截图称为整页资产验收。

通过现有scripts/start-retail.ps1受控更新主预览，主API与隔离验收API的SHA256一致。9100 health、9200 Admin、9300商店及客户订单入口均200，近期日志正常；旧Medusa 9001 health/app仍无监听，属于已停用旧服务。没有对主订单登记送达或发送邮件。

整单取消（原流程还包含捕获退款、未捕获付款取消、预留释放和取消原因）、售后拒绝邮件、更多配送提供者、真实邮件送达、正式数据/历史文件及整期最终流程仍待。C01—C11继续开放。
