# 回收、邮件、SEO 与通知字段设计

第五批：24 表、308 列，累计 147 表、1,583 列；其余 46 表、378 列尚待逐字段处理。这里只是设计和固定快照核对，没有执行 MySQL 导入或业务验收。

生成器 `retail/scripts/map-operations-fields.py` 明确列出每表全部列，未知列和缺列都会失败。JSON/CSV 保存字段表达式、目标类型、空值策略、语义引用和断言，并关联既有源约束、索引清单；不读取业务行或密钥。快照预检只输出聚合数，不输出客户、邮箱、路径、密钥或行 ID。

## 领域承接

| 来源 | 目标与转换要求 |
| --- | --- |
| buyback_request / price | 保留申请、联系方式、退回地址、问卷/报价快照、历史、关闭时间及所有状态；金额是精确主币单位，不乘除 100；未报价 NULL 保持 NULL |
| buyback_photo / device_link | 保留原 ID、私有文件位置、审核及设备关系；item_id 属于 request.items 内嵌项，不能虚构一张商品行表或错接 order_line_item；请求与项归属需联合验证 |
| mail_account / template | 保留业务邮箱及模板，凭据单独解密转新信封；空编辑保留凭据的行为继续由命令契约控制 |
| mail_message / campaign / delivery / event | 保留方向、文件夹/原文件夹、收发正文、附件、provider 关联、尝试次数/首试时间及历史去重身份；历史状态原样留存，另设持久 migration hold，不通过改成 sent 或 draft 掩盖未完成工作 |
| mail_contact | 保留订阅同意、退订、抑制和退订能力；不能因重新导入或新账号同邮箱而重新订阅；unsubscribe_token 是业务退订能力，区别于不迁移的登录 token，并受保护保存 |
| seo_page / setting / provider | 保留公开路径与元数据、设置和提供方配置；公开白名单及密钥遮罩继续适用；对外白名单读取时解密非秘密配置，不因内部加密丢失站点设置 |
| seo_source / crawler_visit / traffic_visit | 保留采集游标、指纹、统计、去重身份及访问历史；read_offset 精确小数存储但执行时必须非负整数；traffic 原 IP 与 crawler IP hash 是不同字段，均不进入公共 DTO |
| seo_monitor / run / evidence / submission | 保留问题、模型、回答、状态、来源证据和提交结果；running 不能冒充成功，不自动重放外部调用；路径更换不能继续使用旧偏移 |
| product_review | 保留审核状态、作者显示快照、评分、商品及历史客户/订单引用；verified_purchase 仍需核对真实购买关系；不能把旧评论所有权授给同邮箱的新账号 |
| customer_notice / webhook_endpoint | 客户通知保留旧主体及事件去重身份，不变成新用户收件箱；外部通知地址也可能携带 capability，URL 与 secret/token 一并受保护，不在导入时试发 |

历史客户仅引用 `historical_customer_subjects`；created_by 是旧 actor 或系统哨兵，不能指向新登录表。邮件 provider_id 是 Resend 外部 ID，不是 Medusa payment_provider 外键。通知 resource_id 和 SEO provider 文本不盲目根据后缀建立错误外键。

## 密钥、旧身份邮件及任务

源码依据：`modules/mail/security.ts`、`lib/seo/core.ts`、`modules/webhook-notifications/security.ts`（均在旧 backend/src 下）。三域都使用 AES-256-GCM，以各域环境密钥或 COOKIE_SECRET 派生；必须按各自配置识别，不能假定可互换。SEO setting 只有 is_secret 的源值需旧解密，普通值保持明文语义；新库统一密封存放，公开读取仍按白名单。字段预检仅核对源类型；后续[离线密钥核对](protected-config-baseline.md)已验证固定快照邮件/通知非空值和Mollie共3项解密及临时AAD往返，尚无持久新信封/轮换/SQL导入证明，SEO空表不算真实域验收。

旧注册验证邮件由 `created_by=registration-verification` 标识，队列源码还关联 auth verification、15 分钟有效期和 token hash。新[历史邮件入口](history-mail-conversion.md)已将固定快照2行的认证正文/error/附件载荷移除并返回永久禁重放处置，普通Go整行/商户入口拒绝绕过；原status和投递时间保留。未持久化转换行或禁令，也未实现worker执行；不是所有任意正文/附件的认证能力扫描。旧auth数据仍不恢复。

其他 queued/sending/paused 邮件、SEO 日志源/监测和外部通知均由独立 migration hold 阻止演练副作用；保留原 enabled/status 供审计。邮件首试时间和 provider 幂等窗口不得重新计时。文件清单还需验证存在、大小、SHA-256、私有存储及权限，不用数据库 storage_key 存在代替文件验收。

## 唯一性与验证范围

源索引涉及：回收 customer+submission_key、报价 product+capacity、关联 item/device；邮件 account.email、account+provider_id、contact.email/退订 token、campaign+contact、event_id；SEO path/key/provider/event_hash；评论 product+customer（仅未软删）、通知 event_key。MySQL 必须分别复现源软删谓词、NULL 和大小写语义，不能直接照搬全部列的全局 UNIQUE。长文本索引键及密封退订 token 的查询索引须另行设计，不能截断到任意 191 字符或使用不唯一的前缀索引宣称等价。

固定快照预检 802 字段值、61 raw 配对、72 引用无失败；自定义语义检查 10 项无失败，发现 2 封旧注册验证邮件需禁止重放、6 张私有照片需文件校验。7 个人工反例核对跨申请照片、重复内嵌项、关闭时间、订阅抑制、队列 hold、精确偏移及评分。空表和这些子集检查不等于回收命令、邮件投递、SEO 采集或权限已实现。此前交易引用和库存差异仍未解决。
