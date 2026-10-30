# 回收打款登记、退回与通知合并

2026-10-30。按原 BuybackAdminCommand 与 updateBuybackRequest 承接四个剩余平台动作，不接入或调用外部转账/承运商。

## 原业务与实现

- mark_paid 仅允许 accepted，付款凭证 trim 后 3–200 个 UTF16 字符；写 paid、payment_reference 和 closed_at。
- request_return 仅允许 received/final_offer，说明 trim 后 3–1000 字符；写 return_requested 与 review_note。
- mark_returning 仅允许 return_requested，退回单号 trim 后 3–150 字符；写 returning 与 return_tracking。
- mark_returned 仅允许 returning；写 returned 与 closed_at。

复用现有 buyback.read/write、默认关闭写开关、当前授权复查、店铺/新账号归属及申请锁。每个动作的状态、凭证/单号、历史、审计和站内通知在同一事务，重复终态命令拒绝。保持原行为：登记已完成的线下付款，不发起转账；登记退回，不购买标签，不修改数量库存或自动改变设备状态。没有新增数据库表。

React Admin 继续用 Ant Design 的按钮、表单和确认 Modal。页面仅在合适状态显示动作，显示原退回地址、付款凭证和退回单号。前台直接复用原回收详情，不改布局或交互。

## 通知行为修正

核对原 customer-notifications.ts 后发现之前 Gin 按每个动作新增通知，与原实现存在差异。本批改为：同一账号/申请存在未读 buyback.updated 时更新其 action/event_key/reference，保留 id 与 created_at；已经读过的提醒保持不变，下一次业务更新新建一条未读提醒。事务中对未读行加锁，写入仍带 read_at IS NULL，避免覆盖已经读过的提醒。设备关联和照片直接审核通过仍不发送提醒。

因此此前文档中的三条逐动作未读通知证据已由一条最新未读提醒替代。具体原始来源为 medusa-store/apps/backend/src/workflows/customer-notifications.ts，非邮件发送器。

## 验证

- 原始 Zod 116 组固定输入、31 组接受，Go 逐项比较归一化输出；空值、UTF16 边界、未知/跨 union 字段、重复键拒绝。
- 实际 MySQL：12 状态 × 8 动作共 96 组；新增四个动作各两事务竞争只有一个胜者，终态关闭时间及付款/退回字段验证，后续失败回滚。通知合并保留 id/创建时间，已读后新建下一条提醒。
- 实际 Astro/React Admin → Gin → MySQL/Redis：原问卷、照片、审核、寄出、收货、设备关联和最终报价之后，分别执行客户接受→登记 TRANSFER-JOINT→paid，客户拒绝→登记 RETURN-JOINT→确认送达→returned。双方刷新保留终态、金额、凭证/物流与 closed_at；后台中文手机布局、客户原手机页面和零浏览器错误验证。
- 检测设备为独立 SQL 人工夹具，转账和退回物流使用人工参考号。验证证明本地业务登记与页面闭合，不证明实际资金到账或承运商送达。读写全部人工账号及隔离库，测试结束清理自有容器/网络/运行器。
- Go 全量、Admin TypeScript、173 项前端测试通过；Astro 391 文件零错误警告，5 条既有提示。旧站 health/app/sell 均 200，stderr 无新增，既有 locales 404 保留。

报告：[打款路径](buyback-joint-report.json)、[退回路径](buyback-return-joint-report.json)。复核原契约：node retail/scripts/capture-buyback-admin-command.cjs --check。联合运行器 python retail/scripts/test-buyback-joint.py；BUYBACK_INSPECTION_DECISION=decline 选择退回路径。

## 尚未完成

通知邮箱的整体业务验收、历史回收申请与私有照片导入、180 天闭单照片清理、真实设备入库到回收的浏览器联合及 HTTPS Secure cookie 联合尚未完成。本批不宣称全部迁移完成。
