# iphone0902 邮件协议迁移

用户于 2026-11-03 扩充范围：从本机 `iphone0902` 迁入 SMTP、IMAP、Resend 与趣果 Relayme，接入现有 Gin/React/Astro 邮件功能。整体任务仍进行中，不能把发信适配器通过测试等同于后台已经能配置所有渠道。

## 来源及迁移边界

- 发信：`iphone0902/backend/internal/priceupdates/mailer.go` 的 `sendSMTP`、`smtpMessage`、`sendRelayme`，及其 SMTP DATA 接收后 QUIT 断开测试。
- 邮箱配置：原 `internal/models/site_mailbox.go`、`internal/handlers/email_mailboxes.go`、`web/src/pages/system/SiteMailboxes.tsx`、`RelaymeMailboxFields.tsx`。
- 收信：原 `internal/handlers/relayme_callback.go` 的 IMAP TLS/STARTTLS、UIDVALIDITY/UID 定位、RFC822 归档与回调验签；原收件箱、附件、草稿、垃圾箱与投递状态实现需逐项接入当前权限和数据结构。
- 当前 Gin 已有 Resend 通知邮箱、注册/订单/客户业务通知队列，不能用原项目的用户、订单或数据库模型直接替换。

## 已完成：发信协议适配器

`maildelivery.NewSMTP` 和 `NewRelayme` 复用现有准备邮件、业务模板、指纹及请求校验。SMTP 支持 TLS/STARTTLS、认证、显式证书校验设置，发送稳定 Message-ID 及 text/plain、text/html MIME alternatives；服务器对 DATA 返回 250 后，QUIT 断开仍为接收成功。请求受上下文及连接 deadline 限制，不输出凭据、正文或收件人。

Relayme 沿用 `POST /api/external/v1/emails`、`Bearer KeyID.Secret`、HTML 与 headers，以及 `201/object=email/status=accepted/id` 接收契约，保留原网关提交 ID。不会把 Resend 专用字段直接传给该网关。

SMTP 和原 Relayme 接口均不具备已证明的远端幂等承诺；适配器暴露 `SupportsIdempotency=false`，发送失败不自动标记可重试。接入队列前，仍须处理租约恢复和人工重试边界，以及 Relayme 非 UUID 提交 ID；目前后台仍只选择原 Resend。

验证：真实本机 SMTP 协议测试验证 DATA/QUIT 接收边界及 MIME；HTTP 测试服务验证原 Relayme 契约、提交 ID 和失败策略。`go test ./...`、相关包测试与 `go vet ./internal/maildelivery` 通过。无真实邮件发送、无数据库变更，预览三端 HTTP 200。

## 接续必做

后台渠道与加密配置保存、IMAP 收件配置和连接、各渠道通知队列接入、收件/回调/归档及原邮件操作、隔离协议联合验收，最后在用户配置账户并明确提供测试收件地址后验证真实投递。现有发信适配器测试没有证明这些范围完成。

## 已完成：IMAP 读取适配器

沿用原项目 go-imap v1.2.1、TLS/STARTTLS 和邮箱级 SHA256 证书指纹信任方式。新增 `maildelivery.FetchIMAP` 以只读 EXAMINE/BODY.PEEK 获取原始 RFC822、接收时间及 UID，返回 UIDVALIDITY，拒绝静默复用旧 epoch 的游标。分页使用受限 UID 数值窗口并返回 `ScannedThrough`，避免空洞 UID 导致游标永远停滞；调用方须先保存消息再推进游标。单封 26 MiB、单批 32 MiB，最多 100 UID，超时及上下文取消关闭连接。

真实本机 TLS 和 STARTTLS IMAP 服务器测试通过：空洞 UID 分页、原文、接收时间、重复检查点、UIDVALIDITY 变化、读取不设 Seen、错误证书指纹拒绝、无响应连接取消。该适配器没有改动真实邮箱，也尚未接入后台收件同步/归档；上述接续项继续开放。

## 已完成：后台渠道配置与通知选用

第108迁移新增独立加密协议配置表，并扩展订单/业务邮件的渠道提交ID长度。后台“通知邮箱”可选择 Resend、SMTP、趣果 Relayme API，配置发送参数与可选 IMAP TLS/STARTTLS/证书指纹。所有密码/Secret 留空保留，响应仅返回配置标志；凭据、带密钥的操作收据均通过原商户密钥环加密。前端保留既有操作UUID/修订号冲突、丢响应原命令恢复及权限。

注册、订单、客户事件发送器按已选邮箱读取当前渠道；Resend 原实现与重试保持，Relayme 提交ID原样保存。SMTP/Relayme 初次请求后重试或进程恢复进入 `channel_review_required`，阻止无幂等承诺的渠道重复发送；现有失败任务不会显示该状态的重新排队入口。没有修改真实邮箱配置或发送外部邮件。

固定验收 owner `012c2fd998ae`：生产Gin API SHA256 `a541bf1a14aea0b61b8dadfc9247cc45bf74de3df44119487fcb36fb8e7b5dad`，隔离MySQL/Redis/React/Astro/Chromium通过；真实Admin保存三渠道、SMTP/Relayme丢响应精确重放、两种密钥及IMAP密码空白保留、独立Go进程解密/选择发送器、手机表单与秘密不回显通过。原Resend独立进程发送夹具、邮件链接验证注册登录、旧已付订单原outbox失败→人工重试→服务接收均通过；SQL为1注册发送、1订单发送、1重试审计、1批准邮箱、7保存收据。来源账户计数不变、自有资源清理通过。忽略证据位于 `logs/retail-native/account-012c2fd998ae`；协议模拟仅限外部邮件，未声称真实收件投递。

`go test ./...`、相关vet、Admin TypeScript通过；主库显式应用1项/跳过107项迁移，并用原启动脚本更新服务。下一项仍是IMAP同步/归档、收件箱和原收发邮件操作；保存IMAP参数不等于已接通收件。

## 付款子流程的待续证据

本轮真实 Mollie 终止重付探针保留在 Git stash `wip: real Mollie terminal retry probe; method cancellation remains open`，尚未作为完成任务提交。隔离 owner `8d5269b067a0` 的方法取消及返回后，正式渠道 GET 仍为 open；owner `ce7b936bc364` 选择方法 expired 后返回托管支付方式选择页。不能把支付方式结果当作整笔付款终止。所有探针的忽略 SQL/日志保留，报告确认自有资源清理；上一轮人工适配器联合验收仍有效，但真实终止重付未通过。
