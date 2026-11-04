# 后台邮箱

邮件入口位于后台顶部，显示收件箱未读数量，每 30 秒刷新。点击进入 `/app/mail`。邮箱使用独立的三栏布局，支持简体中文、英文及手机布局。

## 功能

- 多个 Resend / Relayme 邮箱；设置 → 邮箱管理，选择服务商并配置发件名称、地址、回复地址和凭据。已有邮箱默认继续使用 Resend，不允许直接切换服务商。
- 收件箱、草稿箱、待发送、已发送、归档、垃圾箱；搜索、分页、星标、已读状态和批量操作。
- 写信、回复、回复全部、转发正文；To/Cc/Bcc、附件、草稿自动保存、纯文本和 HTML、隔离预览、可重复使用的模板。
- CSV 联系人导入和订阅同意来源记录；导入不会重新订阅已退订的联系人。
- 营销活动逐人发送，支持全部符合条件的订阅者或通讯录中所选联系人；暂停、继续、失败重试和逐人投递明细。
- 自动退订链接、RFC 8058 单击退订、退信/投诉禁止投递；送达、打开和点击事件记录。

当前使用后台人员共享的邮箱，沿用现有管理员认证；供应商账号由项目现有权限中间件拒绝访问。未迁入参考项目的 SMTP、业务员邮箱分配、文件管理器和询盘集成。附件直接上传，转发来信附件需下载后重新附加。

## Relayme 配置

1. 设置 → 邮箱管理 → 添加邮箱，服务商选择 Relayme。填写实际邮箱地址、Postmaster HTTP/HTTPS 根地址、Key ID 和客户端密钥。完整 API Key 自动由 `<key_id>.<secret>` 组成，客户端密钥同时用于回调验签。客户端密钥和 IMAP 密码沿用 AES-GCM 加密存储，编辑时留空保留，不会返回前端。后台需要设置安全的 `MAIL_ENCRYPTION_KEY` 或 `COOKIE_SECRET`。
2. Relayme 客户端启用 API 和发信 API 权限，发件地址需要 Gateway 授权。保存后，设置页分别显示两个完整公网回调地址。路径与 iphone0902 一致：收件 `POST /api/callback/quguo_mail`；投递通知 `POST /api/callback/quguo_notify`。配置 `MAIL_PUBLIC_URL` 为本系统的公网 HTTPS 后端根地址，不要将另一个项目的域名填入本系统。
3. 完整收件通过 IMAP 读取正文、HTML、Message-ID、回复地址和附件，需配置 IMAP 主机、端口、用户名、密码及 TLS/STARTTLS。使用系统 CA 和主机名校验证书，不支持跳过 TLS 验证。采用只读选取和 PEEK 下载，不改变远端已读状态。缺少 IMAP 配置时收件回调返回错误以便重试，不会把通知伪装成完整来信。
4. 收件通知没有 UID 或 Message-ID，因此用原始邮箱 `username` 确认账号，再用发件人、收件人、已解码主题、服务器到达时间（前后 30 分钟）匹配。候选不唯一时返回错误，不猜测正文归属。最多检查 200 封候选、25 MiB 原始邮件；单次 IMAP 读取总时限 25 秒。匹配结果用账号、文件夹、UIDVALIDITY 和 UID 的摘要作为持久化邮件标识。正文和附件保存在现有邮件表，沿用管理员权限及 HTML 沙箱预览。
5. 回调先读取原始字节，再用 `X-Relayme-Key-ID` 找到启用邮箱的客户端密钥。验证 `X-Relayme-Timestamp`、`X-Relayme-Nonce`、`X-Relayme-Signature`：五行 HMAC-SHA256 原文、UTF-8 密钥、原始百分号编码路径（排除查询参数）、5 分钟时间偏差、无填充 Base64URL 和常量时间比较。验签通过后才解析 JSON，并要求 `X-Relayme-Event-ID` 与 `event_id` 一致。
6. 两个回调共享现有数据库锁；处理成功后持久化事件去重，重复事件返回 204，失败返回非 2xx 由 Relayme 重试。同一个 Key ID 可用于多个本地邮箱，每个邮箱分别保存客户端密钥；处理范围仅限该密钥验签通过的启用邮箱。未配置或不匹配的邮箱返回错误。
7. 发送支持 To/Cc/Bcc、回复地址、纯文本/HTML、自定义回复头和 `filename` + Base64 `content` 附件。201 必须包含 `id`、`object=email`、`status=accepted`、`delivery_status=awaiting_dsn`；`id` 保存为 submission ID。营销邮件沿用逐人投递和退订头。
8. 投递通知按账号、submission ID 和实际收件人关联，保存每人的结果、SMTP 响应码、增强状态码和诊断信息，可在「投递事件」查看。全部收件人成功才显示已送达；任一收件人失败则显示失败；delayed 不会覆盖已有终态。
9. 文档未承诺发送幂等键：Relayme 不自动重试提交。超时、服务端错误、无效受理响应，以及进程中断遗留的发送中记录，标记 `relayme_submission_unknown` 并阻止直接重试。先去 Relayme 核对实际发送结果再决定是否新建邮件。已受理邮件的投递失败也不能通过营销重试重复提交。

本地自动验证：`$env:TEST_TYPE='unit'; pnpm exec jest --runInBand src/__tests__/mail.unit.spec.ts src/__tests__/relayme.unit.spec.ts src/__tests__/registration-email.unit.spec.ts`。覆盖签名、原始字节、路径、时间窗、事件一致性、IMAP 歧义、跨账号/收件人隔离、去重和提交不明保护。真实收发还需要实际 Postmaster 和 IMAP 配置及公网回调。

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
