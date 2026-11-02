# 退款完成后的业务邮件闭环

2026-11-03，C09/C10新增固定检查点入口，复用真实Mollie测试退款完成本地记账→事件入队→投递失败→后台重试→售后完成邮件。C01—C11仍未完成。

```powershell
python retail/scripts/test-local-customer-account.py --refund-mail-from-checkpoint=d996b8c7712f
```

入口只接受已有付款、已验证客户归属及已发送且尚未记账的退款检查点，必须有原远端退款ID和发送时间。恢复独立数据库和Redis前缀，补迁移105，启动正式Gin、Astro、React Admin及退款worker；复用现有恢复接口，不重新购买、分配设备、发货、创建退款，也不手工修改渠道状态。完整私有SQL保存在忽略日志目录，清理只删除本轮临时资源。

## 验收证据

最终owner `ea172a00f5ad`，源pending检查点 `d996b8c7712f`。第一轮 `22aaf10818e7` 页面与邮件通过，第二轮增加原付款/库存/发货/归属指纹检查并通过。公开摘要见[验收报告](refund-mail-native-report.json)，原始报告、投递日志、截图和最终SQL保存在 `logs/retail-native/account-ea172a00f5ad/`。

- 原退款 `re_kM2DLostBExnPFCouZzYJ` 由正式worker认证查询为refunded；原789 EUR指令、退款账本、负向交易、汇总及已读客户通知各一条。后台完成售后，客户查询到退款与completed状态。
- 105迁移后的正式退款记账发出 `refund.created`，后台售后完成发出 `after_sales.completed`；SQL和后台查询均确认各一封sent，无新shipment/processing事件。
- 复用正式投递器，仅Resend外部HTTP传输为隔离模拟，收件人限制example.invalid；六并发领取仅一次POST，首次明确拒绝。实际Ant后台重试时丢弃首次成功响应，重试仍提交同一UUID/版本，第二次投递请求字节和幂等key保持；随后另一封邮件发送完成。
- SQL确认两条sent投递收据及一条后台重试审计；后台不返回冻结信封、发件人/收件人指纹或渠道标识。
- 原payments/payment_captures、库存/预留、设备分配、客户归属及发货/发货项数据指纹前后相同；原主预览订单无退款，主库账号/售后计数不变。独立库/授权、Redis和进程已清理，最终SQL保留。

本次未重新覆盖既有收件归属、发件配置变化与过期防护矩阵（参见[业务邮件投递](gin-customer-event-mail.md)），未验证真实Resend送达。付款和退款使用本机同一Mollie测试key；主预览无新增退款或邮件发送。主Gin 9100 health、React Admin 9200及Astro 9300商品列表均200，近期日志无新增错误。旧Medusa 9001 health/app仍无监听，属于已停用旧服务。

取消、拒绝、签收及回收退回等邮件联合验收、真实邮件回执、正式数据迁移和整期全流程仍待；本条证据不关闭C10或整体交付。
