# Windows 通知邮箱与邮件恢复子流程

2026-11-02，`python retail/scripts/test-local-customer-account.py --scenario=mail` 在当前恢复库的独立副本通过。owner 为 a60942464724；Gin 19100、React Admin 19200、Astro 19300 使用专有数据库和 Redis 前缀。运行器为副本生成独立商户加密密钥，不读取历史邮箱私钥、不改变主环境发信开关。

实际管理员浏览器添加通知邮箱并选择注册发信账户。成功响应被丢弃后，界面保留原命令重试；SQL 只有一个邮箱和一条创建回执。编辑时密钥为空，保存后发送器仍读取原加密密钥，目录接口不回显密钥。第二次编辑产生独立回执，总计两条。

390 宽客户页面实际注册。单独 Go 测试进程调用正式注册发送器，从发送正文提取一次性链接；浏览器访问链接、验证、登录并读取订单页。没有直接将客户标记为已验证，也没有通过业务接口返回测试 token。

订单分支使用副本中真实 Mollie 测试订单原有的 order.placed outbox，不新造支付或重新跑完整购买。独立进程调用正式订单发送器，外部传输先返回永久拒绝；实际后台显示 Failed/provider_rejected。管理员确认重新排队，成功响应丢失后使用相同操作 UUID 和请求内容恢复。第二个进程发送完全相同的冻结正文，后台显示 Provider accepted。再次调度不重复发送。该验证使用测试可执行文件调用正式 Store，不是正式 worker CLI 启停验收。

SQL 核对：注册邮件 sent 1、订单邮件 sent 1、重新排队审计 1、已登记邮箱 1、邮箱命令回执 2。浏览器错误 0。源账号/回收/售后十二表计数前后不变；副本、授权、专有进程和 11 个 Redis 键清理完成。报告见 [本轮记录](local-notification-mail-main-report.json)。邮件发送器、客户、邮箱与身份相关 Go 检查通过。

只替换外部 `https://api.resend.com/emails` 的 HTTP transport，其余实际 API、SQL、加密信封和发送器不模拟。测试检查 sender、recipient、幂等 key 和原订单 789 EUR 正文。Resend 凭据、发件人及收件人均为隔离夹具，未连接 Resend、未发送真实邮件。真实渠道认证、域名、收件、退信与 webhook 仍未验收；Provider accepted 不等于收件箱投递。

原 Medusa 的 customer-notifications subscriber 为发货、交付、取消及售后事件创建站内通知；回收处理也写站内通知。本次保留这些原有行为，未将站内通知冒称为邮件。C10 清单中的回收/售后自动发信要求仍须进一步核对和交付，不能因本轮注册与订单验证而关闭。当前真实发信配置缺失，C01—C11 保持开放。
