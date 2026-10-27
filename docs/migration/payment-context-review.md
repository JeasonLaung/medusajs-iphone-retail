# 支付context中的历史主体与账号资料处置

日期：2026-10-27。仅覆盖payment_session.context，不代表全部支付JSON、整行导入、provider对账或在途支付恢复已完成。

## 来源与范围

固定快照中的21个payment_session.context均为非NULL JSON对象：9个包含customer，2个包含顶层account_holder。customer中含旧地址簿addresses、账号到付款主体绑定account_holders，以及部分交易billing_address。旧地址簿与账号绑定在用户“不迁移旧账号”决定及现有排除映射中不恢复，不能通过嵌入JSON绕过排除；交易账单地址与provider付款主体证据则须保留，不能误当旧登录账号删除。

`rowimport.ReviewPaymentContext`现在使用逐层固定字段白名单：

| 位置 | 处置 |
| --- | --- |
| 根 | 仅customer和account_holder；未知根字段拒绝 |
| customer.id/email/first_name/last_name/company_name/phone | 保留历史联系原文，ID非新账号主键，不按邮箱授予订单权益 |
| customer.addresses/account_holders | 移除整个旧地址簿/账号绑定数组，记录固定字段移除次数，不复制到其他列 |
| customer.billing_address | 保留经核对的账单地址/联系、历史address/customer标识与时间快照；移除is_default_billing/is_default_shipping，不建立客户地址簿或默认地址 |
| customer和billing_address的metadata | 仅允许空对象或JSON null；非空必须另行审查，不按token关键词猜测 |
| 顶层account_holder | 保留交易provider主体的id/external_id/provider_id/email/时间；data/metadata只允许空对象或JSON null，不混同已移除的customer.account_holders绑定数组 |

允许的历史标识最多191字符，字符串/时间/布尔逐项检查；未知键、重复对象键、错误类型、无效UTF-16/UTF-8及超1MiB文档均拒绝，错误固定ErrHistoryPolicy且无源值。此入口只接受JSON对象，JSON null拒绝；SQL NULL须由真正的v2 cell调用方区分。删除数组可以是数组或显式JSON null，其他类型拒绝，不解析其中能力为新身份。

结果为显式投影：被移除字段不再保留；受保留字段的原始JSON值token（包括空白、转义与字符串空格）按RawMessage直接写入，不经过float64或json.Marshal(map)压缩值。输出对象键按固定排序，不能宣称整个原文JSON字节未变。报告仅有固定字段名及计数，无ID或地址资料。

## 临时信封

`SealPaymentContext`先审查，再使用新Ring，以payment-context-history/payment_sessions/context/源sessionID为AAD密封。此列的既有目标设计是LONGTEXT加密封装，因此输出`ers1.`加规范base64的二进制新信封，不能直接把任意AES字节塞进UTF-8文本列。SQL NULL与信封不混同。商户字符串LONGBLOB列仍使用二进制信封，此处不改其存储约定。

`OpenPaymentContext`严格验证文本标记、规范base64、长度、版本/key ID及AAD；拒绝旧密文/明文回退。解密后再次执行内容白名单，并要求移除计数为0：即使有人把未经投影的旧context用正确新密钥密封，读取也会因仍含排除字段而拒绝。该内部函数不授予公开读取或新客户归属，不查询provider，不把旧session激活。

普通Convert及ConfigSecrets仍对非NULL payment_session.context和其他protected_exact_json保持ErrProtection；本轮没有为了让整行通过而放行data/metadata。正式支付转换器还须组合所有字段审查、来源校验、SQL事务、身份边界和运行hold，不能仅调用单列信封宣称支付域已迁移。

## 验证

人工Go测试覆盖已审查历史联系/账单/provider主体保留、旧地址簿/绑定及默认标记移除、原输入不变、精确保留原始token、未知认证字段/非空metadata/重复键/非法类型拒绝、临时信封往返、错行AAD、错误前缀/编码及正确密封但未投影内容的读取拒绝。

真实旧快照只读审计见[payment-context-review-report.json](payment-context-review-report.json)：21个context全部通过临时新信封往返，9个需剥离旧账号资料；移除customer.addresses和customer.account_holders各9次，账单is_default_billing/is_default_shipping各8次。快照hash固定、输入必须Git忽略，v1 context null不猜测SQL NULL；新密钥/信封不落盘，无业务数据库/provider连接，无源值进入报告。原快照未修改，不证明未来新增context键已审查。

人工MySQL测试`reviewed_payment_context_ascii_envelope_sql_roundtrip`使用与目标完全一致的LONGTEXT定义的最小fixture，验证规范文本信封落库原样读回、历史账单原文保留、旧能力移除、SQL NULL独立及错行AAD拒绝。不是完整payment_session写入，也没有真实session/新密钥持久化。结果在mysql-type-fixture-report.json；单核隔离容器按所有权清理。

```powershell
python retail/scripts/audit-payment-context.py --bundle logs/gin-migration/source-20261026.json
```

后续[完整支付行转换](payment-history-conversion.md)已组合审查后的data/context/metadata，真实v2支付42行及人工完整MySQL行写入/回滚通过；持久化操作hold/正式导入、PayPal嵌套商户配置、历史通知/客户metadata及其他内容仍待承接。所有provider事实/金额/引用及历史孤儿对账继续有效，T006/T018/T035不验收。
