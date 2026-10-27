# 请求边界基线与前台 SDK 承接

T001/T004/T007 的部分交付，尚未验收。新增机器证据不表示 Gin 已实现这些 API。

## 可重现的旧输入行为

`retail/scripts/capture-request-validation.cjs` 仅允许加载 15 个 API 验证器和 2 个纯业务工具，以及当前安装的 framework Zod。使用 TypeScript 转译及受限 require，不读取环境/凭据，不加载 app、workflow、container、数据库或网络模块。

- `schemas/legacy-request-inputs.json`：34 个导出的输入 JSON Schema，记录 17 个来源文件 SHA-256；输入结构保留各字段上下限、数组、枚举、strict 及默认声明。
- 同文件单列 69 处 refine/superRefine/transform/trim/大小写等操作及源码位置。它们需要 Gin 独立实现；JSON Schema 不能证明已复制运行行为，也不能替代业务状态、对象授权或文件真实性检查。
- `fixtures/legacy-request-validation.json`：22 个人工用例，有独立接受/拒绝及部分标准化输出预期。覆盖评价星级/分页、仓库空范围与重复项、设备 SN/IMEI、默认值/严格未知字段、重复设备、回收金额/照片、售后数量、邮件操作及禁止公开 tick。

重要差异：旧 CreateReview 的 z.object 会剥离额外 customer_id；新入口依旧只能从会话取得 customer_id，不能将剥离行为视为完整授权。旧 DeviceCommand 声明允许的 action 不一定能成功：例如 supplier_assign 工作流始终拒绝，必须同时使用设备动作契约。

旧设备上传只验证声明/长度的输入结构不等于文件真实。JPEG/PNG/WebP 文件头、解码大小、私有下载和对象归属仍单独校验。回收问卷完整性、报价顺序、重复仓库、SN 归一化及客服时区/联系渠道/营业时间都在结构之外有规则。

## 精确金额与表单适配

旧回收和退款验证器接受 number 并拒绝金额字符串；报价部分以浮点误差阈值检验两位小数。新 API 按通用约定用精确十进制字符串。React/Ant Design 表单和 Astro 交互层必须将输入原文合法化为字符串，不先 Number 再转回；旧显示组件需要明确适配，不能让类型变化导致页面价格消失或金额被缩放。

设备 cost_cents 和星级、数量明确整数保持整数，不能把所有 numeric 字段机械改字符串。金额字段取值范围、两位小数报价、允许有符号调整与退款正数分别设计；数据库 DECIMAL(38,18) 不自动意味着每个 HTTP 金额都允许 18 位小数。

## 36 个 SDK 候选的去向

`storefront-sdk-contract.json` 为源码清单的 36 个调用候选逐项保存来源、行号、SHA-256、目标方法/路径、身份、CSRF、幂等策略、响应类别和任务。28 种 SDK 表达式承接为 27 个新操作设计；未使用导出与调用可达性仍待追踪，不能按未见请求删功能。

| 能力 | 新边界与调用层变化 |
| --- | --- |
| 注册、资料创建、登录 | 新注册命令合并身份与初始资料；删除旧“查询失败就创建客户再登录”策略，网络/数据库故障不能被误当成不存在。客户与员工同邮箱仍是独立身份；保留前台 verification_required/success/error 展示状态 |
| 邮箱验证 | 受限 challenge 只能重发到该新身份邮箱；确认能力绑定 purpose/有效期/替换状态。旧验证链接不恢复；GET 页面不消费凭证，显式提交确认。该处安全交互变化必须在 Astro 对照验证，其他页面不借机改版 |
| 购物车 | 创建签发独立 possession cookie，ID 不是授权。登录后的 claim 是当前购物车所有权转移，不是两个购物车合并。行/地址/地区/优惠码/配送变更要版本检查并重算，不能复用过期支付金额 |
| 支付 | 从 cart 创建 payment attempt，服务端派生 collection 并绑定版本/金额/币种；旧任意 payment_collection_id 不变成公开付款能力。浏览器只取得 allowlist 配置，回跳 GET 只查状态，不隐式完成订单 |
| 订单转移 | 请求不授所有权，接受/拒绝使用新签发且绑定订单/接收人的短期能力，并明确 POST 消费。历史业务不能由同邮箱新账号自动接管；不向任意申请者泄漏订单邮箱 |
| 目录、地区、分类、collection | 明确渠道和地区/币种/语言、可见性、DTO 与分页；不能透传 fields=*。Sitemap、回收选型和搜索都有调用候选，不只迁商品页 |

这份文件不覆盖直接 sdk.client.fetch、Admin 原生、插件以及全部 86 个自定义方法；也没有完整请求/响应字段 schema。T007 继续保留这些待办，不能因 36 个候选已有去向而验收。

## 现有 Gin 草稿差距

直接读取 `retail/backend` 当前源码确认以下问题，后续 T010—T019/T020 实现时必须解决；本轮未运行草稿或对旧服务做修改。

| 位置 | 当前行为及影响 | 所需承接 |
| --- | --- | --- |
| internal/server/router.go | gin.Logger 默认记录 URL；没有通用 request ID；只定义目录及后台登录 | 安全路由日志、请求追踪、受控 panic/依赖失败、完整领域注册 |
| internal/platform/http.go | 错误为顶层 code/message；GuardOrigin 缺 Origin 仍允许、没有 CSRF，按 Host 拼 origin | 统一 error envelope、服务端可信站点配置、会话/匿名写 CSRF；集成回调独立验签 |
| internal/identity/admin.go | MySQL session、进程内限流，仅 role=admin；无客户/仓库/供应商权限版本 | 两类新账号及 Redis 会话/限流，MySQL 动作/对象/字段权限，禁用和撤权检查 |
| internal/catalog/http.go | Decode 原 payload 后覆盖少数字段；公开只过滤 published，不做渠道；简化价格过滤、无库存语义 | 公开 DTO 白名单，渠道/仓库范围，完整变体/图/选项、价格和数量库存；不能公开 raw metadata |
| internal/importer/catalog.go | timestamp 错误变 NULL/epoch、JSON 通用编码及部分字段默认；只导目录 | 严格类型/缺列/时间/精度拒绝，版本化设计映射，全域隔离导入与问题账本 |
| migrations/0001_foundation.sql | ID 80 ASCII、handle 全局唯一、金额 DECIMAL(24,8)、原生 JSON 等旧草稿设计 | 对照已完成的字段处置设计及源索引重建 DDL，不能直接在旧草稿表上声称已迁移 |

这些问题是实现前已知差距，不是线上已发生泄漏的证据；草稿仍未验收。不能通过保持目录页面可读或 `go test` 编译成功来替代整域迁移。
