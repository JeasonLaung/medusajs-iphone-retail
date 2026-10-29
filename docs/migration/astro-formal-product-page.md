# Astro 正式商品页面接入（2026-10-29）

按[当前交付范围](current-delivery-scope.md)推进 C01。新增 `retail/storefront/src/pages/[countryCode]/products/[handle].astro`，承接原商品 handle 与配置 slug URL；SSR 直接调用独立 Gin 公共商品、地区、语言、分类与合集接口，无 Medusa 运行时依赖。此批属于页面接入，购物业务闭环仍未完成。

复用已迁移的原图库、购买区、规格、头尾组件及编译全局 CSS。面包屑来自原 `modules/common/components/breadcrumbs`，仅替换本地链接和图标导入，CSS 原样复制。保留中英文、配置选中、图片联动、已有查询参数/锚点、刷新配置链接、canonical 和 Open Graph。修复 Gin 购买区把“私有购物车尚未准备好”误用于禁用公共规格选择的问题：规格继续可选，购买操作仍由真实 commerce 能力控制。

未知商品/地区/语言组合返回 404；公共依赖失败返回 503，不用空商品伪装成功。页面禁止缓存，不传递用户 Cookie/Authorization 给公共接口，不将依赖错误正文或服务地址写入 HTML。Astro Node 适配器仅信任明确的本机 Host；正式域名配置留待部署阶段处理。

验证：

- Astro 类型检查通过，0 errors；两个既有测试提示未变。正式生产构建通过。
- 现有 61 项单元测试通过。
- `tests/product-page.browser.mjs` 运行构建产物的正式路由，使用独占的人工公共 API：验证 SSR 头尾、规格选择、配置 slug 刷新、参数/锚点保留、canonical 更新、英文/中文、390px 布局、私有头不外传、HTML 转义、404/503 和购买按钮保持关闭。资源在 finally 中回收。
- `tests/runtime.integration.mjs` 已增加真实隔离 Gin 的正式商品路由断言；本批尚未重新运行完整 MySQL/Redis/全部历史浏览器联合演练，旧 `astro-runtime-report.json` 不能当作本批完整验收报告。

可在 `retail/storefront` 使用 Node >=22.23.4 运行 `npm run build`，再执行 `npm run test:product-page`；浏览器可用 `ADMIN_TEST_BROWSER` 指向已安装的 Chromium。该测试不访问真实业务或支付渠道。截图写入 Git 忽略的 `logs/gin-migration/formal-product-{desktop,mobile}.png`。

仍需继续：商品评价与推荐区、购物车/客户身份和数量导航、真实切换国家/语言的购物车迁移、加购及结账支付、其余现有前台路由。当前头部仅处于匿名展示，地区保存尚未接入；这些均不算交付完成。
