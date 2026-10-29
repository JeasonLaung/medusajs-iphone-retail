# 新后台通知邮箱与注册验证发信

承接原 `admin/routes/settings/mailboxes/page.tsx` 的邮箱名称、发信地址、回复地址、启停及密钥留空保留语义；注册发信来源为原 `backend/src/lib/registration-email.ts`。新 React Admin 使用现有 Ant Design 表格、抽屉、表单、复选框、密码输入及反馈组件，沿用项目主题，提供中英界面。表单结构参考 [Form](https://ant.design/components/form/) 与 [Drawer](https://ant.design/components/drawer/)；实现使用项目已安装的 Ant Design 5。

0041 将此前仅存在于离线字段计划的 `mail_accounts` 正式建表，字段与原映射一致，复用原 `merchant-config/mail_accounts/{column}/{id}` AES-GCM 密钥上下文和 email 完整值唯一性注册表。显式 CREATE 拒绝未经核对的同名表。其他离线商户配置表不在本迁移内。

`notification_mail_accounts` 是本期通知用途的明确登记，导入历史记录不会自动登记或激活。存在 `migration_merchant_dispositions` 的邮箱即使误加登记也拒绝编辑和发信；历史配置解除 hold 仍需独立核对，本接口不实施解除。当前界面只列登记的通知账户，不提供历史邮箱编辑、收件箱、通讯录或营销功能。

权限增加 `mail.read`、`mail.write`。已有角色不自动获得新权限；权限管理员可通过原角色管理界面授予。新建系统管理员的显式 Bootstrap 包含这两个动作。权限动作总数为 13，模型 SHA256 是按字节排序的动作名、逐行 LF 且末尾 LF 的 SHA256：`d7f0a6b11cefe0238216f4705112a166f433710d19a09ed5db40f69c876dd9b4`。仓库/供应商不能使用全店邮箱配置接口。

GET/POST `/api/admin/notification-mail/accounts` 使用独立管理员 Redis 会话与原 Origin/CSRF 中间件。POST 严格检查字段、类型、重复键、16 KiB 大小和全局 revision。权限先在事务中复核，再锁配置行并写邮箱、完整地址唯一性 claim、默认选择、回执和脱敏审计；失败整体回滚。相同操作 UUID 与完全相同内容重试返回原账户及修订，不重复新建；更改内容/主体拒绝。回执里的完整请求独立加密，不把发信 key 写入审计或接口响应。

后台入口 `/mailboxes` 支持添加、编辑、启停及指定注册验证账户。邮箱 API Key 与 Webhook Secret 保存后只显示是否配置，编辑时始终为空，空值保留原密钥。保存结果未确认时保留本次操作和内容，允许原样重试；冲突和无权限要求关闭并重新读取。注册账户必须启用；停用时会取消它的明确注册选择。与原行为一致，未明确选择时使用最早启用的通知账户。通知登记上限为 100 个。

API 配置需显式 `NOTIFICATION_MAIL_ENABLED=true` 和 `MERCHANT_KEY_ACTIVE`/`MERCHANT_KEYRING_JSON`。密钥仅在 Gin/worker 侧使用；注册客户意图继续使用独立 `CUSTOMER_KEY_*`。worker 仍需显式 `CUSTOMER_VERIFICATION_MAIL_ENABLED=true`，API 启动不会发送邮件。`REGISTRATION_MAIL_SOURCE=database` 让 worker 每轮重新加载明确选择或最早启用的通知账户，不回落到历史导入邮箱。`environment` 保留之前的显式环境发信设置；未知 source 拒绝启动。

配置更新后，正在途中的发送可能已经发生，不能撤回。已有重试继续受原 sender fingerprint 保护：密钥/名称/回复地址改变后，旧请求不能用新身份重发，应由客户重新申请验证。邮件任务状态查看和运营恢复界面仍未完成；Webhook Secret 在此仅保存，不代表 Resend 邮件事件 webhook 已接入。

验证证据见 [客户与通知隔离报告](gin-customer-accounts-report.json) 和 [后台浏览器报告](gin-notification-mail-report.json)。独占 MySQL/Redis 覆盖真实管理员 HTTP、授权撤销、地址冲突、8 次并发同操作、秘密加密、回执重放及审计写入失败回滚，并实际串联后台保存的邮箱→客户注册入队→合成 sender→一次性验证→登录。浏览器使用真实 React Admin 和合成 API，独立检查表单、失败重试、空密钥、刷新、中英及移动布局；不冒充真实 Gin/MySQL 浏览器联合验收。中英页面通过持久语言偏好分别核对；本夹具在 reduced-motion 下切换语言的下拉定位偶发出屏，语言选择器本次不验收，后续前台/后台联合回归需继续核对。未调用外部 Resend、未使用真实密钥或测试真实收件。订单/回收/售后通知、任务恢复界面和全站闭环仍待完成。
