# 后台邮箱

邮件入口位于后台顶部，显示收件箱未读数量，每 30 秒刷新。点击进入 `/app/mail`。邮箱使用独立的三栏布局，支持简体中文、英文及手机布局。

## 功能

- 多个 Resend 邮箱；设置 → 邮箱管理，配置发件名称、地址、回复地址、API 密钥和 Webhook 签名密钥。
- 收件箱、草稿箱、待发送、已发送、归档、垃圾箱；搜索、分页、星标、已读状态和批量操作。
- 写信、回复、回复全部、转发正文；To/Cc/Bcc、附件、草稿自动保存、纯文本和 HTML、隔离预览、可重复使用的模板。
- CSV 联系人导入和订阅同意来源记录；导入不会重新订阅已退订的联系人。
- 营销活动逐人发送，支持全部符合条件的订阅者或通讯录中所选联系人；暂停、继续、失败重试和逐人投递明细。
- 自动退订链接、RFC 8058 单击退订、退信/投诉禁止投递；送达、打开和点击事件记录。

当前使用后台人员共享的邮箱，沿用现有管理员认证；供应商账号由项目现有权限中间件拒绝访问。未迁入参考项目的 SMTP、Relayme、业务员邮箱分配、文件管理器和询盘集成。附件直接上传，转发来信附件需下载后重新附加。

## Resend 配置

1. 后端配置 `MAIL_PUBLIC_URL` 为可公开访问的 HTTPS 后端源地址，例如 `https://api.example.com`。该地址用于 Webhook 和退订，不能设成前端网站地址。
2. 使用稳定的 `MAIL_ENCRYPTION_KEY` 加密数据库凭据；未配置时使用现有安全的 `COOKIE_SECRET`。不要更换正在使用的密钥，否则之前保存的凭据无法解密。示例密钥 `supersecret` 会被拒绝。
3. 在后台添加邮箱和 Resend API 密钥。API 密钥需允许发送及读取收件邮件。
4. 在 Resend 验证发件域名，为收件域名启用 receiving，并依照 Resend 的 DNS 指引配置记录。
5. 在 Resend 为每个邮箱创建 Webhook，回调地址为设置页展示的 `/mail/webhooks/{account_id}`；订阅 `email.received`、`email.sent`、`email.delivered`、`email.delivery_delayed`、`email.failed`、`email.bounced`、`email.complained`、`email.suppressed`，需要统计时同时订阅 `email.opened` 和 `email.clicked`。
6. 将该 Webhook 的 `whsec_…` 签名密钥保存到对应邮箱。后台通过原始请求体验证签名，失败返回 401；处理失败返回错误，让 Resend 重试。
7. 点击「编写测试邮件」，填写由你控制的收件地址，发送后检查「待发送」「已发送」与投递事件，再从外部邮箱回复检查收件和角标。

HTML 阅读器使用沙箱和 CSP，默认阻止外部追踪图片，用户可手动加载图片。凭据不会返回前端，Webhook 事件仅保存必要的标识和事件信息。

## 发送队列

普通邮件和营销投递均保存在数据库中，Medusa 定时任务每分钟处理最多 10 封，服务重启后继续。单个请求使用 8 秒超时，调用间隔 1 秒；网络或限流错误最多自动尝试 3 次。生产环境需运行 Medusa 的任务执行进程，并按所用 Resend 计划调整容量。

每封邮件使用固定 Resend 幂等键；由于服务商的幂等有效期为 24 小时，本实现超过首次尝试后 23 小时会停止自动发送及安全重试，避免重复邮件。此时需要先在 Resend 核对实际投递结果。单次邮件合计最多 50 个 To/Cc/Bcc 地址；营销邮件对每个联系人单独发送。

营销活动只选择已订阅且未禁止投递的联系人，并在实际投递前再次检查订阅状态。退订页面 GET 展示确认，POST 退订，避免链接扫描器误退订。邮件内包含中英双语退订链接及单击退订头。

「已发送」表示服务商接收；「已送达」依赖回调确认。打开、点击统计也依赖 Resend 域名追踪设置和回调，不保证每个邮件客户端都提供事件。

## 数据库与验证

模块：`mail`。表：`mail_account`、`mail_message`、`mail_contact`、`mail_campaign`、`mail_delivery`、`mail_event`、`mail_template`。新增迁移由 Medusa 生成，应用方式：

```powershell
pnpm exec medusa db:migrate
pnpm run build
pnpm exec tsc -p src/admin/tsconfig.json
$env:TEST_TYPE='unit'
pnpm exec jest --runInBand src/__tests__/mail.unit.spec.ts
```

本地端到端检查：先启动一个测试后台，再设 `MAIL_TEST_URL`（默认 `http://127.0.0.1:9002`），执行：

```powershell
pnpm exec medusa exec ./src/scripts/check-mail.ts
```

检查脚本创建独立的 `.invalid` 域名测试记录、模拟 Resend API，并在 finally 删除测试数据。需已有后台管理员；执行队列测试前会确认没有其他待发送邮件和活动。可选设置 `MAIL_PLAYWRIGHT_MODULE` 为 Playwright 模块路径、`MAIL_QA_DIR` 为截图输出目录，以检查真实浏览器中的中英文界面、设置页、草稿自动保存及移动端。

常用接口：`GET /admin/mail?resource=summary|messages|accounts|contacts|campaigns|deliveries|templates|events`；`POST /admin/mail/commands`；`GET /admin/mail/messages/{id}`；`GET /admin/mail/messages/{id}/attachments/{attachment_id}`。所有后台接口均要求管理员认证。
