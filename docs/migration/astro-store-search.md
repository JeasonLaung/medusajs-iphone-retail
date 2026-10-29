# Astro 正式商店搜索页

2026-10-29。当前 C01 的局部实现；整个前台尚未验收。

正式入口 `/{countryCode}/store`，以 `dk-en`、`dk-zh` 等原地区语言形式工作。地区/店铺配置失败返回 503，未知地区/语言返回 404。原商店 template、搜索框、筛选组件、移动面板、卡片、分页、翻译、CSS 和全局样式移植到 `retail/storefront/src/components/search`；保持原页面层级、50 条分页、卡片现货标记和选项说明。图标复制已安装 MIT SVG，不加入 Medusa 运行依赖。

## 数据和页面衔接

- React InstantSearch 保留组件状态，Next 专用包装替换为通用 InstantSearch；独立 client 将查询白名单翻译为 [Gin 搜索](gin-store-search.md)，不安装 Medusa 搜索 adapter、SDK 或 Next。
- Astro 使用 `getServerState` 在请求内完成搜索，再以同一 serverState 水合。服务端私有 Gin origin 不放入 island props。UI 保留原 query，Gin 处理型号缩写。history 及时写入，修复后退后快速切换排序可能被延迟写入吞掉的 URL 更新；原 URL 编解码完整承接，支持单组多选、range、旧嵌套链接、逗号/百分号、排序、分页和 history。
- 浏览器经固定 `/api/store/search` 只读桥接访问 Gin。禁止转发来访 Cookie/Authorization 或 upstream 响应头；固定服务端 origin/path，5 秒超时、64 KiB 请求/8 MiB 响应上限，禁止重定向，错误统一 503，不回显 upstream body。页面与接口均 no-store。
- 首屏和浏览器使用相同地区币种；原卡片价格 UI 使用 Gin 十进制字符串和已有精确格式化函数。InstantSearch 的 numeric widget 只接收显式数字投影，用于滑块/图形展示，不能用于结算或覆盖 Gin 价格排序。超出 Number 安全范围的展示数据明确失败，不冒充精确值。
- 目录完整 facet 供原选项控件保留无匹配选项；iPhone 的直接分类子节点保留零计数。当前 Gin 在一个结果里返回完整价格分布，原 24 栏直方图和滑块复用该数据，省去旧前端的逐页额外搜索。数值 facet 同时提供 key 和 stats，避免滑块默认为零。
- Skeleton 的原 `@apply bg-ui-border-base` 转换成相同 `var(--border-base)`，因为新运行时消费预编译 CSS，不运行 Tailwind；没有修改其余原 CSS。

## 验证与未完成项

单元验证请求映射、币种、过滤白名单、响应 key/顺序、错误边界、请求分批和原 URL 往返。生产 Node adapter 浏览器验证首屏已包含过滤后的商品、客户端筛选/排序/清空/刷新、分页和后退、现货标签、52 件商品的完整价格分布/上下限、中英与移动面板、窄屏溢出、依赖失败和请求大小。浏览器采用人工公开 API 数据；Gin SQL 行为另见搜索专项，不能将人工价格排序当成支付验收。

与实际隔离 Gin 的联合入口验证正式商店路由和公开依赖；其现有 fixture 没有 iPhone 分类，因此验证的是空结果与页面挂载，不声称已浏览真实商品目录。旧服务始终保留，不使用真实订单/库存/付款作为测试。

以下仍是 C01 完成前的缺口：

- 英文词干、停用词、中文边界和 `ts_rank_cd` 相关度对照；当前 Gin keyword 原型尚未达到旧搜索验收。
- 购买保障的真实店铺配置（原 warranty/return policies）尚未接入，当前使用原组件已有的“联系确认”空配置状态，不编造保障承诺。
- 原组件布局/CSS 已承接，尚需搜索页面同数据的原 Next/Gin Astro 截图对照；本次截图和交互检查不代替逐像素保真验收。
- 全局客户会话、购物车计数、地区语言持久偏好仍待与账户/交易串联；商品卡片进入详情后的完整加购/结账闭环继续按 C02—C07 执行。

运行证据以 [Astro 联合报告](astro-runtime-report.json) 为准，只有当前源文件哈希一致时才代表本次代码。全站 `full_storefront_accepted` 保持 false。
