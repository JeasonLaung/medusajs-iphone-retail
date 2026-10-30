# Astro 回收问卷与申请入口

2026-10-30。承接原 sell-flow、flow-copy、flow CSS、提交表单及 request CSS，补充 Astro 动态路由和同页导航。沿用原 sessionStorage key、型号/容量改选清空、九步问卷、损坏跳题、不符合条件拦截、查看/修改答案、多台设备暂存、联系与原路退回地址、两项确认以及登录弹框。

报价使用 Gin buyback estimate；申请经既有私有 customer bridge 转发到新 Gin 创建接口。桥接保留原 JSON 字节供服务端重复键校验，签名覆盖原字节与客户会话/CSRF；创建只接受 201，不接受 200/202，无新增 cookie 权限。客户客户端失败不自动重试，原表单人工重试保留 submission_key，成功后才清除申请暂存并跳转到原详情路径。

Astro 的客户信息为异步读取：补充迟到登录状态同步及未填写/未触碰字段的姓名、电话预填，避免原服务端预载改为客户端后误弹登录或覆盖用户输入。Ant 6→5 仅调整 Space、Alert 属性；保留当前全站字体与主题。

## 证据

- 165 项 Node 前端测试通过；新增 2 项涵盖原申请载荷、CSRF、响应丢失后同键重试、401 与其他错误、严格创建状态、桥接原字节/签名/路径/查询/禁止设置 cookie。
- Astro check：356 文件，0 错误、0 警告、5 hints。
- 真实 Astro/React 浏览器七组：健康全问卷至汇总、刷新恢复/迟到身份、失败后同键提交与成功目标；匿名弹框不写入且保留表单；屏幕及背面跳题；直入汇总守卫；中文手机无横向溢出；不符合条件拦截/重启；多台暂存。
- 已查看中文手机汇总截图，表单/金额/状态和布局可读。没有声称 Next/Ant6 全页像素验收。
- 旧 9001 health/app 与 8000 dk-en/sell 均为 200。近期日志仍有既存 /store/locales 404；本轮无新增 stderr。

命令在 retail/storefront 执行：

```text
node --experimental-strip-types --test tests/*.test.ts
node node_modules/astro/bin/astro.mjs check
node tests/sell-questionnaire.browser.mjs
```

浏览器 API 响应为拦截夹具，申请详情目标为占位页。本轮不证明 Astro→真实 Gin 创建联合，不证明详情/照片/审核/最终报价/付款退回；这些仍待完成，回收闭环未验收。详见 [结构化报告](sell-questionnaire-report.json)。
