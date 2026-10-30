# 仓库账号履约读取

2026-10-30：React Admin 新增仓库履约入口，使用 Ant Design 表格、搜索、仓库切换和详情抽屉。Gin 提供独立仓库投影，复用 `order.fulfillment.read` 与当前仓库绑定；不返回订单付款、成本、其他仓库包裹或设备内部字段。详情展示商品精确数量、设备序列号和物流单号。本次只完成读取，仓库配货写入仍待迁移。

接口位于 `/api/admin/warehouse/locations/:location_id/fulfillments` 和 `/api/admin/warehouse/locations/:location_id/orders/:order_id/fulfillments/:fulfillment_id`，在登录中间件之后注册。查询参数仅接受单个 `q` 和规范整数 `offset`，详情不接受查询参数。包裹必须未删除、未取消且仍属于指定仓库和订单。

验证分为两部分，不能视为浏览器与真实后端的联合验收：

- `identity/warehouse_fulfillments_test.go` 接入现有装包发货演练，在隔离 MySQL/Redis 和实际 Gin 路由下通过。验证已发货包裹、商品、设备与物流单；移仓、取消、错误订单不可见；撤销仓库绑定或读取权限立即拒绝；仓库账号仍不能调用全店接口，供应商也不能读取。
- `admin/tests/warehouse-fulfillments.browser.mjs` 使用真实 React Admin 和模拟接口，覆盖英文/中文、390/1440 宽度、精确数量、详情展示、依赖故障重试、权限撤销、错误仓库响应和空授权范围，浏览器错误为零。

执行后端集成演练可使用 `ADMIN_AUTH_TEST_BACKEND` 指向隔离的后端源码，再运行 `python retail/scripts/test-admin-auth.py --customer-accounts-only`。该套件包含现有订单装包演练。浏览器脚本在 `retail/admin` 下以 Node 22 运行。此次隔离后端全部 Go 测试和 Admin TypeScript 检查通过；不接触真实业务数据。

Existing Admin browser regression: 31 passed. Existing Ant Form connection warnings remain in after-sales regression cases. Legacy backend health/Admin and storefront account each returned HTTP 200.

## 后续真实联合补验

同日补跑 `python retail/scripts/test-warehouse-inventory-joint.py --order-parcels`（`WAREHOUSE_JOINT_BACKEND` 可指定独立后端源码）。平台生产页面实际预占设备、装包、取消、重新装包和整包发货后，再由生产后台创建仅绑定包裹仓库的新账号。四个独立登录会话覆盖中英/390及1440，查看相同商品、两台设备和实际物流单；已取消包裹不可读、其他仓库和全店订单拒绝，字段投影不泄露内部信息。没有模拟接口或直接 SQL 修改权限。管理员移除读取权限后旧会话返回 401，新登录仍对该包裹返回 403，页面显示无权限。

生产命令形成的数量库存、预留、履约/发货数量、两个设备状态和唯一物流标签经 MySQL 再核对；自有容器/网络/临时程序全部清理。见 [联合报告](order-parcels-joint-report.json)。此前两部分独立验证的描述保留为历史记录；本次补齐了仓库读取的真实联合路径，未完成仓库写操作，规则见 [写入契约](warehouse-fulfillment-write-contract.md)。
