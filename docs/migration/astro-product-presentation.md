# 商品图库与技术规格组件

2026-10-28。将原 ImageGallery、ProductDetails、中英文商品文案及商品 CSS 迁入 retail/storefront/src/components/products。新组件只接收明确 Gin Product DTO；不安装 Next/Medusa 运行时，不从旧服务代理数据。尚未挂载到生产商品路由，整体商品页仍未完成。

## 保留与替换

原 CSS 内容完整保留，末尾仅增加现有 inline-block/align-middle/shrink-0/pb-6/mb-6 的显式 CSS 等价规则；文案文件逐字保留。此处是原样迁移，未顺带调整旧页面配色、圆角或布局。后续页面挂载仍需承接原全局样式、Outfit/DM Sans 字体、1200px 布局与共享导航。

图库保留 stage、计数、上一张/下一张循环、缩略图与 aria-pressed、空图提示、图片 alt/sizes、选中变体改变后复位。使用上一批 useProductSelection 同步购买岛和图库；模型初始图不因默认购买变体悄然替换，用户主动选择则按变体图或模型回退。一个图片时不显示箭头/缩略图。

Next Image 替换为原生 img：维持 fill 绝对定位、contain、padding、主图 eager/high priority 和缩略图 lazy，使用已投影公开图片 URL。未迁移 Next 优化器的多尺寸 srcset/格式转换或图片代理缓存；性能及最终资源路径仍须在真实页面验证。三个图标复用现有 @medusajs/icons 2.21.2 SVG 路径并保留 MIT 许可证，不引入整包。

技术规格读取 display.model_specs，避免消费任意 metadata；保留三项亮点、11 类规格行、尺寸完整后组合、描述原样文本和换行、默认打开的规格折叠与官方链接。缺数据时隐藏对应块，描述中的 HTML 不执行；链接仅允许原有 Apple/support.apple.com HTTPS 格式，继续 target=_blank + noopener noreferrer。传输层另有更严格 URL 验证。

## 真实浏览器组件对照

[运行报告](astro-runtime-report.json)新增 presentation_browser；入口 tests/presentation.browser.mjs。独立测试根目录不属于生产 src/pages，使用人工机型/规格和本地 SVG，测试完成关闭自有随机监听和浏览器，不写旧业务。

对照直接读取旧组件函数体和旧 CSS，以独立虚拟模块运行；只桥接 Next navigation/image 与类型导入，图标读取旧安装包。旧选择 helper 和文案亦直接读取原文件。不会手写第二份预期 DOM 来代替原实现；报告记录原组件、CSS、文案、lock 与新实现哈希。运行该对照需要原仓库的已锁定依赖以及后台的 Playwright。

英文/中文 × 390/768/1440px，各比较初始、缩略图切换、描述展开；另比较空图/空规格和非官方 source，共 40 对区域截图（80 张），每对 PNG 字节一致。人工查看英文移动图库和中文移动规格展开图。额外验证循环/缩略图、选型后图片数量变化及空变体图回退/索引复位、外链属性、文字转义、键盘可见焦点、无横向溢出与零水合错误。截图仅留忽略的 logs/gin-migration/astro-presentation，报告只存哈希。

这证明受控字体和人工数据下两个组件的渲染/交互，不代表旧 Next 优化图片、真实照片、全局字体、共享布局或完整商品页已验收。实际路由、操作区/等级弹窗/购物车、评论推荐、SEO 与真实资料的页面对照继续迁移。后台优先 Ant Design、图表统一 ECharts 的约定不改变前台原样迁移要求。
