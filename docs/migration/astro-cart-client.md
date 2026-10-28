# Astro 的 Gin 私有购物车客户端

2026-10-28。`retail/storefront/src/lib/cart-client.ts` 承接 [Gin 匿名购物车 HTTP](cart-http.md)，不使用 Medusa SDK 或旧认证；作为原购物车流程迁移的协议基础，尚未挂载生产页面、没有加购或结账能力。

## 调用和恢复

浏览器使用 `browserCartClient()`，仅在调用时访问 window/document，不在模块加载时读取浏览器或 SSR 私有状态。请求固定同源 `/api/store/cart-sessions`、`/api/store/carts` 和明确 ID 的详情路径，不接受客户端后端 origin，不复用公共目录 fetcher。

`openScope()` 读取同源可读 scope_csrf cookie，发起同源 POST；HttpOnly scope/possession cookie 由浏览器自动携带。返回的 `CartScope` 只持有 CSRF，不含 bearer token。HTTPS 使用 __Host- 名称；旧 Medusa cookie 和无关凭证不参与，重复/非法 CSRF cookie 拒绝。

`create({operationID,regionID,locale}, scope)` 要求调用方提前生成并保留规范 UUIDv4 操作键，以相同输入和已取得作用域重试。客户端只发送地区/语言，丢弃额外价格、客户和店铺字段；不修改输入。它不会在每次创建或重试时隐式调用 openScope，因此作用域失效不会自动切换幂等命名空间并再建一辆车。

创建成功严格核对 201/首次与 200/重放、原请求地区/语言、明确字段、CSRF/到期和 capabilities。ID 与 MySQL uint64 version 保持字符串，拒绝越界、不规范版本、数值替代、尾随换行、私有新增快照字段及未知购买能力。当前 items/checkout 必须为 false；未来行项目/金额适配另行明确实现，不能仅靠开关让旧 UI 开放付款。

`retrieve(id)` 明确使用 possession cookie，GET 不发送 CSRF、不创建作用域。只有服务端401/404表示无法访问并返回 null；503、网络/超时、协议错误均抛出分类错误。当前 ID 仍需上层流程保存/传入；不是已实现完整页面刷新恢复或客户转移。

## 故障及边界

请求 `credentials=same-origin`、`cache=no-store`、`redirect=error`，不手工拷贝 Cookie/Authorization。正常响应需 JSON 与 private,no-store，读取上限64KiB、12秒超时、严格UTF-8/JSON；不接受公开缓存或HTML成功页。仅暴露规范请求ID、有限Retry-After与本地固定错误分类，不显示任意服务端错误正文。

应用不自动重试。POST 网络、5xx或成功响应损坏可能已经提交，`CartError.uncertain=true` 提醒上层保留原操作及作用域，不能按“没有购物车”改用新键。浏览器/网络层本身可能在未收到响应时重发；固定操作键与后端持久幂等始终必需。错误分类供后续原页面本地化反馈使用，本次没有新增或改写可见文案。

## 实际验证

`python retail/scripts/test-cart-client.py` 使用隔离兼容 Node，运行新 Astro 应用的28项单元测试、类型检查/生产build，以及 Edge 上的真实同源 HTTP 传输测试。输出 [购物车客户端报告](cart-client-report.json)，含准确文件 SHA。只构建新 `retail/storefront`，不构建/重启旧 Next 或 Medusa 服务。

单元覆盖明确请求头与字段、字符串版本、同操作重试、错误分类、CSRF cookie、坏响应/隐私字段/未知能力、MIME/缓存/大小/编码和非法输入拒绝；其余既有19项价格、配置、路由和库存纯规则继续通过。

浏览器使用自有随机回环端口的人工 API fixture，实际加载客户端模块、由浏览器处理 cookie/Origin，验证 HttpOnly scope/possession 不出现在 document.cookie、可读 CSRF、创建和重放、读取、响应中途断开后的不确定错误/同键重试、依赖503不为空及凭证丢失。测试不拦截 fetch，但 API 是人工协议实现，**不是 Gin/MySQL/Redis 端到端证据**；不替代尚待恢复的 Docker 联合回归。浏览器、context和回环监听器在 finally 中关闭。

本次没有改变原页面布局、组件样式、图片或购买交互，也没有把人工 API 放入生产构建。下一步仍需真实 Gin 浏览器串联、行项目/完整可信合计、数量修改及地区变化、原页面挂载和完整交易。T025/T026/T032/T033 不作整体验收。

2026-10-28 后续：[购物车真实浏览器联调](cart-live-browser.md)新增实际客户端→Gin→MySQL/Redis验证，包含四路同键重放及SQL计数；其范围与原人工协议fixture独立，生产页面、行项目和结账仍未完成。

2026-10-28 后续：[购物车持久金额](cart-amounts.md)新增受当前持有凭证保护的amounts查询及Astro字符串DTO，保存税/折扣/配送参与精确计算。不是当前选价重算，checkout仍关闭，完整加购/交易及原页面未完成。

2026-10-28 后续：[Astro加购客户端](astro-cart-add-items.md)新增真实Gin草稿加购命令，独立购物车CSRF与精确回执；不将草稿金额当作可付款重算结果。
