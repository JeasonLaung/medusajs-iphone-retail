# 仓库未装包设备预配释放

2026-10-31：仓库设备页已承接旧 `device-operations.ts` 的 reserved 释放分支。销售设备具有当前仓库配货权限且存在唯一有效未装包预配时显示“解除未装包预配”；复用原 Ant Drawer 的确认、未知响应原正文重试及成功后重新读取。已装包释放继续使用 [原包裹释放](warehouse-release.md)，不会将两种库存语义混在一起。

## 范围与事务

复用 `GET /api/admin/warehouse/locations/:location_id/devices/sale` 的 `allowed_actions` 和原 POST 设备命令；新增动作 `release_reserved`，正文仍为 operation UUID、expected_revision、action 三项，无浏览器订单/分配候选。当前仓库范围、`warehouse.intake.read`、`order.fulfillment.read` 和 `warehouse.allocation.write` 均在事务内检查。普通入库写权限不能替代配货权限，配货释放也不要求额外入库写权限。未授权账号不查询订单绑定，不展示动作；不新增角色自动授权。

设备必须未删除、sale/reserved、仍在当前仓库；唯一活动assignment必须同设备、同有效来源且来源在当前仓库，status=reserved、fulfillment为空，订单有效且不是draft/canceled。原设备和分配事实共同进入opaque revision；原尝试绑定变化会拒绝旧版本，投影不返回订单、商品行或分配内部事实。设备全局身份事务锁沿用现有命令锁，串行保护物理设备分配。

同一个事务取消该assignment、恢复该设备available、写原order/actor的release事件、推进设备revision并保存唯一操作回执。数量库存、预留、订单数量、包裹、付款均不写入。回执失败全部回滚。并发同命令只有一次改变，其余返回回执；重放前仍复核当前权限、仓库、设备、原assignment/order/item/source绑定，不能把旧回执转移到另一次销售。

无需新增SQL迁移；复用原 `warehouse_device_changes` 和设备事件表，现有设备/订单配货运行开关仍默认关闭，启动不自动启用。

## 验证与限制

[仓库专项报告](warehouse-runtime-report.json) 的 `warehouse_reserved_release` 记录真实 MySQL/Gin HTTP、权限分离、拒绝packed及失效订单、分配变化使revision失效、末尾回执故障完整回滚、4路并发单胜者、单一release事件、数量/预留等全表快照不变，以及改变绑定/移仓/撤权后的回执拒绝。专项同时运行既有库存与商品库存回归，Go全量、相关vet和Admin类型检查通过。

[真实浏览器联合报告](warehouse-devices-joint-report.json) 验证新建仓库员工→设备页→英文桌面/中文手机释放→刷新仍可售，首次真实成功响应丢失后保持UUID/版本/正文重试；第三台设备仍reserved。撤销配货权限后会话失效，重新登录后没有释放动作。数据库复核两条取消分配、两条唯一事件、原数量库存不变。浏览器全部业务事实来自真实Gin/MySQL/Redis，响应丢失仅为传输故障注入。

本次不宣称完整新付款订单→配货→发货→客户物流/设备→通知连续联合已完成；该链条和原交付清单其余缺口继续推进。未操作旧开发库或真实设备/订单。
