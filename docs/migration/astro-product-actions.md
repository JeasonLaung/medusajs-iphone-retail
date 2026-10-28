# Astro 商品操作区及等级说明

2026-10-28。新增 ProductActions、原 SelectionRow/GradeBadge/GradeGuide/StickyPurchaseBar、主题及 intersection hook。供后续实际商品页挂载，生产业务路由尚未开放。旧页面、开发服务和业务库未修改。

## 原行为与依赖

操作区沿用原 DOM/CSS 与中英文案，包括容量/等级/电池/颜色、选项联动、配置不完整提示、报价/原价/Apple 首发价/演示提示、摘要、配送链接及桌面/移动固定购买栏。状态与图库使用已验证的同商品选择 hook，精确价格/差额使用 Gin 十进制字符串，未转 Number。radio 原选项顺序和不可用组合规则保留。

原等级弹窗和选项行本已使用 Ant Design；新应用锁定 antd 5.29.3 和 React 19 patch 1.0.3，保留原 ConfigProvider 主题、Modal/Button、S/A 文案、机身/屏幕切换、鼠标放大镜、Tab 循环、Escape/关闭及焦点返回。三份原组件 CSS 完整复制，未用后台风格重画前台。原等级示意图 4 张及缩略图索引明确引用的 76 张 WebP 均按原相对 URL 复制（80 文件/21,256,604 字节），运行报告逐文件核对原始字节 SHA-256；没有临时占位替代真实等级图片。

原 Next Image 已有适配器增加非 fill 图片的 width/height/draggable 支持，等级图维持 640×640/sizes 与图片布局；Next 图片优化器仍待迁移，不冒称资源链路完成。localized link 使用显式国家语言段生成原路径。原 sticky 逻辑继续观察标题及 header 尺寸，不固定猜测导航高度。

## 购物车边界

ProductActions 必须显式提供 commerce：关闭能力或具有 addToCart 的适配器。当前 Gin catalog 的 checkout=false 必须传关闭；未实现真实 Gin 购物车，因此生产不会伪造加购成功或回退旧 Medusa。后续真实适配器接入前，此组件不是可验收的完整购物流程。

适配器 UI 保留原一次加 1、加载禁用、成功数量/跳转、库存/一般错误与 cart 链接。测试适配器只存在独立 fixture，通过浏览器拦截人工 /test-cart 响应，不入生产源码。未宣称测试响应验证了持久化、会话、权限、事务、税或库存预留。

cart-quantities 使用非负整数字符串和 BigInt，累计不丢安全整数之外的位数；拒绝小数/负数/指数/前导零/超长数量，特殊变体 ID 保持普通字典键。当前可购余量取 Gin 的 max_fulfillable_quantity，而不是 pooled inventory_quantity，避免跨仓合计、共享需求或 hold 使购物车误判。跟踪库存但未知容量保守限制；背单/不跟踪仍由独立 Gin availability 门控制。这是既定库存保护的前端落实。

## 验证与剩余

[运行报告](astro-runtime-report.json)的 actions_browser 为独立人工数据的真实 Astro/React/Ant 浏览器测试。参考直接运行旧操作区、弹窗及辅助源码，仅桥接 Next navigation/image 和测试加购边界；新旧使用同一锁定 Ant/React 与受控字体。中英三宽比较初始、256GB、S 级和 A 级屏幕，共 24 对区域截图。动作文本逐字比较；截图尺寸相同，允许每个 RGB 通道最多 1/255 的合成舍入，记录实际最大差及变化像素数量，不称逐像素完全一致。Ant 动画结束后截图；固定栏另验显示和动态 header 偏移。图片比较脚本需要 Python + Pillow；Windows 沿用已有 Python，其他环境可设置 PYTHON 为解释器路径。浏览器运行器复用后台已锁定 Playwright，ADMIN_TEST_BROWSER 可指定现有浏览器，不额外下载运行器。

额外验证无候选、缺价/缺货/历史冻结/单仓限制/checkout 关闭、超安全整数金额及 0.01 差价、S/A 与图片切换、焦点循环/恢复、适配器加载/错误/库存失败/成功、精确请求及跳转。新增 3 项数量单元，累计 19 项。已有图库/规格 40 对严格截图及双岛/真实 Gin 模块串联继续回归；构建与审计另记录。

仍缺实际商品路由、全局字体/布局/SEO、客户购物车与真实加购、评论/推荐以及完整结账。实际页面视觉必须对照原真实基线另行验收。后台优先 Ant Design、图表统一 ECharts 的约定继续执行。
