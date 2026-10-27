# 管理后台库存量与在途量编辑

2026-10-28。承接旧后台库存层更新，专用命令不替代订单预留、履约或设备流程。

## 原行为与实现边界

本机安装的 Medusa 2.21.2：`api/admin/inventory-items/validators.js` 的库存层更新
只接受非负 stocked_quantity / incoming_quantity；`location-levels/[location_id]/route.js`
交给 updateInventoryLevelsWorkflow。inventory 模块 sanitizeInventoryLevelInput 排除
reserved_quantity，预留通过 reservation 流程维护。项目
`medusa-store/apps/backend/src/api/device-guards.ts` 的 guardDeviceInventory 直接 next，
`device-inventory-guard.unit.spec.ts` 明确数量库存不受设备登记限制。
reconcile-device-stock 与 enroll-all-device-stock 的 updates 均为空；不能按单机数重置库存。

新命令 `POST /api/admin/inventory-levels/{id}/stock` 设置绝对数量，不是增量。
请求必须恰含 operation_id（规范 UUID v4）、expected_revision（小写 SHA-256）、
stocked_quantity、incoming_quantity。两种数量均为非负、固定 18 位小数的字符串，
整数最多 20 位；拒绝数字 JSON、重复字段、未知字段、指数写法、超精度和尾随文档。
前台 Ant Input 及纯字符串规范化避免 JS 浮点舍入；保留原有小数精度。

## 事务与差异限制

0014 仅开启 inventory.edit_stock 专用能力，通用 inventory_runtime_schema 写能力继续关闭。
只允许当前有效新平台 admin，Origin / Redis CSRF 在入口验证，事务锁内复查员工
角色、启用状态、密码及撤销代数、库存模型和命令能力；未实现普通员工/仓库角色授权。
锁定库存层及有效库存项/仓库；软删和缺失对象拒绝修改。

版本涵盖库存层身份、库存/预留/在途数量及微秒更新时间，防止旧页面覆盖修改。
同操作同员工/同库存层/同请求仅重放审计结果，不重复写库存；更换内容或对象返回 409。
数量更新、精确 raw 数量配对、微秒版本和 before/after 审计在一个 MySQL 事务内完成。
保留预留、metadata、设备档案和原差异记录。审计失败则全部回滚，Redis 不保存业务结果。

减少库存前，用锁定读取核对 item/location 的持久差异记录和当前有效预留，拒绝已有
hold、预留合计不一致或原余额为负的位置。记录上限超出即失败，不截断。
允许补充库存或仅修改在途量，但不自动解除旧 hold。保留原后台可将库存设置为
小于预留量的语义；该操作不会取消预留，后续消耗会受负余额检查限制。
未来预留/履约写命令须先锁同一库存层并复用差异与权限检查，此处尚未实现这些命令。

## 后台与验证

Ant Drawer / Form / Input / Button / Alert / Descriptions 承担编辑和反馈。
数量读取和编辑分开，保存成功或关闭均重读；结果不确定时冻结原请求，仅显式重试
同一操作。冲突、缺失、权限变化、对账限制分别提示；401 返回登录。
库存明细展示持久历史差异的仓库及原因，不回传原始证据 JSON、快照或账号引用。

[MySQL 联合报告](admin-auth-report.json) 包含 17 迁移、8 路不同操作一胜七冲突、
8 路相同操作七次重放、一条审计、审计失败回滚、精度/原字段保留、历史 hold、
实时预留差异、软删父对象和撤权/关闭命令拒绝。
[浏览器报告](admin-live-report.json) 通过真实 Gin/MySQL/Redis 验证精确在途量持久化、
版本冲突、实时预留差异拒绝和中英三宽抽屉。
[人工 HTTP 界面报告](admin-ui-report.json) 验证不确定结果保持原操作及对账拒绝。

完整库存项/仓库 CRUD、批量更新、预留/扣减/履约/取消恢复、人工对账处置、
完整 RBAC/仓库范围、可靠事件及 Astro 仍待完成。上述检查未修改旧业务库或部署服务器。

后续补充：[0015 预留/释放事务参与组件](inventory-reservation-transactions.md)已通过独占人工 MySQL 验证；默认关闭能力、没有外部路由，订单及履约领域仍待接入。
