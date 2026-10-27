# 仓库范围、响应与逐对象授权

日期：2026-10-27。T004/T005/T007 的源行为证据与目标要求，不代表 Gin 已实现或完整权限已验收。

## 已执行范围

`retail/scripts/capture-warehouse-access.cjs` 在受限模块加载下直接执行源 `lib/warehouse-access.ts`，以及 `api/warehouse-guards.ts` 的 `authorizeWarehouseCommand`。仅用人工员工、设备、订单、包裹和金额；查询、服务和 hasPermission 是明确替身，不启动应用、不访问数据库、不调用业务写入。两个源文件哈希及72项断言记录于 `fixtures/legacy-warehouse-access.json`，`--check` 同时校验行为、用例及源码漂移。

```powershell
node retail/scripts/capture-warehouse-access.cjs --check
```

测试没有执行原生RBAC引擎、HTTP中间件、设备工作流或真实事务。不能把允许通过仓库guard理解为订单状态、付款、库存和其他角色权限也已经通过。

## 员工身份和范围

实际旧函数识别固定仓库角色、metadata.device_role=warehouse，或 warehouse_location_ids 数组。全仓须在仓库身份下显式 warehouse_scope=all；仅有 all 字段不会成为仓库。空数组是空范围；重复位置去重、非字符串和空字符串去掉，但空白字符串没有 trim。新绑定接口必须拒绝空白/不存在仓库，不能将空范围转为全仓。

源 warehouseAccess 找不到员工会返回非仓库且 location_ids=null；authorizeWarehouseCommand 遇非仓库直接返回。这是需由外层完成身份及其他动作授权的分支，不是“未知身份有管理员权限”。Gin 身份不存在、停用或权限查询失败均拒绝，集成凭证也不自动获得全仓或超级管理员权限。

仓库来源须位于范围内且 type=self；即使同仓的 supplier 来源也拒绝。设备 recycled 类别拒绝；returned 使用 warehouse_return，其余旧类别使用 warehouse_intake。目标只接受已声明枚举，不能以旧默认分支放过未知类别。

## 实际逐动作边界

| 动作 | 已运行的允许/拒绝样例及承接要求 |
| --- | --- |
| intake | 每台检查类别动作权限及来源；sale/returned 分别查 intake/return；supplier 来源与 recycled 拒绝 |
| activate/delete/retire/inspect | 逐设备查当前仓库和类别权限；跨仓拒绝；状态条件仍由工作流负责 |
| make_available | 数组第二项跨仓也拒绝，不能只验证首项 |
| update | 当前设备类别与目标类别均需权限；目标位置及来源重新检查 |
| transfer_return | 当前类别权限与 warehouse_return.update 同时需要 |
| assign/order_assign | 仓库拒绝这两个通用分配入口，使用按包裹的 save_allocation |
| save_allocation | 订单须有本仓有效包裹；每项必须带包裹、包裹属于该订单、设备同仓、item属于包裹；第二项跨仓也拒绝 |
| release | assignment关联的设备、包裹及订单均检查；包裹不属于该订单时拒绝 |
| receive | assignment边界及目标自营来源检查；退回状态与数量另由工作流负责 |
| ship | 先查 allocation.update，再查包裹仓库；无动作权限时未读取设备服务 |

未知仓库动作拒绝。assignment已存在时源guard还会检查原assignment的范围；新命令必须同时核对其与本次订单、行、包裹及设备的精确关联，不能仅靠它们各自“都在某个允许仓库”通过。旧guard通过不证明这些交叉关联都已核对。

## 查询和金额响应

订单候选查询使用范围内未取消包裹的 order_fulfillment 链接；空范围使用不匹配仓库条件，全部包裹已取消时不继续查订单链接。包裹详情同时检查位置和路径订单关联，跨位置先拒绝。

局部范围的旧响应只保留本仓包裹，订单行数量为本仓未取消包裹数量之和。相同行多包裹数量累计；其他仓行移除。嵌套 variant 库存按位置裁剪，再按库存项 required_quantity 取可用量的最小值；没有inventory_items返回未定义，不伪造0。全仓分支直接返回原对象；全仓范围仍不是付款、退款或配置动作授权。

实际人工输出发现以下边界，不能直接复制为 Gin DTO：

- 裸字段 `subtotal` 不符合 `_subtotal` 后缀条件，因此行和订单小计原样保留。人工订单全单subtotal=140，本仓total=60时，返回subtotal仍为140。新 DTO 必须显式计算并裁剪每个金额字段，不以字段名后缀猜测。
- 其他匹配金额字段用 Number 按数量比例缩放；0.1×2÷3留下浮点尾数。Gin 使用十进制精确计算，明确币种精度、数量口径及尾差分配；不得返回另一仓金额。
- 本仓已取消包裹仍留在响应数组，但不参与数量。新响应可保留本仓取消历史且明确状态，不得用于计算当前配货/可售。
- 源对象展开会保留未知订单字段、行metadata、库存对象内部字段。新系统使用角色专用allowlist DTO；禁止把raw payload、metadata或以后新增内部字段自动透传。
- 已知付款集合仅投影status；移除全单退换/credit/promotions/配送/summary，客户只留业务联系字段。这些裁剪不能代替新客户归属与字段授权。空范围投影仍能带客户字段，因此必须先判断订单可见性，再组装响应。

## 剩余工作

逐路由挂载和响应包装、原生RBAC、供应商逐动作、所有批量对象交叉关联、导出/附件以及故障拒绝仍需验证。新 Gin 的十进制金额、专用DTO、SQL范围/锁、并发撤权和HTTP集成尚未实现；Ant Design 表格和抽屉必须消费同一范围响应，不能由前端隐藏字段代替授权。T004/T005/T007保持进行中。
