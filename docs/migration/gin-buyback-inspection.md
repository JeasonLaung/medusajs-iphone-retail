# 回收设备关联与最终报价

2026-10-30。依据原 buyback-validators.ts 的 BuybackAdminCommand、workflows/buyback.ts 第 125–150 行与 buyback-request.ts 的 BuybackDeviceLink 模型核对，本批已接入平台设备关联、逐台最终报价及客户接受报价的真实联合流程。后续打款和退回登记见 [登记与退回](gin-buyback-settlement.md)，邮件/图片等仍未整体验收。

## 已实现并验证

ParseInspectionCommand 严格承接 link_device 与 final_offer。ID 按 JS trim 和 UTF16 长度，检测说明 trim 后 3–1000 字符；最终报价每台 0–1000000、有限数、精确到分，复用已由原 Zod 对照的 binary64 金额规则。报价数组 1–10 台，逐项拒绝未知键、重复键、null、错误字段大小写及字符串金额。最大 64 KiB 容纳十台各一千字中文说明。重复 item_id 在原 Zod 输入层允许，业务层已核对唯一性。

实际执行原 Zod 生成 68 组固定输入，21 组接受；Go 逐组比较接受结果和归一化输出，通过零金额、小数边界、最大金额、ID/说明 UTF16 边界、十台长中文说明、缺字段及未知字段。额外验证重复 JSON 键拒绝，以及审核命令解析器仍与检测命令解析器严格区分。机器证据：fixtures/buyback-inspection-command.json；复核脚本 retail/scripts/capture-buyback-inspection-command.cjs --check。Go buyback 包测试通过。

## 实际业务与数据约束

- 关联仅允许 received，申请中的 item 必须存在且未关联。设备 category=recycled、state=inspection，SN/IMEI 与客户提交的值一致；设备 variant 对应同一 product，选项值移除 JS 空白并大写后匹配申请容量。此处不能用其他 SKU 或仅凭 SN 放行。
- 原链接表对未删除 item_id 和 device_id 各有唯一索引；Gin 持久化必须承接，不能只更新申请 JSON，否则跨申请并发可重复占用设备。操作事务还要复核当前授权、店铺和设备归属，失败回滚链接/申请/历史/审计。
- 关联不改变库存数量、不把设备提前变成可售、不发送客户更新通知。现有设备登记已支持 recycled→inspection，可复用实际入库流程。
- 最终报价仅允许 received 且所有台均已关联，报价 item 集合必须恰好覆盖申请、不可重复；每台保存 amount 与 notes，合计保留原逐项相加后按分取整规则，转 final_offer 并产生原事务通知。接受/拒绝继续接已有客户命令。

## 已接入

0078 新建 buyback_device_links，保留未删除 item 和 device 各自唯一、软删和申请/设备外键。新增动作复用默认关闭的 buyback_admin_runtime 和 buyback.read/write 权限，事务先锁申请再锁设备。候选 GET /api/admin/buyback/:id/items/:item_id/devices 仅返回严格匹配的设备 ID；POST 原详情路径识别严格检测命令，最多 64 KiB。跨店铺、仓库角色、当前授权撤销和关闭写开关仍拒绝。

React Admin 使用 Ant Design 的 Select、InputNumber、TextArea 和 Modal，关联设备后才能发送报价。每台展示设备 ID、金额和检测说明；零金额合法，不修改前台原组件或页面。发送后客户通过原前台接受或拒绝。

## 验证证据

- 77 个正式迁移已在隔离 MySQL 实际应用，0078 两个唯一约束实际验证；未包含工作区独立未完成的 Mollie 0037。
- SQL 拒绝错误类别/状态/序列号/IMEI/软删/仓库、产品/容量/失效来源/渠道；22 组非 received 的关联/报价命令拒绝。当前候选与实际写入独立复核；撤权与关闭写入拒绝。
- 整体事务后续失败时申请、链接、历史、审计和站内通知全部回滚。竞争关联和竞争报价均只有一个胜者；单台零金额、多台报价输入乱序/按申请顺序合计、客户接受与拒绝验证通过，数量库存和预留不变。
- 真实 Astro/React Admin → Gin → MySQL/Redis：后台选设备、关联、提交 270.01 EUR 报价，客户在原回收详情接受，双方刷新仍为 accepted；设备链接、逐台检测说明、合计和最终报价通知持久化。关联不发通知、报价通知精确一次、零浏览器错误。检测设备本身为隔离 SQL 人工夹具；本次不声称实际浏览器完成设备入库。
- 原问卷/照片/审核/寄出/收货/取消和账号隔离联合仍通过，原页面及样式继续复用。见 [联合报告](buyback-joint-report.json)。

Admin TypeScript、173 项前端测试、Go 全量与 Astro 391 文件检查通过（零错误警告，5 条既有提示）。较广 SQL 回归在 OrderSchema 后重置隔离数据库失败，不记录为全量通过；修正临时运行器等待最终 MySQL 进程就绪的条件后，AdminAuthentication / InventoryStockEdits / ProductInventory 三套补跑通过，隔离资源已清理。旧 health/app/sell 均 200，stderr 未新增，既有 locales 404 仍在。

## 剩余范围

打款登记和退回物流/送达已在后续 [登记与退回](gin-buyback-settlement.md) 验证。通知邮箱、历史回收申请/图片导入、180 天图片清理及 HTTPS Secure cookie 联合仍待办。设备入库既有模块的独立验证不能替代整个回收付款/退回闭环。
