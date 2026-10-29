# 订单行设备预配货及解除

React Admin 订单详情的每个订单行增加「设备配货」抽屉，使用Ant表格、选择、搜索及反馈组件。读取当前订单行容量和同SKU可售设备，整批预配；尚未装包的预配货可整批解除，保存后重新读取。已有订单、支付与履约证据入口保持原行为，前台未改版。

本批承接原 `workflows/device-operations.ts` 的 assign / reserve_only 与未装包 release 分支，不替代 save_allocation 的装包、履约迁仓、发货、取消包裹或售后换机。原仓库员工不允许使用全店 order_assign；其指定履约范围的分配入口仍待后续实现。

## 条件与事务

- 订单须未取消、非草稿，使用当前 version 的 order_items 与真实成交行。订单号、行、SKU、设备、来源和仓库从数据库验证，不采用浏览器提供的业务状态。
- 原 `lib/order-device-payment.ts` 要求至少一个支付集合且每个集合 completed；只有 authorized 或部分完成不能预配。解除未装包预配无需订单仍已付款，但订单必须有效。
- 原 `lib/order-device-capacity.ts` 同时考虑订单数量、已履约量和活跃设备分配。本入口只预配尚未履约的剩余量，不向已有包裹补配或把已履约数量再次预配。
- 设备须同SKU、sale/available、没有reserved/packed/shipped活跃分配，来源与设备位置一致，仓库关联订单销售渠道。规范SN/IMEI/EID按当前商品要求重查；拒绝以配货为名创建设备或补写身份。
- 配货整批创建reserved分配、设备reserved及assign事件；解除只允许reserved且无fulfillment_id的分配，原设备须仍reserved，变为canceled分配和available设备并留release事件。packed/shipped在此入口拒绝，避免绕过整包取消和库存恢复。
- 数量库存及订单预留均不再次扣减或释放，保持原来设备档案与数量库存分离的规则。

新增显式 order.allocation.write，既有角色不自动授予。接口还需 order.read、order.payment.read、device.read；事务中先锁当前账号/授权，再锁设备全局运行时及订单，之后处理当前行、支付集合与设备。未来相关命令须遵守同一锁顺序。warehouse/supplier不能调用全店入口。

0065新增默认关闭的配货能力和独立不可变收据，已提交迁移共64。设备identity/commands开关仍需显式启用；本批没有为真实数据库自动开写。新账号bootstrap明确列出25个动作，不迁移旧登录授权。

## 接口与重试

GET/POST `/api/admin/orders/:id/items/:item_id/allocation`。GET返回当前支付完成状态、容量、可选设备及配货记录；POST严格接收 operation_id、expected_revision、action（reserve/release）和ids。reserve的ids为设备ID，release的ids为分配ID；每批1—200且不能重复。保留既有Origin/CSRF保护。

摘要包含订单/行、支付集合、分配、候选设备及全局设备修订；冲突要求重新读取。收据绑定账号、订单、成交行及完整请求，当前权限和订单行以及记录目标存在性在重放前重查；重复原请求返回历史操作结果，不再次分配。设备、分配、身份索引、事件和收据在同事务提交，末尾失败整体回滚。界面在未知结果时固定原批次及UUID，提供同操作重试。

## 验证与缺口

隔离SQL覆盖默认关闭、独立动作、授权款拒绝、多支付集合必须全部完成、过期付款摘要、渠道候选裁剪、同操作四并发唯一配货、满额拒绝、解除/恢复容量、不同请求不能复用收据、末尾失败回滚、数量库存/预留指纹不变、权限撤销和已删目标拒绝。Go全包、后台类型/构建及24项浏览器回归通过；新增浏览器用例验证丢响应后固定原批次及当前权限。

真实React/Gin/MySQL/Redis联合验证已付款订单行选择两台→预配→解除一台→刷新持久化，中英/390px与无脚本异常；回查SQL确认分配、设备状态、事件及收据数量，数量库存和预留没有被改变。证据为 order-allocation-report.json、order-allocation-joint-report.json。

本批尚不覆盖仓库员工指定履约分配、已有包裹补配、装包/配送/发货、换机、客户实际物流查询或完整购物闭环。C07保持未验收，后续需将本批预配接到真实履约事务。
