# 直接 HTTP 调用与封装分支承接

这是 T001/T002/T004/T007 的部分设计证据，尚非完整 API 或运行验收。`inventory-http-callers.cjs` 静态扫描 storefront/src 与 backend/src/admin 中的 fetch 调用，支持多行、泛型、字符串、模板、词法 const 和函数参数遮蔽，不执行应用。与旧 SDK 清单互补，不代替它。

## 证据范围

- 58 个实际含调用文件、108 个候选；每个来源文件有 SHA-256。输出 `http-caller-manifest.json` 记录源码行、原表达式、可解析路径、method 证据及选项字段。
- 3 个动态路径和 1 个动态 method 未在扫描阶段臆测；人工追踪下表后，`map-http-caller-contract.py` 将候选展开为 117 条分支设计、88 个方法/目标路径组合，保存 `http-caller-contract.json`。每条记录身份、任务、关键边界、CSRF 与未完成项。
- 9 个人工扫描用例核对模板、泛型、参数遮蔽、块级 const、动态 method/options、默认 GET 和忽略 refetch。扫描器只支持有限静态求值，不是完整 TypeScript 调用图。

| 封装 | 实际分支与迁移边界 |
| --- | --- |
| storefront/lib/data/buyback.ts `call(path,body)` | list/detail/create/update/upload/download 六个调用端；body 存在则 POST，否则 GET。不能把封装的默认 GET 记到全部分支；客户归属、照片嵌套检查和状态机继续适用 |
| admin/components/device-workbench.tsx `command(body,message,path)` | 默认 devices；供应商账号、设备退款、订单售后另传路径。均 POST；这些仍需分别动作授权，不能以 workbench 页面已登录替代退款/账号配置权限 |
| admin/routes/settings/webhook-notifications/page.tsx save | 新增和保存已有 endpoint 两个 POST 路径；空密钥保留、显式 clear 与 test 的外部副作用分别处理 |

路径占位符归一化仅用于去向覆盖，不是 Gin 可直接注册的路由字符串；实施时使用具名参数并逐个 schema。查询参数不构成新 endpoint，仍需逐字段 allowlist；源码 `BACKEND_URL` 前缀是旧 middleware 的后端地址，新 Astro 直接使用新站点配置，不导入旧公开 key。

## 本批补出的能力

直接调用包含 shipping-options 列表/计算、locales、payment-providers、PayPal browser config、product-options、product-variants、search、本人订单/设备/售后/时间线、通知和回收照片。不能用原来 SDK 目录查询代替这些能力。

后台包含设备工作台、回收、邮件附件、SEO、仓库范围、Mollie 配置、外部通知、评论审核和原生 uploads。uploads 不在 67 个自定义路由文件中，却是 SEO 图片选择器实际消费的原生能力；新系统需独立验证文件类别、MIME/文件头、体积和权限，不能一律开放 static。

88 个去向组合目前保留领域查询/命令结构，仅将 store/admin 命名空间移入 `/api`。相同路径下的多个 action、邮件 resource、SEO resource 仍需细分请求/响应与独立权限，不能以一个 POST 已有去向宣称所有命令完成。设备 supplier_assign 仍按旧工作流明确拒绝，不因输入验证器存在就新增行为。

## 回跳和敏感数据

旧 `app/api/payment-return/route.ts` 的 GET 在校验 query 中的 PaymentIntent ID/client secret 后调用 placeOrder。这与新 API“GET 只读”约定冲突；Astro 保留原回跳 URL/当前结果页设计，把状态确认及成单送到有 Origin/CSRF、cart possession 和持久幂等的 POST。回跳 query 不是可信支付事实，禁止记录原 query/client secret 或将它写入公开 DTO。

`PaymentResult` 当前对 `/api/mollie-return` POST 串行轮询，最多 12 次、间隔 5 秒，状态 pending/error 再试、confirmed 后跳转。新实现保留这些展示和交互，补上 CSRF、当前购物车授权、可信 provider 查询及幂等；不能把每轮轮询当作全新支付/订单操作。GET 回跳页面仅重定向/渲染，不隐式写业务。

金额沿用精确字符串契约。邮件、SEO、Mollie、Webhook、供应商账号和设备成本使用独立 DTO 与授权；菜单状态或成功截图不能证明服务端边界。

## 仍未覆盖

所有“未找到调用方”只能记待查，不据此删除路由。此扫描不覆盖不同名称的请求库、原生 Admin 包/插件内部调用、HTML/JS 字符串里的移动入库 fetch、download/href、form action、回调入口或完整调用可达性。`/admin/devices/import-template` 下载、移动凭证和照片、邮件退订/回调及支付 webhooks 继续在全入口映射中补齐。新 API 请求/响应 schemas、幂等/version 精确策略、领域错误码与拒绝测试也仍待交付，T007 不勾完成。
