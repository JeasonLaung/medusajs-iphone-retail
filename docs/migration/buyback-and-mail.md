# 回收与邮件的动作基线

日期：2026-10-27。源码核对成果，关联 T001/T004/T005/T007；尚非实现验收。

## 回收身份与模型

证据：`medusa-store/apps/backend/src/workflows/buyback.ts`、`src/lib/buyback.ts`、`src/api/buyback-validators.ts`。后台访问要求非供应商、非受限仓库的管理员；客户通过 request_id + customer_id 获取自己的申请，不能因知道 ID 或邮箱相同而读取别人的照片。旧账号不迁移，历史申请的原主体标识与必要联系方式属于业务快照，新登录身份单独处理。

创建申请 1—10 台，校验已发布 iPhone 型号和真实容量选项、问卷完整、联系/退回地址及两项明确同意。幂等范围为 customer_id + submission_key，同一提交返回既有申请。逐行生成 item ID、参考报价快照、身份空值与 device_id 空关联，初始 awaiting_photos；缺参考价允许 reference_amount 为 null，不能擅自显示为 0。

## 回收状态转换

| 操作者 / action | 允许原状态 | 结果与约束 |
| --- | --- | --- |
| 客户 `submit_photos` | awaiting_photos | 全部 item 恰好一次提交 SN+15 位 IMEI、申请内均不重复；必需照片齐全且非 retake/expired → photo_review |
| 客户 `mark_shipped` | ready_to_ship | 保存追踪号 → in_transit |
| 客户 `accept_offer` | final_offer | 提交的 final_amount 必须等于当前终价；不接受陈旧报价 → accepted |
| 客户 `decline_offer` | final_offer | → return_requested |
| 客户 `cancel` | awaiting_photos / photo_review / ready_to_ship | → canceled，并记录关闭时间 |
| 管理员 `review_photo` | awaiting_photos / photo_review | 只能审核该类型的最新且未过期照片；retake 必须至少 3 字原因 → awaiting_photos；approved 不自动允许寄出 |
| 管理员 `approve_shipping` | photo_review | 全部必需最新照片 approved，填写寄送说明 → ready_to_ship |
| 管理员 `mark_received` | ready_to_ship / in_transit | → received；允许未由客户标寄出而直接收到 |
| 管理员 `link_device` | received | item 未关联；设备 recycled/inspection，SN/IMEI 与该 item 一致、产品/容量匹配；创建唯一关联，状态不变 |
| 管理员 `final_offer` | received | 每台已关联设备，全部 item 恰好一次有终价和检测说明；精确汇总 → final_offer |
| 管理员 `mark_paid` | accepted | 保存 payment_reference → paid，并关闭；当前是人工付款确认记录，不等于调用在线转账 API |
| 管理员 `request_return` | received / final_offer | 保存原因 → return_requested |
| 管理员 `mark_returning` | return_requested | 保存退回追踪号 → returning |
| 管理员 `mark_returned` | returning | → returned，并关闭 |
| 管理员 `cancel` | awaiting_photos / photo_review / ready_to_ship | 保存原因 → canceled，并关闭 |

状态共 12 种；不同动作可同名但权限、必填参数及状态边不同。每次修改保留 history（时间、动作、平台/客户、状态、原因）。照片改动或设备关联失败的回滚也需承接；Go 应在同一显式事务内完成本地状态、关联及审计。

后台状态变更会产生客户通知；link_device 及照片 approved 不触发该通知，retake 则触发。通知去重与申请历史事件关联，应在持久化 outbox 保留同一事件身份，不使用重试时生成的新时间作为唯一幂等依据。

## 回收照片与保留策略

- 六类：front/back/edges/damage/packaging/sealed。必需 front/back；问卷 frame 为 damaged/visible 时还需 edges，不能要求每台一律传齐六类。
- awaiting_photos 可上传；ready_to_ship 仅 packaging/sealed；每个 item 累计少于 30 条才可继续上传。
- 客户传 base64，后端核对真实 JPEG/PNG/WebP 文件头与声明 MIME，解码不超过 3 MiB。HTTP 接收限制、编码长度限制、解码后限制是不同层，均需保留。
- 文件 private 存储。对外序列化不返回 storage_key；照片下载经过客户归属或后台权限验证。
- 按 item + photo_type 选最新：uploaded_at、同时间再按 ID 排序。重拍不覆盖历史记录，审核和提交只取最新；旧照片不能因新照片被拒而自动重新成为有效版本。
- retentionDays=180；每日 03:00 清理。`expire-buyback-photos` 实际只遍历 closed_at 早于当前时间减 180 天的申请，不按上传时间直接清理仍在处理的申请。先删除私有文件，再清空 storage_key、标记 expired；对象删除失败保留可重试状态，已 expired 跳过。任务时区需在新配置中明确，不凭服务器默认时区推定。

## 回收固定用例

| ID | 用例 |
| --- | --- |
| B01 | 同客户 submission_key 并发创建一次；不同客户不能凭相同 key 读取旧申请 |
| B02 | 未发布型号、不存在容量、无效问卷、缺同意被拒；缺参考报价保持 null |
| B03 | 多台身份漏项/重复 SN 或 IMEI、缺必需照片、最新照片 retake 均拒绝提交 |
| B04 | edges 按 frame 条件必需；只 approved 部分照片不能批准寄出 |
| B05 | 上传伪造 MIME、超解码限额、跨 item/申请、ready_to_ship 非打包照均拒绝 |
| B06 | 原客户和其他客户/仓库/供应商分别访问同一私有照片，后者被拒 |
| B07 | 未关联全部设备不能给终价；设备不匹配型号/容量/身份/状态被拒 |
| B08 | 客户提交过期终价不能接受；未 accepted 不能 mark_paid；重复付款记录不能触发重复转账 |
| B09 | 客户拒绝终价和平台申请退回均可走 returning → returned；终态不能回到编辑流程 |
| B10 | 申请更新、关联、照片审核与通知失败可恢复；同一事件重试不重复通知 |

## 邮件命令与实际提供方

证据：`src/workflows/steps/mail-command.ts`、`process-mail-queue.ts`、`workflows/mail-webhook.ts`、`modules/mail/security.ts`、`api/admin/mail/validators.ts`。实际发信与收件读取使用 Resend，不是通用 SMTP/IMAP 客户端；Gin 迁移应承接已有 API 能力，而非凭“邮箱”菜单假设 SMTP 已实现。

| action / operation | 关键行为及状态限制 |
| --- | --- |
| save_account | API key 和 webhook secret 加密；编辑时空密钥保留原密文；新邮箱缺 API key 拒绝；返回 publicAccount，不能回显密文/明文 |
| delete_account | 有 outbox 或 queued/sending/paused 活动拒绝；否则禁用而非物理删除，以便历史附件与回调继续有凭据 |
| save_draft | 邮箱必须 enabled；已有邮件只允许 draft 编辑；保留附件、回复关联、收件/抄送/密送及创建人 |
| send | draft 完整校验后 → outbox/queued；已经 outbox/sent 返回原 ID，不能重复排队 |
| messages read/unread/star/unstar | 分别更新已读或星标，不借副作用改变投递状态 |
| messages trash/archive/delete | outbox 且非 failed 拒绝；永久删除仅 trash（软删）；保留 previous_folder |
| messages restore | 仅 trash，恢复 previous_folder |
| messages retry | 仅 outbox/failed；first_attempt_at 超 23 小时拒绝，防越过 provider 幂等窗口；重置 attempts/error，但不生成新业务邮件 ID |
| contacts | 批量最多 1000；订阅必须有 consent_source；导入不能静默重新订阅已经退订的人 |
| unsubscribe_contact | 保留退订状态及历史；与公开 token 退订一致处理 |
| save_campaign / campaign | 活动草稿和 queue/pause/resume/retry 是不同命令；收件快照、过滤及状态规则待下轮审查剩余实现 |
| save_template / delete_template | 模板独立于已发送内容快照；输入清理与权限需连同富文本编辑器核查 |

邮件发送队列当前每次最多读取 10 封 queued/sending，单 provider 请求节流约 1 秒、超时 8 秒。成功 → sent 并记录 provider_id；暂时性错误/网络失败且 attempts<3 回 queued，否则 failed。幂等 key `mail/{message_id}`，超过 23 小时的模糊发送不直接重发。注册验证邮件在发送前还要核对验证 token 是否被替换、已验证或 15 分钟过期，不能把旧账号相关队列原样迁入后自动发送。

营销投递的节流、批量限制和租约需继续阅读队列后半段；Redis 丢失不能使已发送事件重新入队。迁移演练不连接真实客户发送通道。

## 回调与收件安全

- 去重 event_id 使用 account_id:event_id，成功处理后才记录，失败允许 provider 重试。
- received 回调读取 provider 实际收件内容，只有收件地址包含该 account.email 才导入，不能把其他邮箱的私有邮件写入本账户。
- 投递状态乱序不回退：失败终态优先，delivered 不回 sent；opened/clicked 另记首次时间。
- bounced/complained/suppressed 活动收件人标记 subscribed=false、suppressed=true；后续导入不能悄悄解除。
- 路由保留 raw body 用于签名验证；具体验签头、时效、禁用邮箱的历史读取及附件授权由 T007/T016 补完整契约，工作流去重不能代替验签。

固定用例 M01—M08：密钥不回显；忙邮箱禁删；重复 send 不重复排队；失败重试保留 key 并拒绝过期模糊发送；退订不被导入恢复；回调乱序不降级；跨邮箱 received 不导入；验签失败与重复事件不触发收件/发信副作用。
