# 购物车真实浏览器联调

2026-10-28。`retail/admin/tests/cart.live.spec.ts` 在独占 Vite 测试页面中加载实际 `retail/storefront/src/lib/cart-client.ts` 模块。所有 `/api` 请求经同源代理送往正式 `cmd/api`，连接带唯一标签的人工 MySQL 8.4 / Redis 7；不拦截或伪造 API，不新增测试运行时路由。只有临时数据准备器开启空购物车命令，生产默认能力仍关闭。

此处使用 Admin 测试宿主是为复用独占代理和浏览器生命周期，不把购物车接入 Admin 产品界面，也不证明 Astro 业务页面已挂载。范围是浏览器私有协议和真实存储，后续仍需原购物车/加购页面、行项目、可信金额和完整交易。

## 检查内容

- 浏览器自动发出正确 Origin，响应 private,no-store 与请求编号；scope/possession 为 HttpOnly、Path=/、SameSite=Lax，仅 CSRF 对 JavaScript 可见。
- 创建返回精简投影及字符串版本；读取后刷新仍由浏览器持有凭证恢复，不把凭据写到 localStorage/sessionStorage。
- 同作用域、同操作键四路并发重放均返回原购物车；换语言复用键返回409，不生成第二辆车。
- 重用创建作用域不 Set-Cookie、不延长原过期时间。丢失 possession 后读取不可访问；保留创建作用域和原操作键可重放并恢复原车。
- 陌生浏览器仅持有 ID 不能读取；购物车凭证不能登录后台；外部 Origin 创建拒绝。
- 浏览器通过后直接查询独占 MySQL：购物车、有效version=1归属、创建回执、创建作用域各1；客户绑定和行项目各0。只提交计数，不导出生成ID或cookie。

## 联合入口与限制

`python retail/scripts/test-admin-live.py` 顺序执行原 Ant 后台浏览器用例及新增购物车用例，然后执行 Astro 构建、28项单元、真实 Gin 基础运行时和原组件截图对照。成功后更新 [联合报告](admin-live-report.json) 的 `cart_browser` 及 [Astro报告](astro-runtime-report.json)；只有整个入口成功才写成功报告，局部通过不能替代整组结果。

首次本批联合运行两项真实浏览器及 SQL 回查通过，但后续 Astro 操作区首次导航15秒超时，整组失败；保留失败日志。新增 pending request 诊断，未改超时、图像差阈值或断言。单独复跑操作区24对截图及全部交互通过，最大RGB通道差1；整组结果以生成报告和源码 SHA 为准。

MySQL 单核768MiB、Redis 单核128MiB；数据库和Redis无主机端口，API仅随机回环端口。最终按精确owner标签清理容器、专用bridge及临时二进制。此处为 HTTP 开发模式；HTTPS/Secure实测、跨浏览器、故障注入、生产页面和完整交易尚未验收。

本批最终整组重跑成功：两项真实浏览器、SQL回查、Astro类型/build与28项单元、40对图库/规格及24对操作区截图均通过。最新后台数据库报告144文件、购物车专项51文件、浏览器联合140文件、Astro170文件及客户端8文件SHA逐项匹配。两个测试owner标签下容器和网络均为0；原9001 health/Admin与8000商品/购物车均200，最后内容完整读取未新增stderr错误。首次超时原因未复现，不宣称已找到或修复根因；诊断保留，阈值与断言不变。
