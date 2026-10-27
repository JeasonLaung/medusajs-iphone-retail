# SEO、流量归因与引用监测基线

日期：2026-10-27。T001/T002/T004/T005 中 SEO 领域核对；T058/T059 尚未实现。

证据：`medusa-store/apps/backend/src/lib/seo/{core,operations,publication,ingestion,citations,google}.ts`、`api/admin/seo/[resource]/route.ts`、`api/seo-validators.ts`、`jobs/seo-monitor.ts`；前台 `src/lib/data/seo.ts` 与 [7 个 route handler](frontend-routes.md)。下文源码路径均属于相应 backend/storefront。

## 操作拆分

| 操作 | 当前效果与限制 | Gin 迁移要求 |
| --- | --- | --- |
| save settings | key 格式、布尔值、site origin、home path、IndexNow key、Bing XML、GA4 ID、GSC verification 校验；提交时间/检查时间为只读 | 不用任意 key-value 写接口替代校验；密钥字段单独授权 |
| save page-metas | path 规范化；title/description/canonical/OG/robots/hreflang/structured data | 保留页面级覆盖；JSON-LD 输出转义 `<>&`；私有页面强制 noindex,nofollow |
| save providers | 仅 chatgpt/deepseek；responses/chat/chat_input 协议、model、base_url、加密 api_key | 保存配置不自动调用或消费额度；协议能力在适配层实现，不引入 Medusa 依赖 |
| save crawler-sources | 本地绝对日志路径；1—10080 分钟间隔；更换路径重置 offset/指纹/错误 | 路径范围和文件读取权限显式配置；不读取任意上传路径或目录 |
| save citation-monitors | 品牌、问题、provider、1—525600 分钟；enabled 仅显式 true | 新建不默认为开启外部定时调用 |
| delete | 仅可写资源；正在 running 的 monitor 禁止删除 | 关联历史/软删语义明确；后台账号权限，仓库和供应商拒绝 |
| scan | 扫描 enabled 日志源，手动 force 或按间隔 | MySQL 租约/去重及偏移恢复，不在 HTTP 长事务中读取全日志 |
| run | 对单 monitor 调用配置的 provider，记录 run 和 evidence | 并发同 monitor 排他；中断恢复、失败原因安全化 |
| indexnow | 读 sitemap/指定 URL，验证公开 key 文件后提交并记录结果 | 长任务持久化；外部成功与本地记录失败可追踪，不把一次按钮点击当可靠作业 |
| google-test | GA4/GSC 刷新 OAuth 且请求实际 property/resource 报告 | 不只测 token 获取；错误脱敏 |
| tick（内部） | 每分钟扫描、到期 monitor、可选自动 IndexNow | 内部作业入口，不接受公开 action=tick；隔离演练默认停外部作业 |

HTTP validator 接受六种 action，内部另有 tick；resource 五种可写。不能以一个通用 POST 算已迁移全部功能。

## 密钥与公开内容

SEO 密钥为 AES-256-GCM，12-byte IV，base64(iv).base64(tag).base64(ciphertext)，密钥由 SHA-256(SEO_ENCRYPTION_KEY 或 COOKIE_SECRET) 派生；弱默认值拒绝。只读迁移时先核对可解密性，不把明文/密文写入报告或 Git；演练使用隔离测试配置。

settings 密钥返回遮罩，空值/遮罩保留原密钥，clear_secret 才清除；secret 不能直接改为公开字段。provider API key 只返回是否配置。公开 `/store/seo` 是明确 publicKeys 白名单，不包含 OAuth/模型 provider secret；`seo.indexnow_key` 是需要公开验证的站点 key，不能与 API key 误归一类。

前台 title/description/page canonical/hreflang/OG 覆盖默认；语言默认生成 en、zh-CN、x-default。账户、结账、购物车、订单、支付结果、邮箱验证、回收问卷/请求强制不索引。SEO API 故障退默认值，不让商店不可用。

保留 `/robots.txt`、`/sitemap.xml`、`/llms.txt`、`/BingSiteAuth.xml`、`/{indexnow-key}.txt` rewrite 与 `/api/seo-artifacts/[key]`；不能只迁后台设置而丢公开验证资源。公开路径的大小写/Content-Type/缓存语义需逐一对照。

## 日志增量和流量口径

- nginx combined 行解析；爬虫先匹配已知 UA 名单，按 source_id + 原行 SHA-256 去重，IP 保存 hash；并非真实性已验证的机器人身份。
- 普通流量只收 GET/HEAD、200—399，排除 bot/spider/crawler/headless/monitor、API/Admin/框架/静态资源。不是所有 pageview 都计入，**直接流量无归因时不入表**。
- 归因优先 click ID（Meta/TikTok/Google/Microsoft/LinkedIn），再 UTM，再非本站 referrer；compatible UTM 可与 click ID 共存。保留 campaign、platform、source、medium、matched_by。
- traffic 目前保留原 IP，geo lookup 调 country.is；爬虫 IP 则 hash。两者不能在迁移报告混作相同隐私处理或把原 IP 复制到公共数据。
- 每源每轮最多读 4MB，仅推进完整行；半行留到下轮；超大单行报错。第一行指纹 + 文件大小检测替换/截断；source offset、已读行、已匹配计数、错误分别保留。
- source collection flags 分别控制 AI crawler 和 traffic。扫描锁超时/过期 5s/600s，不能把旧 Medusa locking API 原样留在 Gin。
- geo 每次取 25 条缺 location 记录，按 IP 去重、跳过内部地址，每 IP 最多更新 1000 条；失败留待重试。新系统演练禁止发真实历史 IP 到外部服务。

## 引用与外部报表

引用 run 保存 prompt/model、answer、完成/失败、品牌是否在答案中、不重复 URL evidence。provider annotations 为 provider_citation，答案中普通 URL 为 answer_link，不能混为供应商已确认引用；品牌命中当前是不区分大小写的 substring。

同 monitor 加锁；新 run 前将遗留 running 标 failed/interrupted。请求最长 60s，锁过期 180s；错误只留 seo.* 或 HTTP status，不保存 Authorization/provider 原始失败体。新 MySQL worker 以持久租约恢复，避免旧遗留 run 被误当完成；重试外部付费调用须记录尝试。

GA4 请求 activeUsers/newUsers/sessions/keyEvents，日期为 daysAgo—yesterday；GSC 到三天前、连续 days、clicks/impressions 汇总、CTR=clicks/impressions、position 按 impressions 加权。整体天数 1—90，默认 31。连接失败返回 connected=false 和安全错误，不能绘成真实零值。新 Admin 对这些图表也使用 ECharts。

## Sitemap 与 IndexNow

仅同站 origin，无 query/hash；最多遍历 100 个 sitemap、50000 个页面，XML 结构校验。URL 手动提交去重且 1—10000 个；提交前站点 key 文件 HTTP 200 且文本严格匹配。响应 200/202 记 success；错误同样保留 submission，不直接抛失去记录。

自动提交需 enabled + auto_submit；失败/检查按小时限频；按每个 URL 的最后成功提交时间与 lastmod 比较，不能一次提交子集就把全站标已提交，超过 10000 的余项下次仍为 pending。

## 固定验收规格

SEO01 私有页面配置 index 仍 noindex；SEO02 空/遮罩密钥保留、显式清除生效、公开 DTO 无秘密；SEO03 半行、截断、轮转、重复行重跑不重复；SEO04 click-id/UTM/referrer 优先级、无归因不误计；SEO05 同 monitor 并发/中断恢复不双跑；SEO06 provider annotation 与普通 URL 分开、空/不完整答案失败；SEO07 Sitemap 跨域/循环/超限拒绝；SEO08 IndexNow 子集/批次/失败重试不吞余项；SEO09 Google 连接错误和真实零值区分；SEO10 SEO 故障时商城页面仍可用；SEO11 所有外部任务演练关闭、历史运行不自动重放。尚未执行新系统用例。

剩余：流量聚合维度/日期/导出口径、全部公开生成文件正文、实际 UI 各表单/抽屉状态、source symlink 与外部 URL 的新访问边界、provider 编译运行验证。T058/T059 不能勾完成。
