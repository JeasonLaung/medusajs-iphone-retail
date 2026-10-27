# 库存预留与释放事务参与组件

2026-10-28。`inventory.ApplyReservations` 是订单工作流使用的内部 MySQL 事务参与组件。
调用方传入已开启的事务；组件不提交、不提供 HTTP、不调用 provider、不发事件。
0015 的 `inventory.reservations` 专用能力默认 FALSE。当前不代表 checkout、订单取消或
履约已经接通。原后台手工预留入口、移动预留与数量更新也尚未实现。

## 原实现核对

本机安装 Medusa 2.21.2 的 core-flows `dist/cart/steps/reserve-inventory.js`：
每行 quantity=required_quantity×订单行 quantity，选 location_ids[0]，对库存项加锁后
调用 createReservationItems；补偿删除所创建的预留。加购检查不等于成单预留，不能
自动将一个行数量分成多仓预留。此组件接收调用方已选定的仓库，不替调用方选仓。

inventory 的 `dist/services/inventory-module.js`：
createReservationItems_ 汇总同库存项/仓库的预留增量并增加 reserved_quantity；
deleteReservationItems_ 及 adjustInventoryLevelsForReservations_ 减去对应数量；
allow_backorder 可以绕过现货覆盖，但仍需有效库存层。其 ensureInventoryLevels
逐条验证可用量，批量同位置需求未在该循环合并。新组件以同位置非 backorder
需求的合计做容量校验，防止两条各自可满足的行合计超过剩余库存。

## 新事务边界

调用方必须先验证当前身份、权限、订单状态、订单行归属、SKU/库存项用量、渠道与仓库，
并将业务状态写入同一事务。OwnerID 与 ActorRef 只是审计身份，不是授权证明；
不能把客户端 body 直接传给此内部函数。新账号不通过相同邮箱接管历史订单。
未完成订单领域接入之前，默认关闭能力且没有外部入口。

- 每次最多 1000 条，拒绝重复预留 ID。规范 UUID v4 操作 ID，完整请求固定并写入
  `inventory_reservation_operations`，包含 owner、actor、kind、明确行绑定与精确数量。
- 对输入副本排序，不修改调用方请求。按库存项/仓库固定顺序锁库存层，之后再读回执。
  即使事务已建立旧 REPEATABLE READ 快照，重试仍用当前锁定读取观察刚提交的回执。
- 同操作完整请求一致返回原结果；不同 owner、actor、内容、动作或对象拒绝。只有回执
  真正提交后才算成功，Redis 不承担幂等业务事实。
- 新建预留前检查有效父对象、持久差异记录和有效预留明细合计；未知数量、过多明细、
  reserved 与明细不一致均拒绝。历史 hold 不能被 backorder 绕过。
- 非 backorder 行在每个位置按合计校验 stocked-reserved；backorder 行允许超出现货，
  不自动制造库存层。reserved 总和超出 DECIMAL(38,18) 拒绝，不截断、不浮点计算。
- 释放必须逐条匹配预留 ID、库存项、仓库、订单行，拒绝已删记录或再次释放。
  允许在持久 hold 下释放，但按原预留数量减少 reserved，不修复、不清除差异记录。
  无效历史值或释放后 reserved 为负则拒绝；缺订单行的手工预留不从本订单入口释放。
- 更新库存层 reserved/raw_reserved/微秒更新时间、创建预留/raw_quantity、软删释放记录、
  保存 before/after 与操作回执都在同一事务。stocked/incoming、设备、metadata 不随之改。
  释放审计保留旧 allow_backorder 的 NULL；历史行原始字段仍留在软删记录中。
- `RequiredInventoryQuantity` 用 scale18 大整数精确相乘；结果低于支持精度或溢出拒绝，
  不向上/向下舍入。订单流程仍需验证行数量和 required_quantity 的业务合法性。

所有错误都要求调用方回滚整个事务，包含组件成功之后其他订单步骤失败的情形。
未来履约、迁仓、库存编辑与取消必须使用相同库存层锁序；释放不等于恢复已经履约扣减
的 stocked。订单/履约命令的持久状态还需防止跨操作重复履约和重复取消。

## 实际证据与尚缺

[MySQL 联合报告](admin-auth-report.json) 的 reservation_transactions 使用人工数据和实际
17 文件 schema，验证最后一件 8 路仅一胜、共享库存项多行合计、8 路同操作一次写入、
旧 RR 快照重放、相反输入顺序的双位置竞争、组件回滚、回执失败回滚、后续 owner 步骤
失败回滚、末位精度/backorder、hold 新建拒绝但释放保留、重复/跨行释放拒绝、NULL
backorder 审计、标识冲突、溢出与模型漂移拒绝。默认关闭能力也实际验证。

[固定快照](catalog-mysql-rehearsal-report.json) 校验新 schema 下旧目录/库存导入、读取
及精确重放保持指纹；该演练不对真实快照执行预留写入。历史 375 缺引用、4 项库存差异
不因此被修复或验收。

尚缺订单正式 schema/行关联、所有权与角色/仓库授权接入、checkout 原子成单、
预留修改/迁仓/手工入口、履约扣减和取消恢复、外部副作用与恢复协调器及 Astro。
不得把本组件标为完整库存、订单或交易模块完成。旧服务与部署服务器未修改。
