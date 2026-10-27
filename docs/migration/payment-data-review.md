# 支付 provider 历史 data 转换

2026-10-27。对应 T006/T018/T035 的迁移组件，不代表 Gin 支付运行时或完整支付行已实现。源码依据为 `medusa-store/apps/backend/src/modules/mollie/service.ts` 和固定离线快照；结果见 [计数报告](payment-data-review-report.json)。

`rowimport.ReviewPaymentData` 接收来源表/行/provider/session/币种/精确金额及 data 原文，只支持 `payment_session`、`payment`。手动 provider 仅接受空对象；这是保留历史，不能启用生产手动付款。Mollie 仅接受经核对的字段，未知 provider、未知字段、重复对象键、错误类型及未审查的 `last_refund` 等载荷全部拒绝，无部分结果。

金额按 DECIMAL(38,18) 契约转换后用有理数比较，不经过 float。核对 amount.currency/value 与行币种/金额、data.session_id 与行 session、metadata.session_id/cart_id 与 data 同名引用。这里不证明该 cart 存在、关联集合完整或远端付款状态正确；provider 快照状态可能滞后于其他业务事实，不据此修改原状态。

交易账单联系人、provider 主体、手续费、checkout 链接等作为私有历史付款证据保留；明确的嵌套结构逐层校验。原 JSON 字节完整保留，不格式化、重算或舍入。URL 只检查可保存的 HTTP(S) 结构，不发请求、重放回调或恢复支付能力。未知认证载荷拒绝，不自动放进扩展 metadata。

`SealPaymentData` / `OpenPaymentData` 使用新业务密钥和 `ers1.` 文本信封，适配目标 LONGTEXT；AAD 绑定 provider、目标表、data 列及原行 ID。解密后再次核对上述来源事实，禁止明文/旧密文回退。此内部读取接口不授予新客户历史交易归属。SQL NULL 需由后续完整行转换入口单独处置，不能当作空对象。

本轮验证：

- 固定快照 42 个非 NULL data：两表各 13 个空手动对象、8 个 Mollie 对象；16 个 Mollie 对象精确金额、币种与上述引用均通过。
- 42 次临时信封往返保留原文，42 次错行 AAD 拒绝；真实密钥/新信封未落盘，无数据库/provider 调用。
- 人工用例验证超过 JS 安全整数的金额与第 18 位小数差异、未知/重复/嵌套认证字段、类型/引用冲突、错 provider/表/行/币种/金额及篡改信封。

运行：`python retail/scripts/audit-payment-data.py --bundle logs/gin-migration/source-20261026.json`，另运行 `go test ./...` 和 `go vet ./...`。人工测试不使用真实支付服务。

仍缺：metadata 等其他保护 JSON、完整行/SQL 接入、真实 v2 导入及付款/退款/在途恢复。普通 `Convert` 与商户配置转换入口继续拒绝非 NULL 保护 JSON，不能绕过本组件或宣称全域迁移完成。
