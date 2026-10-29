# 原分类兼容 URL

本项属于“整个现有前台闭环”的 C01，不新增分类页面或调整现有商店视觉。

原入口为 `medusa-store/apps/storefront/src/app/[countryCode]/(main)/categories/[...category]/page.tsx`，通过 `lib/data/categories.ts` 按完整、以斜杠连接的 handle 查询分类。只有直接父分类的 handle 为 `iphone` 时，将分类名称作为 `series` 参数；其余分类与未知分类均永久跳转到商店。原请求的其他查询参数不传递到目标地址。

Astro 的同名路由承接上述行为，使用现有 Gin 公共分类接口和已有 `storeSeriesUrl` 编码逻辑，返回 HTTP 308。先核对已配置国家与中英语言；无对应地区返回 404。分类接口不可用或返回不一致关系时返回 503，避免把依赖故障误报成成功的无筛选跳转。公共请求不携带客户或管理员 Cookie/Authorization。

验证记录见 [报告](astro-category-redirect-report.json)：全套 97 项前台单元测试通过，Astro 类型检查和构建通过。独占 Astro 正式构建运行时验证 7 项实际 HTTP 响应，涵盖嵌套 handle、中文编码、根分类、未知分类、依赖故障及国家/语言错误。运行时使用合成公共 API，没有运行真实 Gin + MySQL 联合测试；此项不代表商店筛选的全数据验收或整个前台已完成。现有商店筛选验证继续见 [正式搜索页](astro-store-search.md)。

复核命令（`retail/storefront`，Node >=22.23.4）：

```text
node --experimental-strip-types --test tests/*.test.ts
node node_modules/astro/bin/astro.mjs check
node node_modules/astro/bin/astro.mjs build
node tests/category-redirect.runtime.mjs
```

运行时脚本使用自有 19372 端口及随机端口的公共 API 夹具，结束时关闭自有子进程，不修改旧开发服务或业务数据。
