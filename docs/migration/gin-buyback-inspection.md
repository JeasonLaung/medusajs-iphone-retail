# 回收检测与最终报价契约

2026-10-30。依据原 buyback-validators.ts 的 BuybackAdminCommand、workflows/buyback.ts 第 125–150 行与 buyback-request.ts 的 BuybackDeviceLink 模型核对，当前完成输入契约准备；尚未开放检测或报价 HTTP，不能计为检测流程可用。

## 已实现并验证

ParseInspectionCommand 严格承接 link_device 与 final_offer。ID 按 JS trim 和 UTF16 长度，检测说明 trim 后 3–1000 字符；最终报价每台 0–1000000、有限数、精确到分，复用已由原 Zod 对照的 binary64 金额规则。报价数组 1–10 台，逐项拒绝未知键、重复键、null、错误字段大小写及字符串金额。最大 64 KiB 容纳十台各一千字中文说明。重复 item_id 在原 Zod 输入层允许，必须在后续业务层核对唯一性。

实际执行原 Zod 生成 68 组固定输入，21 组接受；Go 逐组比较接受结果和归一化输出，通过零金额、小数边界、最大金额、ID/说明 UTF16 边界、十台长中文说明、缺字段及未知字段。额外验证重复 JSON 键拒绝，以及当前 ParseAdminCommand 仍拒绝未实现的这两个动作。机器证据：fixtures/buyback-inspection-command.json；复核脚本 retail/scripts/capture-buyback-inspection-command.cjs --check。Go buyback 包测试通过。

## 原业务约束与下一步

- 关联仅允许 received，申请中的 item 必须存在且未关联。设备 category=recycled、state=inspection，SN/IMEI 与客户提交的值一致；设备 variant 对应同一 product，选项值移除 JS 空白并大写后匹配申请容量。此处不能用其他 SKU 或仅凭 SN 放行。
- 原链接表对未删除 item_id 和 device_id 各有唯一索引；Gin 持久化必须承接，不能只更新申请 JSON，否则跨申请并发可重复占用设备。操作事务还要复核当前授权、店铺和设备归属，失败回滚链接/申请/历史/审计。
- 关联不改变库存数量、不把设备提前变成可售、不发送客户更新通知。现有设备登记已支持 recycled→inspection，可复用实际入库流程。
- 最终报价仅允许 received 且所有台均已关联，报价 item 集合必须恰好覆盖申请、不可重复；每台保存 amount 与 notes，合计保留原逐项相加后按分取整规则，转 final_offer 并产生原事务通知。接受/拒绝继续接已有客户命令。
- 下一批需完成链接表迁移、设备候选读取、关联事务与故障/并发实库验证，再接后台选择器及逐台报价表单。实际检测/最终报价联合、付款/退回与通知邮箱仍未完成。
