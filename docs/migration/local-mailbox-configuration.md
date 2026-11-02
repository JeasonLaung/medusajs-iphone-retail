# Windows 通知邮箱配置入口

2026-11-02，当前本机NOTIFICATION_MAIL_ENABLED关闭、MERCHANT keyring缺失，后台通知邮箱及订单邮件任务API503。已有Ant/Gin保存与任务恢复功能无需重写，本批开启其实际本机入口。

入口为 `http://127.0.0.1:9200/mailboxes`。通知邮箱中“添加邮箱”可填写发信名称、发信邮箱、回复邮箱、Resend API Key，选择用于注册验证。密钥保存加密，后续读取只返回是否已配置；编辑时密钥留空保留原值。主库仍没有通知邮箱，也未填写虚构Resend凭据。原待发送订单邮件1条、attempts0，不显示已发送。

本机忽略.env生成新的独立持久MERCHANT keyring，完整既有keyring会保留，部分缺失配置拒绝覆盖。它用于新商户/通知邮箱配置，不是旧备份私钥、不尝试解密历史持有账户。启用NOTIFICATION_MAIL_ENABLED和LOCAL_PREVIEW_NOTIFICATION_MAIL_ENABLED；start.bat调用 `configure-local-preview --notification-mail`，限定既定development/127.0.0.1:13307/eurorefurb_local_trial，要求显式API开关、有效keyring与现有正revision设置。仅检查，不创建邮箱、不改变设置revision、不启动发信worker。错误环境、缺keyring、API关闭、错误数据库四项实际命令检查均拒绝。

主注册/订单邮件worker仍关闭，没有真实Resend key/from/收件人验收，也没有实际外发邮件。保存邮箱的“启用发信”是发件账户状态，独立发送worker仍需显式运行配置；不能将填好表单或任务pending称作已送达。后续真实渠道验证须核对提供方接受与最终收件，回收/售后原站内通知也不能冒称邮件。

新增只读脚本（主原生预览、空通知邮箱目录的启动验收）：

```powershell
cd retail/admin
node tests/local-mailbox-read.live.mjs
```

该脚本使用本机专用管理员正常challenge登录，中英/1440和390宽读取实际邮箱目录与邮件任务、打开空密钥编辑器并取消。实际API200/private-no-store、pending任务attempts0、无横向溢出，邮件写请求0、浏览器错误0；不保存邮箱、不重新排队。初轮语言初始化和Drawer工具栏定位不正确导致脚本失败，修正测试后通过，没有为测试改动界面。证据保留于忽略的 `logs/retail-native/mailbox-read`。

同时重跑 `--scenario=mail` 固定隔离子流程，owner61412cfc36dd：实际Ant保存新邮箱及密钥留空保留，注册邮件正文中的链接验证登录，原订单队列永久拒绝、管理员同指令丢响应恢复、独立进程使用原冻结正文再次发送，唯一注册sent1/订单sent1/重试审计1/邮箱1/邮箱回执2；11条Redis键及副本/授权/进程清理，源12表仍0。仅Resend外部HTTP transport使用替身，未发送真实邮件，不能把sent当成实际收件。

最初副本API因继承主CHECKOUT_CONTEXT_PAYMENT_ENABLED=true而普通checkout关闭，配置检查拒绝启动；运行器现显式将该开关默认false，仅付款上下文场景自行启用，防止邮件/账户/回收子流程依赖无关付款配置。原失败日志保留，不放宽生产配置检查。此修正不修改主付款开关。

Go通知邮箱/加密包及配置命令测试/vet、JS/Python/PowerShell语法检查通过。主API9100/后台9200/账户订单及商店9300正常，补款/退款worker仍各1；邮件worker0，主邮箱0/待发送订单任务1，其他交易库存未改。详见 [本机只读与隔离写入证据](local-mailbox-configuration-report.json)。真实发信配置、worker实际投递及C10其他缺口继续开放，不关闭C01—C11。
