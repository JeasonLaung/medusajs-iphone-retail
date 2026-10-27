# 自定义路由逐方法承接

以 source-manifest.json 的 67 个 route.ts、86 个方法为固定集合，逐项连接已有 HTTP 调用和能力入口设计，并补充此前没有调用方设计的 10 个方法。机器清单位于 custom-route-contract.json，每项保留源码、方法行号、哈希、设计引用和剩余验证；共验证 149 个相关源码文件哈希。

这是 T001/T004/T007 的证据，尚未实现或验收 Gin 接口。86 个方法不是 86 个独立功能；同一路由的 action/resource 分支仍需逐项契约。未覆盖的原生后台、插件及 provider 路由不能因不在自定义目录而删除。

## 补充的十个方法

| 旧方法及路径 | 新去向及必须保留的行为 |
| --- | --- |
| GET /admin/custom | /api/admin/custom；员工授权下保留现有 200 空探针，无业务读写，不冒充健康检查 |
| GET /store/custom | /api/store/custom；保留现有 200 空探针，不代表商城业务已完成 |
| GET /admin/inventory-items | /api/admin/inventory-items；继承普通查询，并支持 reserved_quantity / stocked_quantity 正反排序，空聚合按零、ID 稳定次序；过滤与授权后精确聚合再分页，count 同范围 |
| GET /admin/orders | /api/admin/orders；仓库 order-read 与履约仓库范围，嵌套履约、商品数量和字段裁剪；供应商通用接口拒绝 |
| GET /admin/orders/[id] | /api/admin/orders/{id}；明细先查范围再投影，不能通过 fields 查询其他仓库数据；供应商设备绑定不授予订单接口权限 |
| GET /admin/stock-locations | /api/admin/stock-locations；当前范围与业务过滤 AND 后分页计数；null 全部、[] 无仓库 |
| GET /admin/stock-locations/[id] | /api/admin/stock-locations/{id}；明细仓库授权与字段限制，供应商通用接口拒绝 |
| POST /admin/orders/[id]/fulfillments/[fulfillment_id]/cancel | /api/admin/orders/{id}/fulfillments/{fulfillment_id}/cancel；设备 cancel_fulfillment 与数量库存取消一致，保留 no_notification、操作人和完整影响集合 |
| POST /admin/orders/[id]/fulfillments/[fulfillment_id]/shipments | /api/admin/orders/{id}/fulfillments/{fulfillment_id}/shipments；受管设备或已有配货走设备发货，否则走普通商品发货；所有 labels 保留，创建发货不同于修改追踪号 |
| POST /store/payment-collections/[id]/payment-sessions | /api/store/carts/{cart_id}/payment-attempts；改为当前拥有的购物车创建/恢复付款，服务端绑定版本、精确金额和币种，不接受 collection ID 作为权限证明 |

库存排序旧源码对所有候选分批读入内存，再 Number 比较、ID localeCompare；新设计采用精确 SQL 聚合排序，须增加等值、无库存层、分页一致性及 ID 次序对照。不能把原生列表过滤能力缩成只有排序开关。仓库角色目前不能访问通用库存列表；商品嵌套库存的范围裁剪是另一个入口。

订单和仓库列表需继承原生查询参数；旧仓库中间件不仅过滤订单 ID，还改写 fields 和返回结构。供应商的通用订单 API 当前直接拒绝，即使 metadata 有允许的订单 ID，也只供限定设备命令使用。新授权不得扩大为供应商可读完整订单。

履约写入要验证 fulfillment 属于路径 order，检查整个受影响集合。通用发货/取消接口继续拒绝仓库与供应商账号；仓库获准的设备 ship 等命令按独立权限执行。设备/普通商品两条路径均需事务、幂等和可靠外部意图，不使用 Medusa workflow 作为新运行时依赖。

Mollie 旧恢复逻辑会先刷新 provider，多个 authorized 会话拒绝；优先已支付，否则选金额币种匹配且 pending/requires_more 的会话，再删除其他会话。新系统须用隔离 provider 夹具核对恢复、重复付款与取消失败，不直接复制旧浮点比较或假定删除 provider 会话总能成功。生产环境手工支付禁用规则继续继承。

## 可复查范围

```powershell
python retail/scripts/map-http-caller-contract.py --check
python retail/scripts/map-capability-entrypoints.py --check
python retail/scripts/map-custom-route-contract.py --check
```

缺少设计、重复方法、相互矛盾的首选设计、已登记源码变化或补充方法消失都会失败。多个调用方保留各自引用，不增加服务端方法数量；同一方法存在能力入口时，以更靠近服务端边界的能力设计为主，保留调用方证据。

完整新请求/响应 schemas、错误枚举、内联验证器、动态命令与资源、原生/插件内部能力、MySQL 事务和真实 UI 仍待验证。现有后台组件约定保持 **优先 Ant Design，全部图表 ECharts**，不因接口映射齐全将 T007 或界面任务标为完成。
