# Astro 商品选择 React 岛

2026-10-28。use-product-selection.ts 将现有纯配置选择规则接入 React。购买操作与图库各自水合，通过同一商品的地址快照和自定义事件同步；模块没有 Medusa 运行时依赖。生产页面尚未引用该 hook，原 DOM/CSS、购物车及页面保真仍待完成。

## 状态与浏览器行为

使用 [React useSyncExternalStore](https://react.dev/reference/react/useSyncExternalStore)，服务器和首次水合使用调用方传入的 initialLocation；浏览器随后读取 pathname/search/hash。快照是稳定字符串，两个岛订阅同一商品，最后一个订阅卸载时移除临时选择；SSR 不写进程级选择状态。没有修改全局 History 方法。

选择前重新读取当前地址，避免连续点击覆盖刚改变的 query/hash。可用候选按既有规则产生；无候选不改状态或 URL。replaceState 保留原 history.state，不新增历史项或触发页面请求；其他重复 query 和 hash 保留。只有购买岛更新标题/canonical，图库只消费选择。popstate 清除临时选择，并按地址重新定位。

无 slug 变体沿用原规则：保留 pathname 并写 v_id。从组合 slug 页选择它时，临时选择保证当前价格和图库显示刚选配置；刷新后仍按原 slug 高于 v_id 的深链优先级恢复。从模型页选择则刷新仍定位该 v_id。此边界没有改写原 URL 规则。

## 验证与限制

[运行报告](astro-runtime-report.json)的 selection_browser 来自独立 tests/selection-fixture 根目录，使用真实 Astro 开发服务及两个 client:load 岛，测试完关闭自有随机回环监听与浏览器。人工商品仅供测试，不进入生产 src/pages，不接受业务数据库连接。浏览器运行器复用后台已锁定的 Playwright，并记录后台 lock 哈希。

浏览器验证：大于 JavaScript 安全整数的十进制金额正确显示及舍入；双岛图片/选择同步；无页面请求切换；历史长度/state 保留；重复 query/hash；标题/canonical；原生前进后退；实际刷新；slug/v_id 冲突；无 slug 临时选择；有效缺货定位、无候选保持及未知 v_id 默认；水合错误为零。另有既存 16 项单元、生产 build/type check、生产 health 与真实 Gin/MySQL/Redis 模块串联。

测试探针不是商城界面，不能据此验收原商品页的视觉、响应式或完整购买流程。下一步仍需挂载原布局/组件、资源和字体，迁移数量/加购、评论/关联商品及 SEO 扩展，再逐页对照基线。后台继续优先 Ant Design，图表统一 ECharts；前台保留原样式。
