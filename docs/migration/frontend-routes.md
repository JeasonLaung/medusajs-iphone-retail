# 前台路由模式与实现状态入口

由 `retail/scripts/inventory-business.py` 生成。路由组与 parallel slot 从 URL 去除，保留各 slot 的源码证据；本表不证明页面已采集或迁移。

共 36 个 page 文件、32 个去重页面模式，以及 7 个 route handler。

`[countryCode]` 实际使用国家-语言组合（例如 dk-en / dk-zh），不是仅国家码。账号 layout 根据 retrieveCustomer 选择 dashboard/login slot；某个 URL 缺少 login 实现时不能假设有完整登录前页面。

| URL 模式 | 实现入口 / 状态 |
| --- | --- |
| `/[countryCode]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/page.tsx) |
| `/[countryCode]/about` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/about/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/about/page.tsx) |
| `/[countryCode]/account` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/page.tsx)；login: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/page.tsx) |
| `/[countryCode]/account/addresses` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/addresses/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/addresses/page.tsx) |
| `/[countryCode]/account/buyback` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/buyback/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/buyback/page.tsx)；login: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/buyback/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/buyback/page.tsx) |
| `/[countryCode]/account/buyback/[id]` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/buyback/[id]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/buyback/%5Bid%5D/page.tsx)；login: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/buyback/[id]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/buyback/%5Bid%5D/page.tsx) |
| `/[countryCode]/account/orders` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/orders/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/orders/page.tsx) |
| `/[countryCode]/account/orders/details/[id]` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/orders/details/[id]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/orders/details/%5Bid%5D/page.tsx) |
| `/[countryCode]/account/profile` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/profile/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/profile/page.tsx) |
| `/[countryCode]/account/shipping-demo` | dashboard: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/shipping-demo/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/shipping-demo/page.tsx)；login: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/shipping-demo/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/shipping-demo/page.tsx) |
| `/[countryCode]/blog` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/blog/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/blog/page.tsx) |
| `/[countryCode]/blog/[slug]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/blog/[slug]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/blog/%5Bslug%5D/page.tsx) |
| `/[countryCode]/cart` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/cart/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/cart/page.tsx) |
| `/[countryCode]/categories/[...category]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/categories/[...category]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/categories/%5B...category%5D/page.tsx) |
| `/[countryCode]/checkout` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(checkout)/checkout/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28checkout%29/checkout/page.tsx) |
| `/[countryCode]/collections/[handle]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/collections/[handle]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/collections/%5Bhandle%5D/page.tsx) |
| `/[countryCode]/help` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/help/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/help/page.tsx) |
| `/[countryCode]/order/[id]/confirmed` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/confirmed/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/confirmed/page.tsx) |
| `/[countryCode]/order/[id]/transfer/[token]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/transfer/[token]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/transfer/%5Btoken%5D/page.tsx) |
| `/[countryCode]/order/[id]/transfer/[token]/accept` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/transfer/[token]/accept/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/transfer/%5Btoken%5D/accept/page.tsx) |
| `/[countryCode]/order/[id]/transfer/[token]/decline` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/transfer/[token]/decline/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/transfer/%5Btoken%5D/decline/page.tsx) |
| `/[countryCode]/payment-result` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(payment)/payment-result/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28payment%29/payment-result/page.tsx) |
| `/[countryCode]/products/[handle]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/products/[handle]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/products/%5Bhandle%5D/page.tsx) |
| `/[countryCode]/reviews` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/reviews/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/reviews/page.tsx) |
| `/[countryCode]/sell` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/page.tsx) |
| `/[countryCode]/sell/iphone/[...steps]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(sell-flow)/sell/iphone/[...steps]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28sell-flow%29/sell/iphone/%5B...steps%5D/page.tsx) |
| `/[countryCode]/sell/requests` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/requests/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/requests/page.tsx) |
| `/[countryCode]/sell/requests/[id]` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/requests/[id]/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/requests/%5Bid%5D/page.tsx) |
| `/[countryCode]/shipping-demo` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/shipping-demo/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/shipping-demo/page.tsx) |
| `/[countryCode]/store` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/store/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/store/page.tsx) |
| `/[countryCode]/ui` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/ui/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/ui/page.tsx) |
| `/[countryCode]/verify-account` | page: [medusa-store/apps/storefront/src/app/[countryCode]/(main)/verify-account/page.tsx](../../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/verify-account/page.tsx) |

## 非页面入口

GET 不自动等于无副作用；支付/验证/转移令牌入口采集前需审查。SEO 文件与支付兼容 URL 也属于 Astro 迁移范围。

| URL | 方法 | 源码 |
| --- | --- | --- |
| `/api/mollie-return` | GET, POST | [medusa-store/apps/storefront/src/app/api/mollie-return/route.ts](../../medusa-store/apps/storefront/src/app/api/mollie-return/route.ts) |
| `/api/payment-return` | GET | [medusa-store/apps/storefront/src/app/api/payment-return/route.ts](../../medusa-store/apps/storefront/src/app/api/payment-return/route.ts) |
| `/api/seo-artifacts/[key]` | GET | [medusa-store/apps/storefront/src/app/api/seo-artifacts/[key]/route.ts](../../medusa-store/apps/storefront/src/app/api/seo-artifacts/%5Bkey%5D/route.ts) |
| `/BingSiteAuth.xml` | GET | [medusa-store/apps/storefront/src/app/BingSiteAuth.xml/route.ts](../../medusa-store/apps/storefront/src/app/BingSiteAuth.xml/route.ts) |
| `/llms.txt` | GET | [medusa-store/apps/storefront/src/app/llms.txt/route.ts](../../medusa-store/apps/storefront/src/app/llms.txt/route.ts) |
| `/robots.txt` | GET | [medusa-store/apps/storefront/src/app/robots.txt/route.ts](../../medusa-store/apps/storefront/src/app/robots.txt/route.ts) |
| `/sitemap.xml` | GET | [medusa-store/apps/storefront/src/app/sitemap.xml/route.ts](../../medusa-store/apps/storefront/src/app/sitemap.xml/route.ts) |
