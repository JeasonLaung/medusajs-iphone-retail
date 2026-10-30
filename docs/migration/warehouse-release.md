# 仓库解除已装包设备分配

仓库详情新增 Ant Design 设备列表与确认对话框。权限为 `order.fulfillment.read` 与 `warehouse.allocation.write`，仅能操作当前绑定仓库内、未发货的包裹。确认内容明确说明：取消整个包裹并恢复数量库存，所选设备恢复可售，其他设备保留订单预占。沿用原 Medusa 的整包取消语义，不做部分包裹扣减。

专用 GET `/api/admin/warehouse/locations/:location_id/orders/:order_id/fulfillments/:fulfillment_id/release` 返回最小设备投影；POST 在该路径追加 `/:assignment_id`，复用平台包裹取消事务、库存恢复、事件和唯一回执。操作绑定仓库、订单、包裹、目标分配和完整正文。权限、仓库绑定及设备当前位置在回执恢复之前重新检查。

隔离 MySQL 验证通过：双设备包裹解除一个设备，目标分配 canceled、设备 available，另一个分配保持 reserved 且解除包裹关联；库存及履约数量恢复，原请求重放不再次恢复库存；错误目标、ship 动作、撤销权限、设备移仓均拒绝。该分支在事务中回滚，原平台包裹生命周期回归继续执行。

真实 React Admin → Gin → MySQL/Redis 联合通过：创建仅具配货权限的仓库账号，390px 英文页面确认后提交，丢弃首次成功响应并使用相同 UUID、版本及正文重试；取消包裹从仓库列表隐藏，平台重新预占与装包后继续换机及整包发货。数据库持久化复核通过，浏览器错误为零。Go 全包与 Admin TypeScript 检查通过。参见 [联合报告](order-parcels-joint-report.json)。

未操作真实账号、订单或旧数据库。未装包预占的仓库解除入口、完整仓库操作到 Astro 客户物流页面的连续验收仍待完成。
