# 售后设备收回：Gin / React Admin

2026-10-30。继承原 `workflows/device-operations.ts` 的 receive 分支，并接入订单售后审核区。新 Admin 使用 Ant Design Drawer、Form、Select、Table 和反馈组件；前台界面不改动。

## 实际入口

`GET/POST /api/admin/orders/:id/after-sales/:request_id/device-returns`。后台订单详情 → 售后申请 → 标记处理中 → 收回设备 → 选择原发货设备及收货来源/仓库 → 扫描原机序列号、IMEI1/IMEI2 或 EID。

读操作要求当前员工显式具有订单/设备/仓库读取权限；写操作另外要求 `device.intake` 和 `order.after_sales.write`。当前是完整员工入口，仓库身份与供应商不能借此读取全店订单。仓库员工专用作用域入口仍需实现。

0070 迁移新增默认关闭的收回开关及持久化收据；已提交迁移总数 69。设备身份及命令开关也须启用。没有自动操作生产库或开启生产写入。

## 继承规则

- 申请必须是当前订单的处理中申请，设备分配与机器均须已发货；关联申请指定设备时不能收另一台机器。
- 扫描必须匹配原机身份；收回数量不得超过该申请行数量，不能重复收回。
- 退货、换货进入 returned / inspection；维修进入 returned / repair。设备保留原始 source_id，实际位置改为所选来源的 location_id。
- 退货创建真实 returns / return_items 记录，并将原分配关联 return_id；使用旧隔离收货语义：damaged_quantity=1、received_quantity=0，订单行 dismissed 与 written_off 增加 1，requested 不变。收回不增加可售数量、不改预留。
- 维修/换货收回不提前创建退货记录，换货的原生换货交易在后续命令中完成。维修返还后，同一机器可进入后续新申请；旧收据不能跨申请恢复。
- 权限和对象归属先于收据重放检查；UUID 绑定原操作者/订单/申请/设备/扫描/来源/版本。晚期收据失败回滚设备、分配、退货事实与事件。

## 证据

[真实联合报告](device-returns-joint-report.json) 从原预配、装包、取消再装包、发货开始，使用真实 React Admin、Gin、MySQL 和 Redis，继续审核与收回。浏览器仅在成功响应后中断回包模拟断线，API 业务结果不模拟；原请求重试恢复唯一结果。数据库复核收据、退回事实、隔离数量及库存，刷新后仍可读取已收回机器。

[本批验证报告](device-returns-report.json) 还包含原审核规则及数量、并发、权限撤销、软删除、处理状态、请求版本与晚期失败回滚验证，以及 29 项后台浏览器测试。

## 尚未完成

本批只交付员工审核与物理收回，不执行检测、维修返还、同 SKU 换机、实际退款或邮件发送，也不代表 C09 已验收。订单操作数量延续当前 Gin 的当前版本更新方式；旧 Medusa 的 order-change 多阶段版本/事件历史仍需合并对账，不能宣称历史变更流水已完全迁移。后续继续检测处理、库存核对、换修/退款、仓库作用域操作和通知，再进行完整售后验收。
