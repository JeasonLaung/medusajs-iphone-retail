# Gin 同 SKU 换机：资格与候选设备

2026-10-30。本批只完成真实资格读取，不标记换机或整个前台闭环。POST 换机命令、订单变更、新商品行、配货和发货联合仍须继续。

## 已接入

`GET /api/admin/orders/:id/after-sales/:request_id/devices/:assignment_id/replacement` 绑定订单、申请、原销售分配和设备。需要员工的订单、付款、设备、仓库、商品、库存读取及 `order.allocation.write`；专用仓库身份不能进入全店接口。查询参数和空问号均拒绝。

响应返回原机/订单商品/SKU/当前订单版本、资格原因、候选设备及事实摘要版本。`write_available` 固定为 false，读取不预留库存、不创建分配、不修改任何业务记录。不将候选读取暴露成可以完成换机的 UI。

资格继承原流程：已收回的原分配、处理中换机、未关联原生退货、无 reserved/packed/shipped 替换分配、订单有效、所有付款集合已 completed。取消的旧替换分配不阻止重配。普通订单已全部履约不限制售后换机。

候选复用真实分配读取：订单商品原 SKU、sale/available、没有活动销售分配、来源与设备仓库一致、仓库关联订单销售渠道。原机检测已改 SKU 时仍使用订单 SKU。响应不包含 IMEI/EID、成本、备注、事件、操作者或原始支付内容。

## 原执行链及后续写入约束

来源为 `medusa-store/apps/backend/src/workflows/device-operations.ts` 中 assign/replacement_of 和 device-native-exchange 分支；同时核对当前安装的 Medusa 2.21.2 order 模块 `utils/actions/{item-add,return-item,receive-damaged-return-item}.js` 与 `utils/apply-order-changes.js`。

原流程先校验 paid、原分配 returned、处理中 exchange、没有活动替换，再创建真实 order exchange、请求原商品退货、以订单原 unit_price 添加一个同 SKU 新商品行、确认 exchange、begin receive/dismiss/confirm receive 隔离原机。最后将替换分配绑定新商品行，将原分配绑定 exchange 的 return_id。不能直接给原销售行追加一条替换分配并宣称迁移完成。

RETURN_ITEM 增加 requested；RECEIVE_DAMAGED_RETURN_ITEM 增加 dismissed 和 written_off、扣回 requested，不增加 received。原数量库存不自动回补。ITEM_ADD 新建独立商品行/数量；apply-order-changes 使用动作版本生成 order_items 快照，还处理版本化调整/运费/credit line 和 summary。后续事务写入须保留这些数量、金额、版本及引用关系，并与现有订单金额读取、配货/取消/发货、客户设备查询共同验证。

下一批写入需要同一事务重读当前权限与上述事实，验证设备身份及 SKU，持久化不可变操作收据，创建换货/退货/新订单商品/变更动作与版本，再预留替换机。写入成功响应丢失、并发重复和中途失败必须单次生效或完整回滚；同 SKU 也不能假设优惠/税/版本金额天然不变。

## 证据

[资格读取报告](device-replacement-report.json)。隔离 Gin/MySQL/Redis 通过：订单原 SKU、已履约候选、来源/仓库一致性、设备可用状态、重复替换、原生退货关联、付款/申请状态、事实版本变化、撤权/仓库身份/已删除设备/外订单、严格查询与业务设备不变。没有浏览器或换机写入完成声明。

## 订单版本事务参与者

后续换机写入复用 `orders.AdvanceVersionTx`：必须在调用方已授权的真实事务内执行，锁订单并核对当前版本，拒绝草稿/取消订单、待处理变更及已存在的后续版本。按新行 ID 复制当前商品数量、运费关联、优惠调整、credit line 和 summary，保留完整 scalar/raw 字段、可空引用和元数据；税行和商品描述本来不按版本复制，继续共享原快照。

复制后调用现有 ReadAmountSnapshot 核对每个商品/运费及总金额、交易汇总完全一致，再返回新版本；原版本不修改。调用者任意后续错误必须回滚整个事务，不能因参与者返回成功就提交不完整换机。这不是独立公开 API，没有自动赋予权限、创建换货单、新商品或库存预留，也没有替代不可变命令收据。当前换机写入仍不可用。

参与者验证见 [订单版本报告](order-version-report.json)：全部 Go 包、真实 SQL 的 AdminAuthentication / InventoryStockEdits / ProductInventory 通过；不含换机命令或浏览器完成声明。复制时 MySQL 运费优惠生成列由数据库重算，未来版本优惠也被拒绝；测试在独立人工数据中执行并完整清理。

## 换机新商品行事务参与者

`orders.CreateExchangeItemTx` 在调用方事务内从当前订单原商品行读取完整 raw 单价，以一个同 SKU、数量 1 的独立商品行承接换机。沿用原流程的自定义单价，当前促销报价不能覆盖它；当前报价只参与含税标记和促销参考原价，缺少当前报价仍可使用原自定义价。商品标题、SKU 等快照来自当前已发布且属于订单渠道的商品；缺失地区、禁用渠道、不完整配置及不可用税提供者均拒绝。

新行使用当前系统税配置，旧商品税行不改。原生 begin exchange 没有启用 carry_over_promotions，因此本参与者不盲目复制原商品优惠。所有履约、发货、退货及核销数量初始化为 0；订单数量为 1。保留任意精度 raw 单价，SQL DECIMAL 投影仅截取兼容精度；先创建商品与订单关联再回填 totals_id，遵守外键。

本参与者仍未接公开换机命令。换货单、隔离退货、订单变更动作、金额汇总、数量预留及替换设备分配须由同一授权事务完成后才能开放界面。真实数据库证据见 [新商品行报告](exchange-item-report.json)，不表示完整换机已完成。

## 换机订单汇总参与者

`orders.RefreshChangeSummaryTx` 在同一调用方事务内按当前商品、税、优惠、数量及 credit 计算订单总额，再从未删除的实际 order_transactions 重算 transaction_total、paid_total、refunded_total，重算当前版本 credit_line_total。汇总缓存不能替代流水。币种不一致或晚于当前版本的流水拒绝；金额使用既有 totals 的精确 raw 优先算法，不经过浮点或支付币种舍入。

current_order_total 与 accounting_total 继承原 order-change 的当前总额规则，pending_difference 为总额减净交易额，允许负数。保留历史版本、交易流水、original_order_total 及其他历史字段；当前汇总 scalar/raw 一起更新。该参与者不收款、不退款、不推断订单已付，也不单独提交；调用方必须将权限、换货/隔离退货、设备分配、库存预留和操作收据一起完成。

验证使用原精确单价、新税行、含精确低位的实收与负向退款流水，确认新版本汇总与金额读取一致，旧版本及私有历史不变；故障触发器拒绝汇总更新时，已复制的订单版本与新商品行全部回滚。见 [汇总报告](exchange-summary-report.json)。换机公开命令及界面仍未完成。

## 换货和隔离收货订单事务

`orders.ApplyDeviceExchangeTx` 将现有版本复制、新商品行、换货单、退货及关联明细、三个已应用变更动作和两个确认变更接入同一调用方事务。第一次确认产生 v+1，原商品 requested 增加 1；第二次隔离确认产生 v+2，requested 扣回 1、dismissed 和 written_off 增加 1、正常 received 不增加。每阶段重算当前订单汇总；旧版本保留。换货/退货单保留创建时的 order_version，新增商品明细进入两个新版本；ITEM_ADD 动作保留完整精确单价和 order_exchange 引用。

退货单明细的物理 received_quantity 和 damaged_quantity 均为 1；这与订单商品正常 return_received_quantity 为 0 是不同语义。本批也纠正已有普通隔离退货的新写入，使退货明细记录物理收货数量。没有回补数量库存，也不批量修改历史记录。Gin 原生退货与换货显示编号通过同一运行时行锁串行分配，订单锁先于编号锁。

来源继续核对安装的 Medusa 2.21.2：createOrderRelatedEntity_、confirmExchangeRequestWorkflow、beginReceiveReturnWorkflow、confirmReturnReceiveWorkflow、三个 action 实现及 applyChangesToOrder。额外确认：confirm exchange 仅在有 exchangeShippingMethod 时创建数量预留；原自定义换机未添加此配送方式。因此实际替换设备仍按原流程预留设备，打包时处理数量库存。之前后续清单中的“数量预留”不能作为无条件新增步骤。

本批仅是订单事务参与者，尚未开放 POST 或 React 换机按钮。调用方仍须验证当前操作者、已付订单、处理中换机、已收回原分配、可售同 SKU 替换设备，绑定原分配 return_id，生成替换分配、事件和不可变收据，并处理原支付集合差额更新；任意失败必须整笔回滚。真实 SQL 验证覆盖双版本、三动作、完整引用与明细、精确税价和汇总、末尾接收动作故障回滚，见 [订单换货报告](native-exchange-report.json)。

## 实际替换命令与 Ant 界面

`POST /api/admin/orders/:id/after-sales/:request_id/devices/:assignment_id/replacement` 接收且仅接收 operation_id / expected_revision / device_id 三个字符串字段。沿用私有管理员认证、CSRF/Origin 和业务权限校验；拒绝重复 JSON 字段、未知字段、空问号、非法 UUID/版本、非 JSON 和超长请求。GET 的 write_available 现在来自维修服务与分配开关及当前实际写权限，不再固定 false。默认配置没有因此自动启用写入。

命令在真实事务中重新校验已付订单、处理中换机、已收回原分配、无原生退货关联/活动替换、当前候选版本和设备身份。先完成订单换货参与者，再将原分配绑定 return_id；替换设备绑定新订单商品行、原分配、售后申请和来源，状态 reserved。设备事件与既有 device_service_receipts 的全局 UUID 收据一起提交；检查/换机请求字段不同，同 UUID 不能跨命令重放。权限和当前目标引用在重放时仍重新校验；服务写开关关闭后允许核实已有结果。

支付集合在两个确认阶段按原流程刷新：已完成付款集合不改；正差额建立 not_paid 集合，后续非负差额更新这个未付集合，负差额保留原集合和待退款差额。没有创建捕获/退款、没有标记已结清。内部遇到已授权但未捕获的活动集合拒绝处理，须由拥有支付会话取消/重建能力的流程处理；本命令首次要求所有集合已 completed，因此正常初次换机不进入该分支。数量库存不改，沿用无新换机配送方式的原自定义设备预留行为。

React Admin 从售后收回→处理设备打开 Ant 换机抽屉，显示订单原 SKU、SN/来源/仓库候选，提交后显示新商品行、退货单和待结差额。未确认结果冻结选择，重试保留原 operation_id 和完整请求；重新读取后核对真实记录。没有把保存设备分配当作发货或售后完成。

证据分开记录：真实 Gin/MySQL/Redis 验证订单/设备/收据绑定、重复恢复、撤权/删除拒绝、收据失败回滚，以及正/零/负差额；浏览器使用合成 API 验证 Ant 操作及丢响应恢复。见 [命令报告](replacement-command-report.json)。完整 React→真实 Gin→真实 MySQL 的换机/配货/发货联合、独立命令并发竞争、补款渠道/实际退款和通知仍待继续；本批不标记整个售后或前台完成。
