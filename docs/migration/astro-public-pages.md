# 正式公开页面与客服读取（2026-10-29）

本批继续[当前交付范围](current-delivery-scope.md) C01，新增 `/[countryCode]/about`、`/help`、`/blog`、`/blog/[slug]` 四种正式路由模式。文章沿用原来的三个 slug：`choose-your-storage`、`compare-iphone-configurations`、`before-you-order`，中英文全文、图片、章节、返回博客和商店链接均从原实现迁移。

来源与修改边界：

- `medusa-store/apps/storefront/src/modules/layout/templates/information-page`：关于/帮助正文、锚点、链接和原 CSS；将 Next locale/data loader 改为显式 Gin 数据 props。外层内容容器使用 div，避免与 Astro 共享布局的 main 嵌套。
- `modules/home/components/blog-section/index.tsx`、`home-sections.module.css`、`modules/home/i18n.ts` 和原 `app/[countryCode]/(main)/blog/[slug]/page.tsx`：博客/文章结构、图片和全部文案；替换 Next 图片、链接与图标导入。CSS 与翻译原样复制。
- `modules/customer-service/{index.tsx,contact.ts,i18n.ts,styles.module.css}`：原组件直接复制，保留售前/售后分组、WhatsApp/邮箱、营业时间/时区、人工在线状态、未配置和加载失败文案。服务状态在客户端按原逻辑每分钟刷新。

Gin 新增只读 `GET /api/store/customer-service`，从服务端绑定的店铺读取 `customer_service_contacts`。不接收客户端店铺选择；只返回启用联系人及公开字段，不返回 enabled 或整个 metadata。沿用原配置整组校验及未配置返回空数组的语义；数据库/店铺不可用返回 503。读取明确禁止缓存，不设置登录 Cookie，也不修改配置。此接口服务于前台帮助页，不表示客服配置的后台编辑已经迁完。

Astro 用 Gin 地区验证当前国家，复用地区/语言/页脚数据组装；未知文章和国家/语言组合为 404，布局依赖失败为 503。与原帮助页一致，客服单项依赖失败显示原失败提示，同时保留购物/账户帮助正文。

本批验证包括：62 项前台单元测试、Astro 类型检查/生产构建、正式构建页面的中英浏览器导航和安全链接、23 组隔离 MySQL/Redis 联合测试（包含新客服匿名读取/禁用过滤/店铺范围/只读检查）、Go 全包测试与相关 vet。图片及规格原组件在本机 Chrome 的原源码对照通过；旧版 headless shell 曾出现选中缩略图边缘 9 像素差异，未改变生产 CSS 或放宽比较阈值。

随后完整 `test-admin-live.py` 通过，新增正式路由与真实隔离 Gin 的 SSR 检查、人工数据正式页面浏览器检查，以及原图库/规格/购买区/购物车/页脚/页头对照均完成；详见 [Astro 验证报告](astro-runtime-report.json)。独占数据库、Redis、网络和临时进程均清理。报告明确保留 `full_storefront_accepted=false`，不能把本批回归通过解释为购物/回收全站闭环已完成。

仍未完成：商店搜索筛选（原每页 50、InstantSearch 状态/分面/价格区间与分布，见 [搜索契约](search-contract.md)）、首页、分类/合集、评价、客户账户、购物车到支付/订单、回收及售后闭环。商店/账户链接保留原目标；目标页面尚缺不算功能闭环完成。新页头仍是匿名展示，真实账户/购物车数量和换区保存继续待接入。
