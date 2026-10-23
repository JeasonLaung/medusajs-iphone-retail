# SEO / GEO 迁移分析与验收

更新时间：2026-10-23。参考项目：`C:/Users/Administrator/Desktop/other/iphone0902/backend`。目标项目：当前 `medusa-trial`。

## 迁移范围与结论

迁移的是旧项目的 SEO/GEO 业务功能，使用 Medusa 自定义模块、工作流、受保护 Admin API 和定时任务。后台直接迁入参考项目的 Ant Design / ECharts 页面组件，保留 Medusa 导航壳和当前商城内容、购买/回收流程与样式。后台入口为 `http://localhost:9001/app/seo`，支持英文、简体中文及窄屏。

旧数据库的历史访问记录、运行记录和生产凭据没有自动复制；旧项目只读分析。当前没有配置的 Google/AI 服务显示“未连接”或缺少配置，不以演示数据冒充真实结果。

## 源码分析与功能对照

### 参考界面与全部图表

`backend/src/admin/components/seo-reference` 直接迁入参考项目的 Operations、TrafficSources、CrawlerLogs、Providers、CitationMonitor、DataSourceConfig、NginxLogSourceDialog、PageMetas、SEOSettings 与 ResizableTable。菜单采用参考结构：SEO/GEO 总览、爬虫访问、流量来源、AI 服务商、引用监测；配置和日志源保留弹窗操作，不另拆成十二个页面。

全部 6 个 ECharts 图表实例保留：总览 GA4 四指标趋势、总览流量趋势、流量来源每日趋势，以及来源、平台、国家三张环形图。保留图例、提示、百分比、筛选、日期范围及响应式尺寸；Search Console 保留参考的每日可排序表格。路径排行保留每页十条、来源筛选、搜索与完整统计总量。

适配层 `client.ts` 使用当前 Medusa SDK 会话，将旧接口映射至读取 API 与写入工作流，转换字符串 ID、时间与分页格式。密钥不回显；引用证据区分平台引用与正文链接。数据源弹窗补入站点设置、页面 SEO、高级设置和提交历史，保证迁移功能可操作。OG 图片选择使用当前 Medusa 上传与 URL 输入；旧独立文件系统管理器不作为 SEO 模块复制。样式限定在 SEO 容器，未全站引入 Ant Design 重置。

图表验证使用受控数据覆盖完整系列与交互；另以授权账号读取真实接口验证页面。没有真实 Google/AI 凭据时，真实页面显示未连接，受控数据仅存在于浏览器验证拦截中。

| 功能 | 旧项目实现 | 当前实现与闭环 |
| --- | --- | --- |
| 统一设置、密钥维护 | `internal/handlers/seo.go`、Admin Settings | 后台配置 → 工作流校验 → `seo_setting` 持久化；密钥 AES-256-GCM 加密、读回脱敏；空白保留、明确勾选才清除 |
| 页面 SEO | `SEOPageMeta`、PageMetas 管理页 | 路径、标题、描述、canonical、OG 图片、hreflang、JSON-LD、robots → 公开白名单 API → Next.js 真实服务端 HTML |
| robots / llms / Bing / IndexNow key | 旧构建流程生成静态资源 | 动态公开路由读取已保存设置，下次请求生效；未启用的验证文件返回 404 |
| Sitemap | 静态站点产物与 sitemap 解析 | 按真实国家/语言、商品及变体、集合、博客生成；尊重 noindex 和外部 canonical；每 1,000 条分片并生成索引 |
| IndexNow | 手动、Sitemap 选取、构建后自动提交 | 实际 Sitemap → 筛选/勾选 → 校验同站地址与线上 key 文件 → 提交 → 保存 HTTP 状态、响应与成功/失败记录 |
| 自动推送 | 构建产物 lastmod、上次提交时间 | 每小时至多一次检查/尝试，按每个 URL 的成功提交记录和 lastmod 判断变化；每批最多 10,000 条，后续批次不会被全局时间戳漏掉 |
| GA4 | `internal/handlers/seo_google.go` | OAuth refresh → 指定 property 报表权限验证 → 总指标、趋势、每日数据；前台 GA4 脚本由开关和 measurement ID 控制 |
| Search Console | 同上 | 独立 OAuth、站点资源、报表；保留约 3 天报告延迟口径、按展示量加权平均排名；前台输出验证 meta |
| 实时站点审计 | 公开资源与英文首页检查 | 检查 robots、Sitemap、llms、canonical/hreflang、有效 JSON-LD、可抓取正文、标题描述，共 7 项，读取实际 HTTP/HTML |
| Nginx 日志源 | `internal/seomonitor/crawler.go` | 本地绝对路径、采集开关、间隔、手动扫描；字节游标、首行指纹、截断/轮转检测、半行保留、失败状态和准确累计数 |
| 爬虫访问 | User-Agent 分类 | AI/搜索爬虫明细、日期/路径/名称筛选、趋势和排行；爬虫 IP 哈希存储 |
| 流量归因 | click ID、UTM、Referer | 兼容的 click ID + UTM 优先；保留 campaign、medium、平台、来源、路径、国家及匹配依据；排除资产、后台、失败请求和普通机器人 |
| 路径统计 | TrafficStats | 全部符合筛选的记录参与统计；按路径访问数、每日独立 IP 累计、来源拆分和分页；同时指定语言/路径时按交集筛选 |
| AI 服务商 | ChatGPT / DeepSeek | 地址、模型、加密 key；明确选择 Responses、Chat 或兼容网关 Chat input 三种协议 |
| 品牌/引用监测 | `internal/seomonitor/citation.go` | 配置任务 → 手动或定时运行 → 保存 running/completed/failed → 保存回答、品牌命中与链接证据 → 后台详情查看 |
| 调度与并发 | 旧服务轮询 | Medusa 每分钟调度，按各配置间隔执行；日志扫描、单监测任务、IndexNow 使用锁；中断运行保留明确失败记录 |

### 分析中发现并处理的关键差异

1. 旧后台能维护页面 SEO，但在参考 backend 中未找到 `SEOPageMeta` 的公开消费链路。本次补齐了当前商城的真实 HTML 输出，后台字段可验证生效。
2. 旧 ChatGPT 请求被强制转换为向 `/chat/completions` 发送 `input`，仅适用于特定兼容网关。本次保留兼容选项，同时支持标准 `messages` 和 Responses `input`；ChatGPT Responses 请求使用 `web_search`。
3. 旧引用统计混合了服务商注释与正文 URL。本次区分“平台引用”和“正文链接”，品牌是否被提及单独记录。
4. 旧请求调试存在记录 Authorization 的路径。本次不记录密钥、请求授权头或服务商错误正文；Google 与 AI 凭据均加密。
5. 当前商品/国家组合产生超过 20,000 个 URL。Sitemap 改为索引和分片，避免单文件过大以及管理端抓取上限问题；自动推送按 URL 跟踪，避免超过 10,000 条后漏推。
6. 页面元数据没有覆盖交易隐私规则：账户、购物车、结账、订单、支付结果、账户验证、回收填写及回收申请页面强制 noindex/nofollow。

## 数据与 API

新增独立模块 `backend/src/modules/seo`，共 10 张表：设置、页面、日志源、爬虫访问、归因访问、服务商、监测任务、运行记录、证据、IndexNow 提交记录。迁移 `Migration20261023040324` 已应用。

- 管理读取：`GET /admin/seo/:resource`。管理写入：`POST /admin/seo/commands`，通过工作流执行，外部不能调用内部 `tick` 动作。
- 公开读取：`GET /store/seo?path=...` 与 `/store/seo/pages`。仅返回发布需要的白名单设置和页面元数据，不返回 OAuth 或 AI 密钥。
- 公开产物：`/robots.txt`、`/llms.txt`、`/sitemap.xml`、`/sitemap.xml?part=N`、`/BingSiteAuth.xml`、`/{indexnow_key}.txt`。
- 后端核心逻辑：`backend/src/lib/seo`；后台：`backend/src/admin/routes/seo`、`components/seo-*`；前台输出：`storefront/src/lib/data/seo.ts` 和 `modules/common/components/seo-data`。

## 使用顺序

1. 在“配置”中设置 `seo.site_url` 为实际部署域名，`seo.home_path` 为实际首页，如 `/dk-en`。域名必须与对外验证资源和 Sitemap 一致。
2. 在“页面 SEO”维护真实完整路径；例如 `/dk-en/about`。hreflang 填语言到完整 URL 的 JSON 对象，结构化数据填有效 JSON 对象或数组。
3. GA4 配置 `ga4.measurement_id`、`ga4.property_id`、`ga4.oauth.client_id/client_secret/refresh_token`；需要前台采集时启用 `analytics.enabled=true`。在概览执行“测试连接”。
4. GSC 配置 `gsc.resource`、`gsc.oauth.client_id/client_secret/refresh_token`；验证码填 `gsc.verification_code`，启用 `seo.google_search_console.enabled=true`。`gsc.sitemap_url` 保留为配置项，不表示已经调用 Google 提交 Sitemap。
5. IndexNow 设置 `seo.indexnow_enabled=true` 与 8–128 位字母/数字/连字符 key；先确认 `/{key}.txt` 可公开访问，再勾选 Sitemap URL 提交。需自动推送时启用 `seo.indexnow_auto_submit=true`。
6. Bing 设置 `bing.verification_enabled=true` 和完整 `bing.site_auth_xml`。
7. “Nginx 日志源”填运行后端机器能读取的 combined access log 绝对路径；可用环境变量 `SEO_LOG_ROOTS` 限制允许目录（Windows 用分号分隔）。前端控制台日志不能替代反向代理访问日志。
8. “AI 服务商”填写实际地址、模型、key 和协议；新“AI 监测”默认停用，先手动执行检查结果，再按需启用定时请求。

加密依赖 `SEO_ENCRYPTION_KEY`，未单独设置时使用项目 `COOKIE_SECRET`；不能为空或默认 `supersecret`。已写入密钥后不要直接更换加密密钥，否则需重新录入凭据。

## 验证证据

- 单元测试：`backend/src/__tests__/seo.unit.spec.ts`，覆盖归因优先级、机器人排除、日期/IP、统计、密钥保护、XML/HTML、协议及证据区分、失败落库、GA4/GSC 独立报表与 IndexNow 分批遗漏回归。
- 真实数据库集成：`backend/src/scripts/verify-seo.ts`，通过 `medusa exec` 执行，无第二个 Web 后端；临时 SEO 数据在 finally 删除，不变更订单、设备、库存、支付或原有配置。
- 集成验证通过 7 组：密钥加密/公开隔离；4 类公开资源发布；完整 Sitemap 索引遍历（本次 21,952 条）；7 项实时审计；真实页面标题/JSON-LD/canonical；增量日志扫描/去重/半行/轮转/汇总；本地 AI 协议夹具到运行和引用证据持久化。
- 浏览器：使用授权调试账号，只读访问真实 API；英文和中文表单提交由 Playwright 拦截，仅验证请求体，不通过该账号修改业务数据；检查各管理页导航、390px 窄屏、真实商城 canonical 和 JSON-LD，无浏览器脚本错误。
- 参考界面复验：`logs/verify-seo-reference.cjs` 验证全部 6 个图表、GA4 四系列与图例切换、日期/语言/路径过滤、路径排行分页、配置弹窗、字符串监测 ID 与两类证据；15 项 SEO 单元测试通过。
- 后端、Admin、商城 TypeScript 检查；Medusa lint 无错误。开发服务运行时按项目规则不执行构建，不清理 `.next`/`.medusa`。

域名口径：公开 SEO 配置始终返回生效的站点域名，依次取数据库 `seo.site_url`、后台 `STOREFRONT_URL`、本机默认地址，避免前台 Sitemap 和后台采集使用不同回退值。本机环境的 `STOREFRONT_URL` 当前指向开发隧道，复验时该外部地址曾超时；本地集成在尚无显式站点配置时临时写入 localhost，结束即删除。实际部署需在数据源的站点配置中填写可访问的对外域名，外部隧道可用性不属于图表或接口验证通过的含义。

本地验证不等于外部账户验收：当前没有真实 Google/AI 凭据，不宣称真实 GA/GSC 报表已授权、AI 付费接口已成功、IndexNow 已被接收或页面已被搜索引擎收录。AI 协议/证据使用本地夹具验证；Google 公式和权限请求使用受控响应测试。启用实际服务后可在后台完成真实连接与提交结果核验。

## 数据含义与边界

- 爬虫分类基于 User-Agent，不进行反向 DNS 身份认证，不能直接证明已收录或已被 AI 引用。
- “平台引用”来自接口返回的引用注释；“正文链接”只表示回答中出现 URL。DeepSeek 或兼容网关不返回检索证据时不会伪造引用。
- 品牌提及按不区分大小写的文本包含判断。它不代表推荐强度、排名或因果关系。
- 流量是可归因的成功 HTML 访问请求，不等同于 GA4 会话；独立 IP 按 UTC 日期计算每日去重后累计，不等同于真实人数。国家来自 IP 解析，失败保留未知。
- 默认调度每分钟检查，实际采集按配置间隔；日志每轮最多 4 MB，后续继续增量读取。统计全量分页读取，超大日志库应后续增加专用聚合/归档策略。
- Sitemap 抓取保护上限为 100 个文件、50,000 个 URL；单次 IndexNow 最多 10,000 个 URL。超限明确报错，不静默当成完整成功。
