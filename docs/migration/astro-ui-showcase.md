# Astro UI 演示页

2026-11-01。C01的`/[countryCode]/ui`原来在Next返回200、Astro返回404，导致现有导航中的UI入口失效。现已接入Astro正式路由，复用Gin公开地区/语言/页脚读取，保留原七个演示区域。未知国家/语言仍404，依赖错误沿用503边界。

来源是`medusa-store/apps/storefront/src/modules/ui-showcase`及其原common组件。主CSS和中英文文案逐字节复制；按钮、国家搜索、选择行、评分、面包屑、商品缺省图和骨架复用已有迁移组件。缺少的common基础控件、浮动输入、原生选择、复选框、排序、成色标签、通用弹窗及演示金额汇总放在showcase的局部目录，保留来源说明，不替换现有购物或账户组件。原外层main改为div以适配Astro共享main，CSS类不变。

原站使用的Ant Design API与当前Astro依赖不同：Alert的title改为当前版本的message，Timeline项的content改为children；类型导入按当前TypeScript要求调整。账户IconButton沿用原Button透明变体包装，不安装Medusa运行时或增加依赖。Ant控件使用已有StorefrontTheme及项目语义配色；原展示CSS不作重新设计。

页面保持演示边界：数据、选择、分页及表单成功状态仅保存在本页React状态；上传使用原`beforeUpload={() => false}`，图片只在浏览器预览。金额汇总是原固定示例数字，未接实际购物车、订单、库存或支付命令。刷新重置选择，不能将表单成功提示当作业务保存。页面保留noindex/nofollow，并给出当前地区的canonical；共享布局新增可选robots参数，未传入的已有页面不受影响。

本地新旧开发服务可用时，从仓库根运行`node --experimental-strip-types retail/admin/tests/ui-showcase.native.mjs`。脚本使用已安装Chrome及Admin的Playwright，报告/截图仅写入Git忽略的`logs/gin-migration/ui-showcase-*`，不要求登录或业务写权限。

验收见[实际浏览器/数据库报告](ui-showcase-native-report.json)。使用实际Next8000及Astro9300/Gin9100/持久99迁移MySQL，中英×390/1440共8组。验证七区标题/原shared初始文字对照、按钮反馈、国家搜索、有效PNG本地缩略图解码、评分、原生选择、单选/多选/禁用、分页/时间轴、空值及错误邮箱校验、成功/重置、两种弹窗焦点与Escape、刷新选择重置及无横向溢出。浏览器拦截外部HTTPS资源，不合成任何API结果；因此不据此称远程字体/旗帜图片或逐像素全部验收。

测试分别检查Headless UI实际可见面板和焦点入窗时序，而非零尺寸dialog根；两字中文按钮按Ant插入空格后的无障碍名称匹配。源站四组及Astro四组使用相同操作。业务非GET/HEAD/OPTIONS请求记录必须为空；原站开发工具偶发的单一`POST /__nextjs_original-stack-frames`是读取源码堆栈的诊断，按原站8000精确来源和路径独立记录，不泛化允许其他POST。另检查中英文手机/桌面从导航键盘进入UI并通过面包屑回首页。

数据核对沿用切换后逐表指纹和213项外键的只读验证，证明主业务事实及保留原86库不变；未改变后端源码、数据库门禁、服务端口或运行二进制。类型检查、原商店30款SSR、评价页SEO未被演示robots污染及新旧服务健康检查一并记录。没有在运行中的checkout构建或清理输出。

尚未交付物流演示、完整SEO及C01其他状态，也不替代C02–C11的账户、购物支付、履约、售后、回收与通知完整联合验收。
