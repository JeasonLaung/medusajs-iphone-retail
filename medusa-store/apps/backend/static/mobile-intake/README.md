# 独立移动端设备入库

页面入口：`/static/mobile-intake/index.html`。纯 HTML + Vue 3 全局构建 + Vant 4，通过 script 标签加载本地库，不使用 Admin React、后台布局或前端打包流程。只通过 `/mobile-intake` API 交换数据。

## 使用

1. 打开后台「设备入库」，页面默认选中自有仓库；有多个自有仓库时可切换。
2. 在「手机临时入库」填写手机可访问的后端域名或局域网地址。
3. 点击「创建临时链接和二维码」，复制链接或用手机扫码打开。
4. 手机端选择 SKU，扫描或填写序列号、IMEI / EID，补充成本、库位及备注，然后确认入库。成功后清空设备标识和备注，保留 SKU、成本、库位。

摄像头需要浏览器信任的 HTTPS 和相机权限；HTTP 可以手动填写。现有 `start-device-camera.bat` 提供本地 HTTPS，手机需信任对应的本地 CA。手机与电脑使用局域网时，请不要把 localhost 或 127.0.0.1 作为手机访问地址。

临时链接有效期为两小时，绑定创建人及自有仓库，只允许查询 SKU 和创建入库记录。链接持有人无需后台登录；请仅分享给入库人员。每次请求重新检查创建人的权限及仓库。服务端验证签名和到期时间，仓库不能通过提交参数改写。令牌放在 URL fragment 中，请求通过 `x-intake-token` 传递。轮换后端 JWT 签名密钥会使现有链接失效。

设备提交复用现有入库工作流，保留序列号 / IMEI 唯一性与 SKU 配置校验。入库状态为待检测，与后台入库一致。采购成本沿用设备账本的 `cost_cents` 整数分字段，界面显示 EUR 金额。最近入库记录仅在当前页面内存中保留。

## 接口

- `POST /admin/devices/mobile-intake`：后台登录后提交 `{ "source_id": "…" }`，返回令牌和到期时间。
- `GET /mobile-intake`：验证临时令牌，返回仓库名称和到期时间。
- `GET /mobile-intake/variants?q=…&offset=0`：分页搜索 SKU，不返回库存和采购成本。
- `POST /mobile-intake`：提交单台设备；仓库由令牌确定，拒绝额外的命令和仓库参数。

## 验证

在 backend 目录设置 `TEST_TYPE=unit` 后运行 `pnpm exec jest --runInBand src/__tests__/mobile-intake.unit.spec.ts`。

启动开发服务后，设置 `PLAYWRIGHT_MODULE_PATH` 为 Playwright 模块绝对路径，运行 `pnpm exec medusa exec ./src/scripts/check-mobile-intake.ts`。脚本使用独立测试商品和仓库，验证真实入库、中英文 UI、重复拦截、摄像头启动关闭、后台二维码，并在 finally 中清理测试设备及商品仓库。

## 本地依赖

公网 FRP 入口仅对 `/static/mobile-intake/` 启用专用资源优化：将首屏 CSS 内联，并将 Vue、Vant、翻译和页面逻辑合成一个普通脚本响应，使用 Gzip、内容版本及缓存校验。原始 HTML 和 JS 仍可直接运行，无需前端构建。扫码库在点击打开摄像头时才加载，关闭页面或扫描窗口会释放摄像头。API、后台及商城的转发方式不受该优化影响。

验证专用资源路由：`node --test scripts/mobile-intake-assets.test.cjs`（项目根目录）。通过本地 FRP 网关验证完整扫码页时，可额外设置 `MOBILE_INTAKE_BASE_URL=http://127.0.0.1:18080` 和 `MOBILE_INTAKE_SCAN_ONLY=1`，再运行上面的入库检查脚本。

- Vue 3.5.13（MIT）
- Vant 4.9.21（MIT）
- @zxing/browser 0.1.5（MIT，内含 @zxing/library Apache-2.0 组件）

第三方发布文件存放在 vendor，保留原始许可声明。升级时替换对应发布文件，并重新验证扫码、表单和页面布局。
