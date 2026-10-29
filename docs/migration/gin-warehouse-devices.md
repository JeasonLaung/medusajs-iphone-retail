# 仓库设备分类列表与生命周期操作

本批承接原 `warehouse_intake` 与 `warehouse_return` 的分类读/写，以及设备 `activate`、`make_available`、`retire`、`delete`。不将设备总模块、仓库入库或配货履约宣布完成；整个现有前台闭环的范围不变。

新 React 菜单“仓库设备”使用 Ant Radio、Table、Input.Search、Image.PreviewGroup、Drawer 和反馈组件。只选择当前获授权的仓库和销售/退回类别；回收不在仓库员工可操作类别中。列表每页50，按完整SN/IMEI/IMEI2/EID搜索，保留前导零。展示设备标识、SKU名称、状态、库位、照片；不返回成本、备注、客户、订单、分配记录、来源名称或其他仓库的数据。

`GET /api/admin/warehouse/locations/:location_id/devices/:category` 只允许 offset 与 q，各最多一次；offset为0—100000的规范整数，搜索最多100字符。响应返回当前页、has_more和设备摘要。完整范围在SQL中先过滤再分页，不在浏览器裁剪全店列表。

0060迁移增加四个明确动作：warehouse.intake.read、warehouse.intake.write、warehouse.return.read、warehouse.return.write。销售设备读取/命令分别要求前两者；退回设备使用后两者。仓库账号管理沿用当前授权校验及会话撤销，允许独立勾选这四项；旧角色不自动增加权限。已有 inventory.read 等不能推导出设备权限。全店端点继续拒绝warehouse主体。

每次读取和命令均在事务内调用当前仓库授权检查；需要真实warehouse主体、有效账号、当前动作、开启范围门控、selected/all范围及未删除仓库。全店员工或供应商不能利用此入口。未选仓库等于没有范围。

`POST /api/admin/warehouse/locations/:location_id/devices/:category/:device_id` 要求显式 operation_id、expected_revision、action，拒绝未知/重复键、null、附加JSON及查询参数，继承Origin、CSRF、登录与私有响应。UUIDv4操作收据绑定账号、路径仓库、类别、设备和完整请求。授权及设备当前仓库/类别先于重放检查；软删除重放可恢复原结果，迁出仓库、撤权、删除仓库后不能重放。

命令持有统一设备身份锁，先比较读取摘要，再检查状态和销售历史。设备状态、事件、全局修订和收据同事务写入，末尾失败全部回滚。摘要包含当前设备字段及全局修订；另一命令后旧编辑请求返回409。客户端不确定时保留原请求重试，409/403等确定失败要求关闭并重读。

沿用源码的具体规则：

- make_available：sale、self来源、pending，且无 reserved/packed/shipped/returned 历史分配，进入 available。
- activate：sale、self来源、inspection，同样拒绝上述历史分配，进入 available。
- 激活两类动作都检查真实商品号码要求、已存身份、SKU配置；不补写默认成色/电池，不跳过跨字段唯一身份占用。
- retire/delete：reserved、packed、shipped、repair_ready状态拒绝；任何上述销售历史拒绝。retire进入retired；delete软删除，保留设备、事件及销售历史。
- 所有这些动作不改变数量库存、预留、订单或支付。照片仍遵循公开设备照片语义，不用于私有回收文件。

列表的 allowed_actions 按当前写权限、状态、来源和销售历史给出界面可操作项，实际POST再做全部校验；可见按钮不构成授权。新界面当前一次处理一台，原批量make_available、设备编辑、转退回、检测、指定履约的配货/解除/发货仍需继续接入。

隔离验证：59项迁移，Go全包、Admin类型检查/构建及原19项后台浏览器回归。SQL/HTTP验证选仓/类别、回收拒绝、只读按钮、50+分页、完整号码搜索、无成本备注投影、4并发一次修改、不可变重放、对象迁仓后拒绝重放、撤权、过期摘要、末尾收据回滚、软删重放、销售历史与锁定状态、号码/成色/电池核对、删除仓库。父入库用例对数量库存完整指纹保持不变。

真实浏览器联合：`python retail/scripts/test-warehouse-inventory-joint.py --warehouse-devices`。管理员通过UI创建新仓库账号和四项分类权限，员工登录后执行pending可售、inspection激活、退役、退回设备删除并刷新；直接跨仓及全店商品请求拒绝。管理员移除销售写权限后旧会话撤销，重新登录只读；中英和390宽度验证。回查持久MySQL状态、四份事件/收据及精确数量库存不变，完成归属容器/网络清理后才写报告。

合成API浏览器另验丢响应同请求重试、409不能盲重试、只读/分类控件、越仓响应拒绝、分页、空范围及加载恢复，不作为真实Gin联合证据。

证据：warehouse-devices-report.json、warehouse-devices-joint-report.json。测试只使用独立新建数据，不操作本机业务数据或部署服务器。

后续已补 [仓库员工带照片入库](gin-warehouse-intake.md)，使用独立范围端点和共用事务；本文件的原验证证据保留其当时范围。


## 销售设备转退回（2026-10-30）

仓库设备页新增原有 `transfer_return` 操作，使用 Ant 表单填写必需的转退回原因（规范去首尾空白，最多2000字符）。操作后同一设备进入 returned / inspection，保留ID、SN/IMEI/EID、SKU、来源、位置、原备注和照片；原因记录到设备事件，不覆盖设备备注。

执行要求当前销售读/写权限及退回写权限；退回读取仍独立控制。设备须位于当前授权仓库，销售类别、pending/inspection/available状态，无 reserved/packed/shipped 活跃分配，并且当前位置存在有效自有仓来源。历史returned分配允许转退回；供应商来源设备只要当前位于自有仓，也沿用原规则允许。不能用设备原来源代替当前位置判断。

继续使用统一设备事务锁、摘要和UUID操作收据。事务末尾失败时类别/状态/事件/修订均回滚。成功后从销售列表移至退回待检测列表；原销售路径的相同请求可以重放原收据，另起操作不能再次转退回。重放也需当前权限、当前仓库和完全相同的请求。数量库存、预留和支付不在该设备命令中修改。

验证包括严格JSON及2000字中文原因边界、权限撤销、当前仓库/自有来源、三种活跃分配、历史退回许可、供应商来源设备在自有仓转退回、末尾故障回滚、审计内容、跨类别收据恢复和改变请求拒绝。真实浏览器联合通过管理员建仓库员工→员工填写原因转退回→退回页显示待检测→刷新保持结果，并回查持久SQL类别/状态和事件/收据及库存不变。完整检测、编辑、配货发货及前台售后/回收仍待后续，不能将本次转退回当成这些流程已完成。
