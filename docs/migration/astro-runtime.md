# Astro 独立 SSR 底座

2026-10-28。新增 `retail/storefront`，当前实现生产 Node SSR、React integration、Gin 区域传输、原国家/语言工具及 `/health`。未实现的商城页面仍返回 404，不提供替代首页或假业务页面。T025/T026 与整体前台尚未验收。

## 运行与依赖

锁定 Astro 7.4.2、@astrojs/node 11.1.9、@astrojs/react 7.1.1 及 React 19.2.8；Node 至少 22.23.4。新应用独立 npm lock，不添加 Next 或 Medusa 依赖。参考 [Astro SSR](https://docs.astro.build/en/guides/on-demand-rendering/) 与 [Node adapter](https://docs.astro.build/en/guides/integrations-guide/node/)；版本与运行时要求以 package.json/lock 为准。

初次验证为避免调整旧服务的 Node，单独下载官方 Node 22.23.4 Windows 运行时并按官方 SHASUMS256 校验，保存在忽略的 logs/gin-migration。该本机路径不写入应用配置，不是生产部署方案。

在兼容 Node 下，进入 retail/storefront 执行 `npm ci`、`npm run build`、`npm test`。开发命令为 `npm run dev`，回环端口 9300；生产 `npm start` 运行 dist/server/entry.mjs，监听地址/端口由 HOST/PORT 控制。当前任务不启动常驻预览、不部署或切流，旧 9001/8000 服务继续运行。

服务端秘密环境变量 GIN_API_ORIGIN 只接受 HTTP(S) origin，不能含凭据、路径、query 或 fragment，示例为 http://127.0.0.1:9100。没有 Medusa 回退地址。Astro secret env 不进入客户端；health 请求只使用该配置，固定调用 `/api/store/regions`，不转发请求 cookie 或账号头、不跟随重定向、不缓存会话数据。[后续商品模块](astro-products.md)增加独立传输及投影，尚未被业务页面引用。

区域传输有五秒超时与 2 MiB 实际字节上限、严格 UTF-8/MIME/JSON 和 DTO 校验。区域/国家总数、代码、归属、重复及 count 一致性验证后才返回明确字段；金额及国家编号不转数字。`/health` 仅验证这条 Gin 读取路径，成功 200，配置/依赖/数据异常 503，固定 JSON 与 no-store，不设置 cookie；不声称已覆盖客户登录、购物车或全部后端依赖。

## 页面保真边界及验证

`storefront-route.ts` 从旧实现原样复制，单元测试比较源码及原国家/语言组合行为。尚未接入 Astro 国家重定向 middleware、公开商品/搜索/价格、原 CSS/字体/资源、布局和 React 交互岛。后续逐页迁移必须对照既有截图及状态，不能把本底座当作页面保真验收。Ant/ECharts 是后台约定，不替换前台组件。

[运行报告](astro-runtime-report.json) 记录类型/build、10 组单元测试（原底座 3 组及商品/金额 7 组）、当前 0 项依赖审计及生产 Node 进程到独占 Gin/MySQL/Redis fixture 的验证。fixture 有 1 个区域/2 个国家；生产 SSR 健康响应与缺少配置时的 503、无会话 cookie、no-store 和未迁移业务页 404 均验证，两个自有 Astro 子进程退出。商品读取额外在源模块层连接同一 Gin，不冒充生产商品页面验证。

执行入口是 `test-admin-live.py`，由它创建并持有独占 Gin/MySQL/Redis，再调用 `test-storefront-runtime.py`；不接受业务库连接。兼容 Node 若不在 PATH，可将 STOREFRONT_TEST_NODE 设置为对应可执行文件；Windows 浏览器沿用 ADMIN_TEST_BROWSER。原 Ant 全量浏览器用例继续执行；外层 finally 清理独占资源。这里的浏览器证据属于后台，新 Astro 业务页面尚无浏览器验收。
