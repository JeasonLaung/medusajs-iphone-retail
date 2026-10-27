# 前台页面与采集基线

日期：2026-10-27。T003 进行中，尚未验收。页面源码以 `medusa-store/apps/storefront/src/app/[countryCode]/` 为根；路由组与 parallel slots 不属于 URL。同一 URL 的登录前后实现必须分别核对。

## 首轮采集

本机目录：`logs/gin-migration/storefront-baseline-v2/`。机器索引为该目录的 `capture.json`，包含路径、状态、标题、canonical、视口、横向溢出、坏图、页面异常及截图 SHA-256。目录被 Git 忽略；不把可能含业务内容的截图和原始运行报告提交到仓库。

每个用例均采集 en / zh，宽度 390 / 768 / 1440px，高度 900px，Europe/Copenhagen 时区及 reduced motion。截图文件名为 `<case>-<language>-<width>.png`，共 78 张。

| case | 路径（前缀 /dk-en 或 /dk-zh） | 截图数 | 页面异常 |
| --- | --- | --- | --- |
| home | / | 6 | 6 次 hydration mismatch |
| store | /store | 6 | 0 |
| about | /about | 6 | 0 |
| help | /help | 6 | 0 |
| blog | /blog | 6 | 0 |
| article | /blog/choose-your-storage | 6 | 0 |
| reviews | /reviews | 6 | 0 |
| sell | /sell | 6 | 0 |
| ui | /ui | 6 | 0 |
| cart-empty | /cart | 6 | 0 |
| account-logged-out | /account | 6 | 6 次 hydration mismatch |
| verify-missing-token | /verify-account | 6 | 6 次 hydration mismatch |
| payment-result-missing-reference | /payment-result | 6 | 0 |

78 个页面均返回 HTTP 200；没有检测到横向溢出或已加载图片损坏。18 个页面实例报告 hydration mismatch，属于旧实现基线缺陷，不能把 HTTP 200 或截图存在当成浏览器无错。Next 开发工具的 issue 提示属于开发环境产物，不复制到 Astro 页面，视觉比较应排除这一浮层。

已人工查看移动英文首页及桌面中文目录长图。其余截图尚需逐张视觉复核；本次没有采集加载、抽屉、筛选展开、表单错误、登录后数据或真实支付状态。

### 商品详情补采

从当前公开目录读取可见商品链接，补采 `/dk-en/products/swappie-iphone-17` 与对应中文路径。输出在 `logs/gin-migration/storefront-product-baseline/`，相同宽度/语言组合共 6 张，均 HTTP 200、无页面异常、无检测到的横向溢出或坏图；累计 **84 张 / 14 个页面状态**。已人工查看英文 390px 长图。

截图固定当前可售配置和真实公开目录的只读状态，不代表不同容量/电池/颜色、grade guide 弹窗、图片切换或加购已验收。脚本新增 `--product-handle` 严格 slug 参数，用于可重复指定公开商品，仍然禁止写请求。

## 其余 URL / 状态清单

以下列出后续采集分组，不宣称已完成。保留源码中的完整路由清单，动态参数使用明确的只读样本或隔离测试夹具。

| 范围 | URL / 状态 | 采集与迁移约束 |
| --- | --- | --- |
| 商品 | /products/[handle]、/categories/[...category]、/collections/[handle] | 真实公开商品；型号、容量、颜色、成色、电池、图片、价格、无货、无效 handle |
| 列表 | /store 及查询参数 | 搜索、排序、筛选联动、空结果、分页、返回恢复；不只检查初始列表 |
| 账户 | /account、/account/profile、/account/addresses、/account/orders、/account/orders/details/[id] | 登录/注册/找回、验证、资料地址、空/有订单、无权访问；不修改旧真实客户数据 |
| 回收 | /sell/iphone/[...steps]、/sell/requests、/sell/requests/[id]、/account/buyback、/account/buyback/[id] | 步骤与回退、照片、估价/终价、各状态操作、账户兼容路径及通知 |
| 交易 | /cart、/checkout、/order/[id]/confirmed、/payment-result | 有货/空/无货购物车、登录恢复、地址配送优惠、第三方支付、错误/等待/成功；用隔离数据，禁止为截图产生真实支付或库存变动 |
| 订单转移 | /order/[id]/transfer/[token] 及 /accept、/decline | 查看是否隐含写操作后再采集；有效/过期/已使用/越权；只用隔离令牌 |
| 邮件验证 | /verify-account | 有效/过期/已使用/缺失；不消费旧用户真实令牌 |
| 演示 | /shipping-demo、/account/shipping-demo | 保持演示边界；不擅自扩成生产物流 |
| 共同交互 | 国家/语言、导航、搜索、购物车抽屉、cookie/通知、页脚、移动菜单 | 键盘、焦点、窄屏、加载/空/错误、浏览器前进后退与跨页状态 |

已通过静态生成器确认 36 个 `page.tsx` 去重后正好对应任务中的 32 种 URL，另有 7 个非页面 handler，逐项源码见 [路由模式表](frontend-routes.md)。账号 parallel slots 会重复表达 URL，但登录前后的不同状态仍须分别验证。

## 可重复采集

脚本：`retail/scripts/capture-storefront.cjs`。从仓库根执行：

```powershell
node retail/scripts/capture-storefront.cjs --playwright-root <已安装-playwright-的-node_modules-目录> --output logs/gin-migration/storefront-baseline-v2
```

脚本只允许 localhost/127.0.0.1，输出必须在 Git 忽略目录。禁止写请求，单独允许当前目录查询实际使用的只读 `POST /store/search`；不登录、不提交表单、不加购。不应把这个允许项泛化到任意 POST，也不应依赖通用 GET 限制来保证含消费令牌的页面无副作用。

下一轮按商品 → 公共交互 → 隔离登录与交易状态补齐。截图数量、API 响应与业务验收分别记录。
