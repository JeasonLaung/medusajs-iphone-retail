# 设备命令、权限及事务基线

日期：2026-10-27。覆盖 T001/T004/T005 中设备命令的源码核对；尚未运行迁移实现或改变旧业务数据。

## 证据与适用边界

下列路径以 `medusa-store/apps/backend/src/` 为根：

- 请求及验证：`api/admin/devices/route.ts`、`api/admin/devices/validators.ts`。
- 实际命令处理：`workflows/device-operations.ts`、`workflows/device-source.ts`、`workflows/device-refund.ts`。
- 供应商授权：`api/device-access.ts`；仓库授权：`api/warehouse-guards.ts`、`lib/warehouse-access.ts`。
- 身份与配置：`modules/devices/{identity,policy}.ts`、`lib/device-identity.ts`。
- 支付及数量：`lib/order-device-payment.ts`、`lib/order-item-quantities.ts`、`lib/order-device-capacity.ts`。
- 移动入库：`api/mobile-intake`、`workflows/mobile-intake.ts`、`lib/mobile-intake-token.ts`。

旧入口 `/admin/devices` 的 POST 先调用仓库、供应商授权；除 `source` 走独立来源工作流，其余进入 deviceOperationWorkflow。工作流内也校验 actor 和授权，因此不能在 Go 中仅保护 HTTP 路由而放开内部命令。

下表“管理员”指通过相应身份及动作授权的非供应商、非受限仓库账号，不等于任意已登录用户。原生 RBAC 与 API key 权限仍需独立审查。新账号不沿用旧账号授权数据。

## 动作级授权

所有仓库操作必须先校验对应动作权限，再校验对象所在仓库；默认拒绝未列出的动作。供应商默认拒绝，除以下明确分支。

| action | 供应商 | 受限仓库员工 | 领域任务 |
| --- | --- | --- | --- |
| `source` | 拒绝 | 拒绝 | T042 |
| `enroll` | 拒绝 | 拒绝 | T031、T040 |
| `intake` | 拒绝 | 对每台设备校验类别 update 和来源可访问 | T041 |
| `update` | 拒绝 | 原设备及目标位置、来源可访问；原/新类别均有 update | T040、T041 |
| `activate` | 拒绝 | 自己仓库设备 + 对应类别 update | T041 |
| `make_available` | 拒绝 | 逐台自己仓库设备 + 对应类别 update | T041 |
| `delete` | 拒绝 | 自己仓库设备 + 对应类别 update | T040、T041 |
| `retire` | 拒绝 | 自己仓库设备 + 对应类别 update | T041 |
| `transfer_return` | 拒绝 | 原设备类别 update + warehouse_return update，设备属于可访问仓库 | T045 |
| `supply` | 仅绑定来源 | 拒绝 | T042 |
| `order_assign` | 拒绝 | 拒绝；须改走指定履约的分配入口 | T043 |
| `assign` | 自己来源、设备仍位于该来源仓库、授权订单；禁止 replacement_of | 拒绝 | T043、T046 |
| `supplier_assign` | 授权层校验来源、订单并禁止 replacement_of；但业务层当前总是拒绝 | 拒绝 | T043 |
| `save_allocation` | 拒绝 | warehouse_allocation update；订单及每个指定履约可访问；设备与履约同仓、行项目匹配；原分配也在范围内 | T043、T044 |
| `create_fulfillment` | 拒绝 | 当前默认拒绝；不能由“有订单权限”推导可创建 | T037、T044 |
| `release` | 原分配的来源和订单都获授权 | warehouse_allocation update；设备及分配关联的订单/履约可访问 | T043、T044 |
| `fulfill` | 全部分配存在，且来源/订单均获授权 | 当前默认拒绝 | T037、T044 |
| `ship` | 履约至少有一条分配，全部属于授权来源/订单 | warehouse_allocation update + 指定履约可访问 | T037、T044 |
| `cancel_fulfillment` | 同上 | 当前默认拒绝 | T037、T044 |
| `receive` | 拒绝 | warehouse_return update；原分配/设备及目标来源在范围内 | T045 |
| `inspect` | 拒绝 | warehouse_return update + 原设备类别 update，设备在范围内；回收设备仍受类别限制 | T041、T045 |
| `repair_ship` | 拒绝 | 当前默认拒绝 | T046 |

仓库的“读取共享商品定义”并不授权任意关联查询。当前 guard 会限制 fields 遍历、裁剪库存 levels、裁剪订单行及履约、隐藏其他仓库的售后行，并拒绝不相关 API。供应商获得的是受限 bootstrap 响应与设备接口，不能顺带请求原生客户/订单明细。T015 必须覆盖直接请求、列表、导出及附件。

## 状态与副作用

| action | 关键前提及状态结果 | 数量库存 / 原生交易影响 |
| --- | --- | --- |
| `source` | 名称不重复；type self/supplier；店铺必须有默认销售渠道；创建来源及关联仓库 | 创建 stock_location，并关联默认渠道；不是仅新增来源名称 |
| `enroll` | SKU 未接管、开启库存、关闭超卖、恰好一个每台消耗 1 件的库存项；库存项不与别的 SKU 共用 | 创建设备追踪配置及事件，不凭设备数重置数量库存 |
| `intake` | 校验整批身份、SKU 成色/电池、已存文件；普通入口只允许 self 来源；sale → available，returned/recycled → inspection | 当前只创建设备档案/事件，不增加原生库存量；整批最多 200 台，任一错误全部失败 |
| `update` | 序列号及审计 ID 不可编辑；活跃分配时不能改 SKU/来源/位置/状态/类别；不能手工进入 reserved/packed/shipped/repair_ready；pending/available 只能 sale | 修改档案并留 before/after；不按档案差值同步数量库存 |
| `activate` | self、sale、inspection；无 reserved/packed/shipped/returned 历史分配；身份和配置有效 → available | 只改设备状态 |
| `make_available` | self、sale、pending；无上述历史分配；ID 不重复，全部通过 → available | 只改设备状态 |
| `delete` / `retire` | reserved/packed/shipped/repair_ready 拒绝；存在 reserved/packed/shipped/returned 分配拒绝 | delete 软删、retire 改 retired，留事件；不直接删销售历史、不调整数量库存 |
| `transfer_return` | sale + pending/inspection/available，无活跃分配，当前位置存在 self 来源 → returned/inspection | 保留设备 ID 与来源；只转类别/状态并记录原因，不生成退货退款 |
| `supply` | supplier 来源、合法库存 SKU；quantity 是设置值，不是增加值 | 更新供量档案及 previous/quantity 事件；当前不修改原生数量库存 |
| `order_assign` | 所有输入号码应指向同一已登记设备；序列号和已提供 IMEI/EID 必须与记录相同；未知设备拒绝 | 规范化为 assign；不能借配货新建设备或补写号码 |
| `supplier_assign` | 验证器仍接受，权限仍有分支，但 prepareDevices 明确返回 DEVICE_INTAKE_REQUIRED | 属于保留但禁用的旧命令，迁移不能因名称恢复“登记并配货” |
| `assign` | 订单有效且所有 payment_collections completed；订单行/SKU 匹配；设备 sale/available 且无活跃分配；仓库关联订单渠道；数量未配足 | 一般生成 reserved 分配/设备；可绑定已有未发履约变 packed；替换设备走原生 exchange，具体见后文；普通预分配不额外扣数量库存 |
| `save_allocation` | 批量设备和 assignment_id 均不可重复；已发/送达/取消履约不能修改；重复选择同一 reserved 设备保留原分配 | reserve_only 保持 reserved；否则 pending 分配同来源同仓时创建履约并转 packed；可在限制下移动既有 manual 且未出标签的整包履约，需迁移库存扣减位置 |
| `create_fulfillment` | 规范化到 save_allocation，继承完整请求的履约仓库、配送、行数量；可先创建无设备号码的履约；仅选部分设备也不能减少实际履约数量 | 按完整履约数量处理预留/扣减；留存其余订单数量的预留；发货前仍需配齐设备，不能把“无号码可创建履约”误读为“无号码可发货” |
| `release` | 分配 reserved/packed，设备状态同步；packed 时整包都必须未发且同来源订单 | 取消目标分配、设备 available；若 packed，取消整个原生履约恢复库存，其他设备/分配退 reserved，而非释放整个包裹的所有配货 |
| `fulfill` | 所选 reserved 分配均存在、同订单同来源；来源位置、待发量、配送方式、库存及预留有效 | 创建原生履约，设备/分配 packed；预留可能迁仓/拆分，剩余预留保留；一次不能混不同来源 |
| `cancel_fulfillment` | 订单与履约有效，未发/未送达/未取消；关联分配全 packed | 原生取消恢复该包数量库存；所有设备/分配退 reserved，清 fulfillment_id；不是 available |
| `ship` | 全部分配 packed、设备 packed、位置与履约一致；设备追踪 SKU 的行配足；提交的原生发货行与包裹完整数量一致 | 设备/分配 shipped，创建原生 shipment/label；不能对同履约任意拆行发货或再次扣同一库存 |
| `receive` | 原设备/分配均 shipped；扫描原号码；售后 processing、原订单行/设备匹配、未超申请数量 | 分配 returned，设备类别 returned；维修 → repair，其他 → inspection，移至接收来源仓库；退货类型走原生 non-restocking return，不直接加可售数量 |
| `inspect` | 类别 returned/recycled，状态 inspection/repair；检验照片、成色、电池、目标 SKU；pass 时配置匹配 | 普通 pass → sale/available；原机维修 pass → repair_ready 且 SKU 不变；repair/retire/supplier_return 各去相应状态。当前代码不自动增加数量库存，旧注释“adds stock once”不能作为实现证据 |
| `repair_ship` | 分配 returned、设备 repair_ready、repair 申请 processing，无其他活跃分配 | 原分配及设备回 shipped，写追踪号与事件；不创建新销售/新订单行/普通库存扣减 |

`activate` 的 inspection 分支与 `inspect` 的类别分支不同，不应合并为一个无条件“上架”。`save_allocation` 的跨仓整包移动限制也不能套给仓库员工：仓库授权先要求目标设备与指定履约同仓。

## 同 SKU 换机与退款

换机配货必须指定本订单行已 returned 的原分配，原分配有关联 processing 的 exchange 申请；不能重复为同一原机创建有效替换分配，也不能挪用另一个原生退货流程。原机原 SKU/成交价进入原生 exchange，新产生的订单行与 replacement_of、原 return_id 关系必须回写，不能只把新设备挂到旧行忽略订单变更。

设备退款入口 `/admin/devices/refunds` 独立于上述 action：请求必须 confirmed，return 申请 processing，要求收回的设备全部有 returned 分配。付款记录必须属于本订单；金额为主要货币单位，不能超过捕获金额减已退。原实现用 `after-sales:{request_id}` note 查重；Go 要保留结果语义，并用持久化操作记录处理“外部成功但本地尚未记录”的恢复，不只依赖查 note 或 Redis 锁。

没有序列号设备的退款申请原实现明确提示使用原生订单退款；T047 必须承接这条原生路径，不能让非设备订单失去退款能力。

## 库存事实修正

`lib/device-stock.ts` 的 max(device_count, supply_quantity) 计算仍存在，但当前查到的非测试调用方是诊断脚本；`workflows/reconcile-device-stock.ts` 明确返回空 updates。设备库存接管不意味着每次设备档案变化都改写数量库存。当前工作流的显式库存 adjustment 主要出现在跨仓移动未发履约；一般扣减/恢复由原生履约和取消负责。

因此迁移应以原生 inventory/reservation/fulfillment 的实际数量为事实，再保留设备追踪和供量记录；不得把诊断 cap 强行写回库存。Go 的幂等重试要防止原生语义被重放两次。库存迁移用例须分别断言原生数量、设备状态、分配状态、预留和履约数量，不能只断言 available 设备数。

## 移动链接与身份约束

移动入库从签名 token 读取 actor/source/category，不信任客户端另传来源；调用同一设备命令，保留 mobile_intake_nonce。移动删除还要求同 actor 的 intake 事件含同 nonce、来源/类别相同、状态 pending/inspection/available，且满足普通删除的无销售记录限制；不是持 token 就能删整仓设备。

SN 为必填文本并规范化；IMEI1/IMEI2 为 15 位、EID 为 32 位文本。产品 metadata.requires_imei 优先，其次 device_type，最后商品标题/handle/type 识别 iPhone；要求至少一个无线标识。同机重复无线标识、已有记录任意 IMEI/EID 跨列冲突均拒绝。照片既检查 URL 形式，也检查文件记录和真实 URL 对应，不能只相信客户端 photo.id/url。

## 待转写的固定验收用例

以下不是已运行测试；T041—T047 实现时转为隔离数据库集成测试和必要 UI 用例。现有 `src/__tests__/device-stock-allocation.unit.spec.ts`、`device-stock-reconciliation.unit.spec.ts` 等是额外证据，不能仅复制 mock 代替事务验证。

| 用例 | 输入/并发与预期 |
| --- | --- |
| D01 | 两人同时配同一 available 设备，仅一人成功；唯一活跃分配约束生效，失败方无残留事件/预留 |
| D02 | 未付款、仅授权、部分捕获、一个支付集合未完成均拒绝配货；全部 completed 才进入后续验证 |
| D03 | order_assign 未知 SN 或冲突 IMEI 拒绝，不能新建设备；supplier_assign 始终按当前禁用规则拒绝 |
| D04 | 两台入库第二台跨列号码重复，整批无新增设备；单台内部 IMEI1=IMEI2 也拒绝 |
| D05 | sale self 入库设备 available，原生数量库存不因登记增加；returned/recycled 入库 inspection |
| D06 | supplier supply 设置值重复提交不累加；数量库存不被设备数或 supply 公式覆盖 |
| D07 | reserve_only 只变 reserved；创建数量 2 的履约仅配 1 台时仍按 2 扣减/处理预留，未配齐拒绝发货 |
| D08 | 未知来源/渠道、SKU不匹配、超订单行数量、另一仓设备均按相应范围和状态拒绝 |
| D09 | packed 包含 A/B，解除 A：原履约取消并恢复库存；A available/canceled，B reserved/reserved |
| D10 | 取消未发履约：全部设备 reserved，库存恢复一次；重复取消拒绝/返回既有结果，不重复恢复 |
| D11 | manual 未打标签包裹可按规则迁仓，库存原仓恢复/目标仓扣减；已预订 provider 或有标签拒绝 |
| D12 | 仓库员工不能通过 assign/order_assign 绕过 fulfillment_id；同订单其他仓行、库存、成本不可见 |
| D13 | 供应商只能处理绑定来源和授权订单；更换来源、越权订单、replacement_of 均拒绝 |
| D14 | receive 扫错号码、申请非 processing、超已申请数量、重复收回均拒绝；成功不直接加可售数量 |
| D15 | inspect 普通 pass 改 sale/available，但不偷偷改变原生数量；维修 pass 改 repair_ready 并要求原 SKU |
| D16 | repair_ship 复用原销售分配，回 shipped；不创建新订单行、不扣普通销售库存 |
| D17 | 同 SKU 换机保留原成交价，新订单行/原机/替换机/return 关联一致；同原机并发换两台仅一个成功 |
| D18 | 退款前未收齐、付款不属该订单、超过实收余额均拒绝；超时后重试只执行一次外部退款 |
| D19 | 移动 token 篡改/过期、另一 nonce 创建的设备删除、无权限 actor 均拒绝；会话来源不能被 body 覆盖 |
| D20 | 数据库或外部交易阶段失败时，设备、分配、数量库存、预留、履约及审计不存在半完成；可通过持久化操作状态恢复 |

## 仍需追踪的项目

已新增 [原生入口保护基线](native-guard-contract.md)：实际guard的25条挂载、46个隔离样例及4条路径/方法缺口。特别保留数量库存guard为no-op、取消订单无活跃分配可继续、退回按任何分配记录阻止，以及专用发货/取消需要处理器承担完整检查。它不替代以下原生处理器与事务追踪。

- 管理员/API key 的原生 RBAC 动作权限、读取接口的全部字段脱敏、列表过滤和导出路径。
- 原生数量库存的手工管理入口与设备追踪迁移后的对账展示，区分“档案可售”和“数量可售”。
- 订单取消、批量改配及原生订单编辑与设备 guard 的联动，不归入单纯设备删除/取消履约。
- token 的签名、期限和 actor 权限变化后的失效规则，以及文件照片授权。
- 以上记录为当前实际行为；如发现旧行为有缺陷，先记录差异，不在迁移时擅自改变业务规则。
