# 原生入口上的项目保护规则

T001/T004/T005/T007 的部分证据。`fixtures/legacy-native-guards.json` 从项目 middlewares.ts 的对象和数组 map 挂载语法提取9个保护函数、25条方法挂载，连同六项真实原生入口追踪保存34个来源哈希；实际执行 device-guards.ts/payment-guards.ts，使用人工容器、查询和设备端口，共46项独立断言。未启动 Medusa、未读环境/凭据、未连接业务库或 provider。

安装版本来自本地 @medusajs/medusa/package.json：2.21.2。另静态检查对应项目 route.ts 和安装的 dist/api/**/route.js 的方法导出，**21条找到处理器来源，4条未找到准确文件或方法**。找到源码也不证明 HTTP、身份、验证、事务或 provider 已运行。

## 设备与数量库存

| 保护函数及挂载 | 真实旧行为 | Gin 必须承接的约束 |
| --- | --- | --- |
| guardDeviceOrder：POST orders/:id/fulfillments、edits | 订单含受管变体，或有 reserved/packed/shipped 分配时拒绝；普通商品无活跃分配放给后续处理 | 原生履约/编辑不能绕开设备配货与履约命令；普通商品仍有完整流程，不机械禁止所有订单 |
| 同函数：POST orders/:id/cancel | 有受管变体但无活跃分配可继续；有活跃分配拒绝 | 保留整单取消的状态与全部影响对象校验，不把“含设备”作为唯一判断 |
| 同函数：POST orders/:id/fulfillments/:fulfillment_id/shipments、cancel | 专用路径直接 next，不读设备数据 | 专用处理器必须承担完整订单/履约归属、设备状态、库存/付款和幂等；不能由这次 next 推断已授权 |
| guardDeviceInventory：POST/DELETE 三种 location-levels 路径，DELETE inventory-items/:id | 明确是 no-op，直接 next | 设备档案不拥有数量库存；手工数量维护仍按库存动作权限、预留/删除等领域约束执行，不添加“序列号设备禁止维护库存”的新规则 |
| guardDeviceReturn：POST admin/returns、returns/:id/receive/confirm、store/returns、admin/exchanges、exchanges/:id/request/confirm | 找到订单后，只要有任何设备分配记录就拒绝，不按 reserved/packed/shipped 筛选 | 设备退回经核对原机和检测流程；普通商品原生退回/换货仍需承接；不能只阻止某个页面按钮 |

上述路径省略 `/admin/` 的仍为后台路径。完整方法及路径以 fixture 为准。退回保护先使用 body.order_id，缺失时从路径 return/exchange 读取订单；人工反例确认 body 可优先于路径。新命令必须核对路径资源、body 与数据库归属一致，不能复制该优先级作为绕过检查的方式。缺订单、没有查询结果或保护函数调用 next 都不是有效订单/付款授权证据，后续处理器仍须拒绝无效资源。

数量库存的 delete/force、负库存、预留及删除后历史引用等行为要继续核对原生处理器，no-op 不等于没有规则。受管设备状态与数量库存的独立关系见 [设备命令](device-commands.md)。

## 生产手工付款禁用

| 入口 | 项目旧保护 | 新边界 |
| --- | --- | --- |
| POST store/admin payment-collections/:id/payment-sessions | 生产 provider_id=pp_system_default 拒绝 | 购物车拥有者或明确员工付款动作；地区/provider白名单，付款尝试绑定金额币种版本；禁止通过后台选择手工 provider |
| POST admin/payment-collections/:id/mark-as-paid | 生产无条件拒绝 | 不提供可伪造实收的生产捷径；保留拒绝并使用真实 provider 结算事实 |
| POST store/carts/:id/complete | 手工会话 pending/requires_more/authorized/captured/pending_authorization 任一存在即拒绝；canceled/error 不触发此保护 | 完整成单仍需付款、库存、重复订单和可信 provider 状态校验；非活跃手工会话不应错误阻止合法在线付款 |
| POST admin/orders/:id/payment-sessions/authorize | 按请求 payment_session_id 查 provider，手工拒绝 | 必须同时验证 session 属于路径订单、当前动作权限与金额状态；只查 provider 不足以证明归属 |
| POST admin/payments/:id/capture | 按 payment ID 查 provider，手工拒绝 | 真实订单/付款范围、可扣金额、provider幂等与审计；禁止以改名或 worker 路径绕过手工禁令 |
| GET store/payment-providers | 生产必须 region_id，按 region 查询并排除手工 provider，投影 payment_provider | 公开 allowlist DTO、地区和启用状态；不返回密钥 |
| GET admin/payments/payment-providers | 在已有过滤条件上 AND 排除手工 provider | 员工授权、相同生产禁令，不能用普通配置权限触发交易 |

开发/test 环境这些生产保护直接 next；这不表示生产支持手工实收。新环境要用明确配置及隔离测试 provider，不能由浏览器参数切换生产限制。原生保护在找不到 cart/payment/session 时可继续，Gin 必须由最终命令保证资源存在和当前归属，不能仅复制 guard 返回值。

## 四条注册与处理器不一致的证据

| 注册的方法/路径 | 当前准确源码检查 | 处理方式 |
| --- | --- | --- |
| POST /admin/orders/:id/edits | 项目及安装包该路径无 route 文件 | 实际创建为 POST /admin/order-edits；保留“编辑不绕过设备状态”业务要求；不把旧挂载当成功调用证据 |
| DELETE /admin/inventory-items/:id/location-levels | 原生文件存在，导出 POST/GET，未导出 DELETE | 批量/单仓删除需按真实处理器契约承接；不造一个已存在的旧 DELETE 功能 |
| DELETE /admin/inventory-items/:id/location-levels/batch | 原生文件存在，导出 POST，未导出 DELETE | batch POST 的 delete 数组是另外的实际能力，不能因这条 DELETE 未导出而漏迁批量删除 |
| POST /admin/exchanges/:id/request/confirm | 项目及安装包该路径无 route 文件 | 实际确认为 POST /admin/exchanges/:id/request；保护必须针对真实入口，不能据此取消换货能力 |

这只是准确路径/导出检查，不能排除框架动态路由、插件、编译扩展或其他入口。四项不标“未使用可删除”；未发起真实付款、退货或库存请求来探测。

已继续检查六项真实入口的导出及 workflow 调用并记录哈希：POST /admin/order-edits → beginOrderEditOrderWorkflow，POST /admin/order-edits/:id/request → requestOrderEditRequestWorkflow，POST /admin/order-edits/:id/confirm → confirmOrderEditRequestWorkflow，POST /admin/exchanges/:id/request → confirmExchangeRequestWorkflow，POST 库存 location-levels/batch → batchInventoryItemLevelsWorkflow，DELETE 单仓 location-levels/:location_id → deleteInventoryLevelsWorkflow。订单编辑使用 body 中订单及后续 order_change 归属，不能继续依赖不存在的路径 params.id；确认时也要重新检查设备状态。原生 request/confirm 的完整状态规则、响应和权限仍待运行验证，不能仅凭函数名称认定流程已理解。

## 可重复验证与剩余工作

```powershell
node retail/scripts/capture-native-guards.cjs --check
```

真实 guard 函数在受限 require 下执行；框架错误类、remoteQueryObjectFromString、服务查询为显式替身。测试固定输入、预期放行/拒绝、查询对象/过滤/字段/分页及端口调用序列，不加载原生处理器。方法挂载从 AST 取出，新增未识别语法会失败；源码/安装版本/输出变化会导致 --check 失败。

尚需逐项核对原生实际路由及请求/响应、认证/RBAC、编辑/退回/换货/库存事务、provider失败和并发恢复，形成新 Gin schemas/命令与 Ant Design 操作界面。现有46个样例不作为新实现或全交易验收；T001—T007 继续保留未完成项。
