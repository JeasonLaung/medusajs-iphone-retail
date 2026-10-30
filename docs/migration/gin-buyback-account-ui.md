# 原回收账号页面接入 Astro / Gin 客户端

2026-10-30。Astro 新增原 `/[countryCode]/account/buyback` 和详情 `.../buyback/[id]`，接入既有账号登录与侧栏，页面私有 no-store、noindex，详情沿用原详情标题。保留原回收 React 列表/详情、文案及 CSS；未改版。Medusa 服务端动作替换成既有签名 Astro bridge 的 Gin 客户端，浏览器无 Medusa SDK/服务端运行依赖。

## 保留的行为

- 每页 20 条、账户汇总、刷新/失败重试、申请链接、状态与历史。
- 原序列号/IMEI 校验、错误滚动定位、正反面及边框条件照片、可选包装/封箱照片。
- 原浏览器白底缩放至 1800、WebP 95% 压缩、私有预览与审核标签；真实图片压缩后上传内容由浏览器测试回读并验证成功解码。
- 提交照片、物流输入、接受/拒绝报价、取消及原确认弹框；过时报价保留弹框和错误，人工重试，无自动命令重试。
- 只适配 Ant6→既有 Ant5 的 Alert/Steps/Modal 属性，保留既有共享主题。七个图标取既有 MIT Medusa Icons SVG 实现及类型，保留原图标和授权，不增加运行时库依赖。

客户端只接受当前详情的固定公开顶层字段，校验日期、金额、归属照片/设备、分页完整性、响应码和申请 ID；业务错误仅透传预定义状态匹配。写操作沿用 session CSRF；本轮不修改 Gin 后端或真实业务数据。

## 验证和边界

`node --experimental-strip-types --test tests/*.test.ts`：173 项通过。Astro check：387 文件、0 错误、0 警告、5 个既有 hints。

`node --experimental-strip-types tests/buyback-requests.browser.mjs`：七组真实 Astro/React 浏览器交互，覆盖 23 条分页/汇总/失败恢复、真实 PNG→WebP/三槽解码预览、身份/照片校验、五动作、过时报价、中文物流及匿名登录。浏览器 API 是拦截夹具，不能作为真实 Gin 联合证据。

原源码由测试插件直接读取，仅替换框架/API/Ant版本属性；中英文×390/1440×列表/待上传/最终报价共 12 组页面主体截图，在共享 Ant5 环境下文字及像素完全一致。此结论不包含原 Next/Ant6 整页、真实图片视觉或所有状态的像素验收。

旧 9001 health/app、8000 dk-en/sell 均 200；stderr 时间未新增，既有 `/store/locales` 404 仍存在。未对旧登录账户或真实订单/设备/库存/付款做修改。

真实 Astro→Gin→MySQL/Redis 创建/上传/客户命令联合、历史文件导入及 180 天清理、平台审核/收货/检测关联/最终报价、付款/退回及通知邮箱继续待办。当前页面接入不代表回收闭环完成。详见 [验证报告](buyback-account-ui-report.json)。
