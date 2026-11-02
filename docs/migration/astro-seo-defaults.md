# Astro 默认 SEO

2026-11-01。实际原站/Astro头信息审计发现：Astro缺少原站默认语言链接、Open Graph URL/公开页文字、Organization/WebSite JSON-LD、图标声明和私有页面robots。首页标题缺少来源实际后缀，商店及商品标题则多了后缀。已按来源行为补齐；后台配置覆盖与发现文件仍未完成，不称完整SEO已迁移。

来源为原 `lib/data/seo.ts`、`modules/common/components/seo-data/index.tsx` 和 `app/layout.tsx`。新增 `lib/seo.ts` 与 `components/layout/seo-head.astro`，共享Storefront及独立checkout/payment-result/sell流程head复用。保留原私有路径正则，私有页面强制noindex/nofollow，UI/物流演示保留显式robots；错误页无其他robots时noindex。canonical去查询/fragment，公开页显式商品canonical仍保留。默认en/zh-CN/x-default三链接沿原路径替换语言段；JSON-LD仍是Organization与WebSite，序列化转义<、>、&。图标逐字节复制原favicon.ico和apple-touch-icon.png。无body布局、CSS或字体调整。

原站withSEO包装器会以完整metadata title覆盖页面原OG title。实际首商品对照确认Astro商品title/OG多出的品牌后缀，现修正为来源完整标题，SKU路径也带配置title；客户切换配置时既有动态document.title和canonical行为保留，未改selectionMetadata。首页展示title带原站实际品牌后缀、OG用原copy title，商店title与OG均保留原Refurbished iPhones。账户页title依赖登录状态的剩余差异未在本轮改写。

[真实浏览器证据](seo-defaults-native-report.json)：实际Next8000与Astro9300/Gin9100/MySQL，共58次页面访问。中英的首页、商店、关于、帮助、评价、博客、文章和首商品共16组，title/description/canonical/三语言链接/OG/JSON-LD/图标全部一致，仅归一化本地localhost/127.0.0.1:8000/9300 origin，未屏蔽外部URL差异。浏览器无页面异常，无合成API响应；外部HTTPS资产阻止，不能据此称远程字体或全站像素验收。未采到可见合集链接，合集和全部SKU未据此称完整覆盖。命令：`node retail/admin/tests/seo-defaults.native.mjs`。

私有路径另读取head：账户/购物车/回收/付款结果等Astro路径具备noindex。保留实际状态差异：无购物车的原checkout404，当前Astro因主客户/交易门禁关闭503，故不将这一页称新旧业务一致或checkout联合验收。付款结果仅检查Astro缺少ID的head，未执行支付；原付款缺少上下文可能持续请求/跳转，未对其作网络空闲或业务验收承诺。测试改用文档加载等待并使用有效回收model路径，不靠网络空闲掩盖这些边界。

前台209测试全部通过，Astro check 451文件、0错误/警告、17提示。新测试覆盖默认canonical/语言、私有优先级、产品OG图片与安全JSON-LD；既有variants中错误的页面原OG预期按实际withSEO包装器修正，客户动态选择预期未变。9001健康/Admin、9100健康、Astro商店/关于/图标200，初始商店30款，3语言链接和1个JSON-LD；近期非空服务日志无panic/fatal/意外5xx。无backend/schema/env/门禁/binary变化，不重复进行与本轮无关的SQL迁移回归。

后续仍缺Gin公开SEO配置和页面覆盖读取、React Admin必要配置、导入/受保护配置审计、动态site origin、robots.txt/llms.txt/sitemap及分片、Bing/IndexNow验证文件、配置GA/GSC行为和真实配置联合。不能以默认常量代替这些现有可配置功能。SEO失败时保持原站可购物的fallback行为也需要配置接入后验收。C01地区交互和其余状态、C02–C11整体闭环继续保持未完成。
