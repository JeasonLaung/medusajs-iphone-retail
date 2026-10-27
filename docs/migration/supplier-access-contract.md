# 供应商入口与逐对象授权

日期：2026-10-27。补充T004/T005/T007，不替代[供应商账号及绑定](supplier-account-contract.md)。旧个人授权与账号不导入，以下规则用于新账号显式重建权限。

## 实际执行证据

`retail/scripts/capture-supplier-access.cjs` 直接执行 `api/device-access.ts` 的 deviceAccess、guardSupplierAdmin、authorizeDeviceCommand。83项人工端口样例，哈希及结果见 `fixtures/legacy-supplier-access.json`。不启动Medusa、不访问业务数据库、不修改设备/订单/付款；服务端查询为明确替身。

```powershell
node retail/scripts/capture-supplier-access.cjs --check
```

--check核对源码哈希和固定用例结果。实际HTTP鉴权、原生RBAC、查询实现及完整工作流/事务未运行；供应商scope通过不表示业务状态也通过。

## 身份和入口

源身份必须是员工metadata.device_role=supplier且有字符串device_source_id；来源查询强制type=supplier且必须存在。无来源、来源消失、账号及secret key都不存在时拒绝。查询故障向上传递，不变成无范围管理员。

旧逻辑将普通员工及找不到员工但存在的secret API key标记admin作用域，这不是动作授权证据。旧middleware对无actor及actor_type=api-key直接next；Gin必须先验证具体身份、停用状态、动作及范围，旧密钥不导入，新集成凭证不自动获得管理员权限。

供应商的通用订单、客户、商品、RBAC角色、邮件及设备照片入口拒绝；只允许设备工作台、设备variants、warehouse-access/me及必要后台bootstrap分支。GET /admin/devices/?limit=20会规范掉查询和单个尾斜线后匹配；POST /admin/devices仅允许进入后续命令授权，不能据此执行任意action。

员工仅可修改自己的first_name/last_name。夹带metadata或修改他人即拒绝。旧me响应只投影员工基本资料且不含密码，但保留metadata；新bootstrap须使用明确DTO及新绑定，禁止复用内部metadata模型。权限bootstrap为空，通知为空；店铺bootstrap仅id/name，不给配置秘密。这些响应便于受限后台启动，不赋予通用订单或客户权限。

## 命令级范围

| 动作 | 旧guard实际检查与新承接 |
| --- | --- |
| supply | 请求source必须为绑定source；供货数量与幂等在领域命令验证 |
| supplier_assign | device.source与订单必须属于授权，replacement拒绝；仅scope通过。实际工作流仍拒绝DEVICE_INTAKE_REQUIRED，不能实现成供应商登记新设备捷径 |
| assign | 已有设备source及location必须匹配绑定来源，订单须授权，replacement拒绝 |
| release | assignment的source及order都必须授权 |
| fulfill | 查询ID列表，返回数量必须相等且非空；每条source/order都验证；重复请求ID在常规唯一查询结果下数量不符而拒绝 |
| ship/cancel_fulfillment | 按包裹查询全部assignments，非空且每条source/order都验证；混合来源包裹整批拒绝 |
| 其他 | source/enroll/intake/update/activate/delete/retire/make_available/transfer_return/save_allocation/order_assign/receive/inspect/repair_ship及未知动作均拒绝 |

人工测试将跨范围记录放在第二项，确认整批成员都检查。fulfill旧guard依赖服务查询的ID过滤，单独只比较数量并不再次核对每个返回ID；人工替身返回等数量不同ID时guard仍通过。这是分层证据，不能据此宣称真实查询会忽略过滤。Gin命令使用实际锁定的请求ID集合，并校验完整集合及对象关联。

若两条assignment分别来自两个都已授权的订单，scope guard会通过；这不允许合并成一个履约。源码 `workflows/device-operations.ts` 的fulfill分支另有去重、reserved状态、同order/source及配送方式条件。后续完整工作流验证必须保留这些规则，不能只迁guard；本轮没有执行该工作流。

## 查询和字段承接

源码查读 `api/admin/devices/route.ts`：供应商order_id先在授权列表内，device_id先按source_id查询；assignment同时按source及授权order过滤。订单专用查询包含收件地址、行与包裹、付款集合status，不查询付款金额及退款明细；通用/admin/orders入口仍拒绝。设备工作台GET的完整查询及响应裁剪尚未执行，不能把这段源码查读当成测试通过。

新React/Ant Design工作台继续提供现有供货、已入库设备配货、发货及解配流程。Gin必须对来源、订单和包裹逐次查当前授权，使用专用DTO；有订单授权不代表能读所有客户资料或修改订单。来源、仓库、库存、设备成本及内部事件的精确字段白名单仍须对照实际页面消费固定，不能直接返回数据库模型。

新账号不因邮箱相同认领历史订单，旧device_order_ids等个人授权不导入。新绑定去重并拒绝空ID/空白ID/不存在对象；旧身份函数只过滤非字符串，重复和空字符串会留下，不能原样复制为新输入规则。

剩余：完整供应商查询/字段DTO，原生RBAC及API key新最小权限，多进程撤权、SQL对象锁、设备工作流状态/失败恢复和HTTP集成。83项样例不代表该领域已迁移，T004/T005/T007继续进行。
