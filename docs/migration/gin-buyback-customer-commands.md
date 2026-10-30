# 客户回收申请操作

2026-10-30。Gin 新增客户 POST `/api/store/customer/buyback/requests/:id`，支持原五类动作 submit_photos、mark_shipped、accept_offer、decline_offer、cancel。沿用原状态限制，无新增自动重试语义：已成功转态后重复操作拒绝，不追加重复历史。

## 契约与业务

实际执行原 BuybackCustomerCommand Zod 校验器生成 71 组对照，其中 24 组接受；归一化输出也与 Go 对比。序列号 JS trim/Unicode uppercase/UTF16 长度、item_id trim、15 位 ASCII IMEI、物流 3..150 字符、JS Number 归一化后 0..1000000 的分币金额，保持原行为。严格原始 JSON 拒绝重复键、大小写别名、额外字段、错误 union 分支和 null。

命令在客户账号锁后复核 Redis，再锁申请行；只认新账号/店铺显式归属和当前有效申请，沿用默认关闭的回收写开关。申请、设备身份、状态、物流、历史和取消时关闭日期在同一事务保存。

提交照片要求设备身份恰好完整覆盖，SN/IMEI 各自唯一，最新版本的正反面及 damaged/visible 边框所需 edges 齐全；pending/approved 可用，retake/expired 拒绝。mark_shipped 仅 ready_to_ship；接受报价仅 final_offer 且金额仍与当前报价的 JS Number 相同，拒绝报价仅 final_offer；取消仅 awaiting_photos/photo_review/ready_to_ship。客户动作按原流程不向该客户生成站内通知。接受报价只是确认，不会执行付款。

Astro bridge 增加固定 POST 命令路径和 200 成功契约，保留原字节签名、CSRF/cookie 白名单、16 KiB 输入与私有 no-store；回复允许原详情数据大小，五个既有业务错误码仅按匹配状态透传且隐藏服务端说明。原列表/详情 React 组件尚未接入，本轮不声称页面闭环。

## 验证

- 75 迁移的隔离 MySQL/Redis/Gin 验证 12 状态×5 动作的 60 组矩阵。
- 两部设备、条件 edges、缺照片/retake 拒绝，SN/IMEI 重复拒绝，账号隔离，双提交仅 1 成功/1 冲突，报价接受/拒绝并发仅 1 成功；过时报价、写禁用、会话撤销拒绝，取消 closed_at、物流归一化、历史及后续失败整体回滚。
- 真 HTTP 验证 CSRF/session/query/重复 JSON，原客户动作不发站内消息。
- 原 Zod 对照/源码 SHA 校验、Go 全量通过。前端 171 项测试、Astro 360 文件 0 错误/警告（5 hints）通过。
- 后台鉴权、库存编辑/商品库存 SQL 回归范围另见报告。旧 health/app/sell 均 200、近期 stderr 未新增，原 locales 404 保留。

平台审核/物流接收/检测关联/最终报价、付款退回、通知邮箱仍未验收；测试里的平台状态仅人工 SQL 设置。后续接回原列表、详情、照片及五个客户操作并做真实浏览器联合。详见 [报告](buyback-customer-command-report.json)。
