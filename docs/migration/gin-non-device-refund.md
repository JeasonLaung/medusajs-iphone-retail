# 无序列号申请的原付款退款

2026-11-03，补 C09 未发货商品与不登记序列号商品的退款分支。复用客户售后、后台审核和原退款指令/账务/通知流程，不新增数据库迁移。C01—C11 整体仍未完成。

原站 `medusa-store/apps/backend/src/workflows/create-after-sales.ts` 允许未发货的序列号管理商品不选设备申请退货，也允许不管理序列号的商品申请退货。`device-refund.ts` 将无设备申请交给原生订单退款。新 Gin 原退款参与者要求每行数量等于设备数，因此这些合法申请无法提交退款指令。

现在无设备的 return 行从当前订单版本读取真实商品行与精确数量，检查申请数量；序列号管理商品仅允许未发货部分。已发货设备证据仍存在时，关闭当前 SKU 跟踪也不能绕过收机。仍有设备选择的行继续要求原订单、原商品、原售后对应的实际已退回设备；无设备 exchange 不进入本分支。重复商品行、错行、零数量、超量及非 processing 申请拒绝。退款准备和首次发往渠道之前均重新验证，以阻止申请获批后才发货的情况。

退款继续由原加密 CREATE 请求、原付款及可退实收绑定，使用现有幂等指令、后台 worker、原账本和客户通知。该财务操作不会自动收货、取消包裹或释放库存预留，后台中英文提示已说明；不增加独立退款 Key 或更换商户密钥。售后完成条件沿用原站规则。

## 固定检查点验收

```powershell
go -C retail/backend build -o ../../logs/retail-native/retail-api-non-device-refund.exe ./cmd/api
$env:ACCOUNT_TEST_API_BINARY = (Resolve-Path logs/retail-native/retail-api-non-device-refund.exe).Path
python retail/scripts/test-local-customer-account.py --non-device-refund-from-checkpoint=790ce0fc9e71
```

最终独立 owner `a840e5940b0e` 从既有真实 Mollie paid 检查点复制恢复，不重新购物或付款。实际 Astro 手机页面提交无设备退货，实际 Ant 页面审核并提交 789.00 EUR 退款；客户申请与退款 POST 分别丢失首个响应，再以完全相同命令恢复。退款记账后后台完成售后，客户中英文手机详情均读取到唯一退款时间线和三条准确业务通知，无页面脚本错误/横向溢出。

仅本轮外部退款适配器返回人工 refunded，**没有向 Mollie 创建新的远程退款**，不能将本报告当作真实渠道退款验收。Gin 业务接口、当前权限、原加密付款读取、MySQL 事务、持久指令、账务和客户通知均走正式实现。既有真实渠道 pending→refunded 验收另见 [Windows Mollie 退货退款](local-refund-main.md)。

同一副本内 14 组外层回滚检查覆盖跟踪/不跟踪、发货/取消包裹、关闭跟踪后仍有设备分配、无包裹设备证据、错行/重复/超量/零数量、换机与设备收机要求及状态变化。另在浏览器已提交的真实指令之后补入发货事实，首次执行拒绝且渠道调用为零。撤去该测试事实后，通知写入故障使退款/负向交易/汇总整体回滚；原渠道观察独立持久化，六并发正式 Run 恢复均成功，累计一笔人工渠道 POST、零次额外渠道查询，只有一笔退款、负向交易和已记账指令。

副本库存、预留、设备、包裹及退货表前后指纹一致。主库 29 张商品/仓库/设备表指纹与 12 项客户业务计数均不变；专属数据库/授权、8 条 Redis 键和服务进程已清理，忽略目录保留持久 SQL。前两次夹具准备错误（已有跟踪行、普通账号创建 trigger 的权限）及首次配置缺少 checkout adapter 导致的启动拒绝已修正；不把这些失败轮算作通过。

Go 全包测试及退款 vet、Admin TypeScript、Python/JS 语法及 diff 检查通过。原生主预览恢复和当前健康结果见 [报告](non-device-refund-report.json)。本项只关闭该退款分支，真实支付其他渠道、邮件实际收件、正式数据关系核对与本期整站联合验收继续。
