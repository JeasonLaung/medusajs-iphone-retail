# 迁移源码盘点清单

日期：2026-10-26。由当前源码扫描生成，供迁移追踪，不表示这些功能已在新系统实现。

## 1. 前台页面 URL 与源文件

`[countryCode]` 当前承载国家与语言组合，例如 `dk-en`/`dk-zh`；实际入口兼容与重定向见 middleware。以下合并 Next route group 和 parallel route，36 个页面文件对应 32 个 URL 模式。

| URL 模式 | 当前源文件 |
| --- | --- |
| `/[countryCode]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/page.tsx) |
| `/[countryCode]/about` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/about/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/about/page.tsx) |
| `/[countryCode]/account` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/page.tsx)<br>[medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/page.tsx) |
| `/[countryCode]/account/addresses` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/addresses/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/addresses/page.tsx) |
| `/[countryCode]/account/buyback` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/buyback/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/buyback/page.tsx)<br>[medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/buyback/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/buyback/page.tsx) |
| `/[countryCode]/account/buyback/[id]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/buyback/[id]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/buyback/%5Bid%5D/page.tsx)<br>[medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/buyback/[id]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/buyback/%5Bid%5D/page.tsx) |
| `/[countryCode]/account/orders` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/orders/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/orders/page.tsx) |
| `/[countryCode]/account/orders/details/[id]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/orders/details/[id]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/orders/details/%5Bid%5D/page.tsx) |
| `/[countryCode]/account/profile` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/profile/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/profile/page.tsx) |
| `/[countryCode]/account/shipping-demo` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/shipping-demo/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/shipping-demo/page.tsx)<br>[medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/shipping-demo/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/shipping-demo/page.tsx) |
| `/[countryCode]/blog` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/blog/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/blog/page.tsx) |
| `/[countryCode]/blog/[slug]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/blog/[slug]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/blog/%5Bslug%5D/page.tsx) |
| `/[countryCode]/cart` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/cart/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/cart/page.tsx) |
| `/[countryCode]/categories/[...category]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/categories/[...category]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/categories/%5B...category%5D/page.tsx) |
| `/[countryCode]/checkout` | [medusa-store/apps/storefront/src/app/[countryCode]/(checkout)/checkout/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28checkout%29/checkout/page.tsx) |
| `/[countryCode]/collections/[handle]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/collections/[handle]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/collections/%5Bhandle%5D/page.tsx) |
| `/[countryCode]/help` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/help/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/help/page.tsx) |
| `/[countryCode]/order/[id]/confirmed` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/confirmed/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/confirmed/page.tsx) |
| `/[countryCode]/order/[id]/transfer/[token]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/transfer/[token]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/transfer/%5Btoken%5D/page.tsx) |
| `/[countryCode]/order/[id]/transfer/[token]/accept` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/transfer/[token]/accept/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/transfer/%5Btoken%5D/accept/page.tsx) |
| `/[countryCode]/order/[id]/transfer/[token]/decline` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/transfer/[token]/decline/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/transfer/%5Btoken%5D/decline/page.tsx) |
| `/[countryCode]/payment-result` | [medusa-store/apps/storefront/src/app/[countryCode]/(payment)/payment-result/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28payment%29/payment-result/page.tsx) |
| `/[countryCode]/products/[handle]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/products/[handle]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/products/%5Bhandle%5D/page.tsx) |
| `/[countryCode]/reviews` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/reviews/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/reviews/page.tsx) |
| `/[countryCode]/sell` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/page.tsx) |
| `/[countryCode]/sell/iphone/[...steps]` | [medusa-store/apps/storefront/src/app/[countryCode]/(sell-flow)/sell/iphone/[...steps]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28sell-flow%29/sell/iphone/%5B...steps%5D/page.tsx) |
| `/[countryCode]/sell/requests` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/requests/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/requests/page.tsx) |
| `/[countryCode]/sell/requests/[id]` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/requests/[id]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/requests/%5Bid%5D/page.tsx) |
| `/[countryCode]/shipping-demo` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/shipping-demo/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/shipping-demo/page.tsx) |
| `/[countryCode]/store` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/store/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/store/page.tsx) |
| `/[countryCode]/ui` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/ui/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/ui/page.tsx) |
| `/[countryCode]/verify-account` | [medusa-store/apps/storefront/src/app/[countryCode]/(main)/verify-account/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/verify-account/page.tsx) |

## 2. 前台端点与服务端数据层

这些 Next 路由不在后端 67 个自定义 API 文件统计中，也需要迁移或兼容。

| 路由文件 | 方法 |
| --- | --- |
| [medusa-store/apps/storefront/src/app/api/mollie-return/route.ts](../medusa-store/apps/storefront/src/app/api/mollie-return/route.ts) | GET, POST |
| [medusa-store/apps/storefront/src/app/api/payment-return/route.ts](../medusa-store/apps/storefront/src/app/api/payment-return/route.ts) | GET |
| [medusa-store/apps/storefront/src/app/api/seo-artifacts/[key]/route.ts](../medusa-store/apps/storefront/src/app/api/seo-artifacts/%5Bkey%5D/route.ts) | GET |
| [medusa-store/apps/storefront/src/app/BingSiteAuth.xml/route.ts](../medusa-store/apps/storefront/src/app/BingSiteAuth.xml/route.ts) | GET |
| [medusa-store/apps/storefront/src/app/llms.txt/route.ts](../medusa-store/apps/storefront/src/app/llms.txt/route.ts) | GET |
| [medusa-store/apps/storefront/src/app/robots.txt/route.ts](../medusa-store/apps/storefront/src/app/robots.txt/route.ts) | GET |
| [medusa-store/apps/storefront/src/app/sitemap.xml/route.ts](../medusa-store/apps/storefront/src/app/sitemap.xml/route.ts) | GET |

服务端数据文件：

- [medusa-store/apps/storefront/src/lib/data/after-sales.ts](../medusa-store/apps/storefront/src/lib/data/after-sales.ts)
- [medusa-store/apps/storefront/src/lib/data/buy-policies.ts](../medusa-store/apps/storefront/src/lib/data/buy-policies.ts)
- [medusa-store/apps/storefront/src/lib/data/buyback.ts](../medusa-store/apps/storefront/src/lib/data/buyback.ts)
- [medusa-store/apps/storefront/src/lib/data/cart.ts](../medusa-store/apps/storefront/src/lib/data/cart.ts)
- [medusa-store/apps/storefront/src/lib/data/categories.ts](../medusa-store/apps/storefront/src/lib/data/categories.ts)
- [medusa-store/apps/storefront/src/lib/data/collections.ts](../medusa-store/apps/storefront/src/lib/data/collections.ts)
- [medusa-store/apps/storefront/src/lib/data/cookies.ts](../medusa-store/apps/storefront/src/lib/data/cookies.ts)
- [medusa-store/apps/storefront/src/lib/data/country-select-translations.ts](../medusa-store/apps/storefront/src/lib/data/country-select-translations.ts)
- [medusa-store/apps/storefront/src/lib/data/customer-service.ts](../medusa-store/apps/storefront/src/lib/data/customer-service.ts)
- [medusa-store/apps/storefront/src/lib/data/customer.ts](../medusa-store/apps/storefront/src/lib/data/customer.ts)
- [medusa-store/apps/storefront/src/lib/data/fulfillment.ts](../medusa-store/apps/storefront/src/lib/data/fulfillment.ts)
- [medusa-store/apps/storefront/src/lib/data/locale-actions.ts](../medusa-store/apps/storefront/src/lib/data/locale-actions.ts)
- [medusa-store/apps/storefront/src/lib/data/locales.ts](../medusa-store/apps/storefront/src/lib/data/locales.ts)
- [medusa-store/apps/storefront/src/lib/data/mollie.ts](../medusa-store/apps/storefront/src/lib/data/mollie.ts)
- [medusa-store/apps/storefront/src/lib/data/notifications.ts](../medusa-store/apps/storefront/src/lib/data/notifications.ts)
- [medusa-store/apps/storefront/src/lib/data/onboarding.ts](../medusa-store/apps/storefront/src/lib/data/onboarding.ts)
- [medusa-store/apps/storefront/src/lib/data/orders.ts](../medusa-store/apps/storefront/src/lib/data/orders.ts)
- [medusa-store/apps/storefront/src/lib/data/payment.ts](../medusa-store/apps/storefront/src/lib/data/payment.ts)
- [medusa-store/apps/storefront/src/lib/data/product-reviews.ts](../medusa-store/apps/storefront/src/lib/data/product-reviews.ts)
- [medusa-store/apps/storefront/src/lib/data/products.ts](../medusa-store/apps/storefront/src/lib/data/products.ts)
- [medusa-store/apps/storefront/src/lib/data/regions.ts](../medusa-store/apps/storefront/src/lib/data/regions.ts)
- [medusa-store/apps/storefront/src/lib/data/sell-models.ts](../medusa-store/apps/storefront/src/lib/data/sell-models.ts)
- [medusa-store/apps/storefront/src/lib/data/seo.ts](../medusa-store/apps/storefront/src/lib/data/seo.ts)
- [medusa-store/apps/storefront/src/lib/data/store-search.ts](../medusa-store/apps/storefront/src/lib/data/store-search.ts)
- [medusa-store/apps/storefront/src/lib/data/variants.ts](../medusa-store/apps/storefront/src/lib/data/variants.ts)

## 3. 自定义后端 API

共 67 个 route.ts 文件；仅列本地自定义实现。Medusa 内置与插件 API 另需按前后台实际调用补齐，不能把本表当作全部 API。

| 路径模式 | 方法 | 源文件 |
| --- | --- | --- |
| `/admin/buy-policies` | GET, POST | [medusa-store/apps/backend/src/api/admin/buy-policies/route.ts](../medusa-store/apps/backend/src/api/admin/buy-policies/route.ts) |
| `/admin/buyback/[id]/photos/[photo_id]` | GET | [medusa-store/apps/backend/src/api/admin/buyback/[id]/photos/[photo_id]/route.ts](../medusa-store/apps/backend/src/api/admin/buyback/%5Bid%5D/photos/%5Bphoto_id%5D/route.ts) |
| `/admin/buyback/[id]` | GET, POST | [medusa-store/apps/backend/src/api/admin/buyback/[id]/route.ts](../medusa-store/apps/backend/src/api/admin/buyback/%5Bid%5D/route.ts) |
| `/admin/buyback` | GET | [medusa-store/apps/backend/src/api/admin/buyback/route.ts](../medusa-store/apps/backend/src/api/admin/buyback/route.ts) |
| `/admin/buyback-catalog` | GET, POST | [medusa-store/apps/backend/src/api/admin/buyback-catalog/route.ts](../medusa-store/apps/backend/src/api/admin/buyback-catalog/route.ts) |
| `/admin/buyback-prices` | GET, POST | [medusa-store/apps/backend/src/api/admin/buyback-prices/route.ts](../medusa-store/apps/backend/src/api/admin/buyback-prices/route.ts) |
| `/admin/custom` | GET | [medusa-store/apps/backend/src/api/admin/custom/route.ts](../medusa-store/apps/backend/src/api/admin/custom/route.ts) |
| `/admin/customer-service` | GET, POST | [medusa-store/apps/backend/src/api/admin/customer-service/route.ts](../medusa-store/apps/backend/src/api/admin/customer-service/route.ts) |
| `/admin/devices/import-template` | GET | [medusa-store/apps/backend/src/api/admin/devices/import-template/route.ts](../medusa-store/apps/backend/src/api/admin/devices/import-template/route.ts) |
| `/admin/devices/mobile-intake` | POST | [medusa-store/apps/backend/src/api/admin/devices/mobile-intake/route.ts](../medusa-store/apps/backend/src/api/admin/devices/mobile-intake/route.ts) |
| `/admin/devices/photos` | POST | [medusa-store/apps/backend/src/api/admin/devices/photos/route.ts](../medusa-store/apps/backend/src/api/admin/devices/photos/route.ts) |
| `/admin/devices/refunds` | POST | [medusa-store/apps/backend/src/api/admin/devices/refunds/route.ts](../medusa-store/apps/backend/src/api/admin/devices/refunds/route.ts) |
| `/admin/devices` | GET, POST | [medusa-store/apps/backend/src/api/admin/devices/route.ts](../medusa-store/apps/backend/src/api/admin/devices/route.ts) |
| `/admin/devices/supplier-accounts` | GET, POST | [medusa-store/apps/backend/src/api/admin/devices/supplier-accounts/route.ts](../medusa-store/apps/backend/src/api/admin/devices/supplier-accounts/route.ts) |
| `/admin/devices/variants` | GET | [medusa-store/apps/backend/src/api/admin/devices/variants/route.ts](../medusa-store/apps/backend/src/api/admin/devices/variants/route.ts) |
| `/admin/inventory-items` | GET（const 导出，包含原生接口委托） | [medusa-store/apps/backend/src/api/admin/inventory-items/route.ts](../medusa-store/apps/backend/src/api/admin/inventory-items/route.ts) |
| `/admin/mail/commands` | POST | [medusa-store/apps/backend/src/api/admin/mail/commands/route.ts](../medusa-store/apps/backend/src/api/admin/mail/commands/route.ts) |
| `/admin/mail/messages/[id]/attachments/[attachment_id]` | GET | [medusa-store/apps/backend/src/api/admin/mail/messages/[id]/attachments/[attachment_id]/route.ts](../medusa-store/apps/backend/src/api/admin/mail/messages/%5Bid%5D/attachments/%5Battachment_id%5D/route.ts) |
| `/admin/mail/messages/[id]` | GET | [medusa-store/apps/backend/src/api/admin/mail/messages/[id]/route.ts](../medusa-store/apps/backend/src/api/admin/mail/messages/%5Bid%5D/route.ts) |
| `/admin/mail` | GET | [medusa-store/apps/backend/src/api/admin/mail/route.ts](../medusa-store/apps/backend/src/api/admin/mail/route.ts) |
| `/admin/mollie` | GET, POST | [medusa-store/apps/backend/src/api/admin/mollie/route.ts](../medusa-store/apps/backend/src/api/admin/mollie/route.ts) |
| `/admin/orders/[id]/after-sales` | GET, POST | [medusa-store/apps/backend/src/api/admin/orders/[id]/after-sales/route.ts](../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/after-sales/route.ts) |
| `/admin/orders/[id]/cancel` | POST | [medusa-store/apps/backend/src/api/admin/orders/[id]/cancel/route.ts](../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/cancel/route.ts) |
| `/admin/orders/[id]/fulfillments/[fulfillment_id]/cancel` | POST | [medusa-store/apps/backend/src/api/admin/orders/[id]/fulfillments/[fulfillment_id]/cancel/route.ts](../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/fulfillments/%5Bfulfillment_id%5D/cancel/route.ts) |
| `/admin/orders/[id]/fulfillments/[fulfillment_id]/shipments` | POST | [medusa-store/apps/backend/src/api/admin/orders/[id]/fulfillments/[fulfillment_id]/shipments/route.ts](../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/fulfillments/%5Bfulfillment_id%5D/shipments/route.ts) |
| `/admin/orders/[id]/fulfillments/[fulfillment_id]/tracking` | POST | [medusa-store/apps/backend/src/api/admin/orders/[id]/fulfillments/[fulfillment_id]/tracking/route.ts](../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/fulfillments/%5Bfulfillment_id%5D/tracking/route.ts) |
| `/admin/orders/[id]` | GET | [medusa-store/apps/backend/src/api/admin/orders/[id]/route.ts](../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/route.ts) |
| `/admin/orders` | GET | [medusa-store/apps/backend/src/api/admin/orders/route.ts](../medusa-store/apps/backend/src/api/admin/orders/route.ts) |
| `/admin/product-reviews/[id]` | POST | [medusa-store/apps/backend/src/api/admin/product-reviews/[id]/route.ts](../medusa-store/apps/backend/src/api/admin/product-reviews/%5Bid%5D/route.ts) |
| `/admin/product-reviews` | GET | [medusa-store/apps/backend/src/api/admin/product-reviews/route.ts](../medusa-store/apps/backend/src/api/admin/product-reviews/route.ts) |
| `/admin/seo/[resource]` | GET | [medusa-store/apps/backend/src/api/admin/seo/[resource]/route.ts](../medusa-store/apps/backend/src/api/admin/seo/%5Bresource%5D/route.ts) |
| `/admin/seo/commands` | POST | [medusa-store/apps/backend/src/api/admin/seo/commands/route.ts](../medusa-store/apps/backend/src/api/admin/seo/commands/route.ts) |
| `/admin/shipping-demo/dhl` | GET | [medusa-store/apps/backend/src/api/admin/shipping-demo/dhl/route.ts](../medusa-store/apps/backend/src/api/admin/shipping-demo/dhl/route.ts) |
| `/admin/stock-locations/[id]` | GET | [medusa-store/apps/backend/src/api/admin/stock-locations/[id]/route.ts](../medusa-store/apps/backend/src/api/admin/stock-locations/%5Bid%5D/route.ts) |
| `/admin/stock-locations` | GET | [medusa-store/apps/backend/src/api/admin/stock-locations/route.ts](../medusa-store/apps/backend/src/api/admin/stock-locations/route.ts) |
| `/admin/warehouse-access/me` | GET | [medusa-store/apps/backend/src/api/admin/warehouse-access/me/route.ts](../medusa-store/apps/backend/src/api/admin/warehouse-access/me/route.ts) |
| `/admin/warehouse-access` | GET, POST | [medusa-store/apps/backend/src/api/admin/warehouse-access/route.ts](../medusa-store/apps/backend/src/api/admin/warehouse-access/route.ts) |
| `/admin/webhook-notifications/[id]` | POST, DELETE | [medusa-store/apps/backend/src/api/admin/webhook-notifications/[id]/route.ts](../medusa-store/apps/backend/src/api/admin/webhook-notifications/%5Bid%5D/route.ts) |
| `/admin/webhook-notifications/[id]/test` | POST | [medusa-store/apps/backend/src/api/admin/webhook-notifications/[id]/test/route.ts](../medusa-store/apps/backend/src/api/admin/webhook-notifications/%5Bid%5D/test/route.ts) |
| `/admin/webhook-notifications` | GET, POST | [medusa-store/apps/backend/src/api/admin/webhook-notifications/route.ts](../medusa-store/apps/backend/src/api/admin/webhook-notifications/route.ts) |
| `/mail/unsubscribe/[token]` | GET, POST | [medusa-store/apps/backend/src/api/mail/unsubscribe/[token]/route.ts](../medusa-store/apps/backend/src/api/mail/unsubscribe/%5Btoken%5D/route.ts) |
| `/mail/webhooks/[account_id]` | POST | [medusa-store/apps/backend/src/api/mail/webhooks/[account_id]/route.ts](../medusa-store/apps/backend/src/api/mail/webhooks/%5Baccount_id%5D/route.ts) |
| `/mobile-intake/devices/[id]` | DELETE | [medusa-store/apps/backend/src/api/mobile-intake/devices/[id]/route.ts](../medusa-store/apps/backend/src/api/mobile-intake/devices/%5Bid%5D/route.ts) |
| `/mobile-intake/photos` | POST | [medusa-store/apps/backend/src/api/mobile-intake/photos/route.ts](../medusa-store/apps/backend/src/api/mobile-intake/photos/route.ts) |
| `/mobile-intake` | GET, POST | [medusa-store/apps/backend/src/api/mobile-intake/route.ts](../medusa-store/apps/backend/src/api/mobile-intake/route.ts) |
| `/mobile-intake/variants` | GET | [medusa-store/apps/backend/src/api/mobile-intake/variants/route.ts](../medusa-store/apps/backend/src/api/mobile-intake/variants/route.ts) |
| `/store/buy-policies` | GET | [medusa-store/apps/backend/src/api/store/buy-policies/route.ts](../medusa-store/apps/backend/src/api/store/buy-policies/route.ts) |
| `/store/buyback/estimate` | POST | [medusa-store/apps/backend/src/api/store/buyback/estimate/route.ts](../medusa-store/apps/backend/src/api/store/buyback/estimate/route.ts) |
| `/store/buyback/prices` | GET | [medusa-store/apps/backend/src/api/store/buyback/prices/route.ts](../medusa-store/apps/backend/src/api/store/buyback/prices/route.ts) |
| `/store/buyback/requests/[id]/photos/[photo_id]` | GET | [medusa-store/apps/backend/src/api/store/buyback/requests/[id]/photos/[photo_id]/route.ts](../medusa-store/apps/backend/src/api/store/buyback/requests/%5Bid%5D/photos/%5Bphoto_id%5D/route.ts) |
| `/store/buyback/requests/[id]/photos` | POST | [medusa-store/apps/backend/src/api/store/buyback/requests/[id]/photos/route.ts](../medusa-store/apps/backend/src/api/store/buyback/requests/%5Bid%5D/photos/route.ts) |
| `/store/buyback/requests/[id]` | GET, POST | [medusa-store/apps/backend/src/api/store/buyback/requests/[id]/route.ts](../medusa-store/apps/backend/src/api/store/buyback/requests/%5Bid%5D/route.ts) |
| `/store/buyback/requests` | GET, POST | [medusa-store/apps/backend/src/api/store/buyback/requests/route.ts](../medusa-store/apps/backend/src/api/store/buyback/requests/route.ts) |
| `/store/custom` | GET | [medusa-store/apps/backend/src/api/store/custom/route.ts](../medusa-store/apps/backend/src/api/store/custom/route.ts) |
| `/store/customer-service` | GET | [medusa-store/apps/backend/src/api/store/customer-service/route.ts](../medusa-store/apps/backend/src/api/store/customer-service/route.ts) |
| `/store/notifications` | GET, POST | [medusa-store/apps/backend/src/api/store/notifications/route.ts](../medusa-store/apps/backend/src/api/store/notifications/route.ts) |
| `/store/orders/[id]/after-sales` | GET, POST | [medusa-store/apps/backend/src/api/store/orders/[id]/after-sales/route.ts](../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/after-sales/route.ts) |
| `/store/orders/[id]/devices` | GET | [medusa-store/apps/backend/src/api/store/orders/[id]/devices/route.ts](../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/devices/route.ts) |
| `/store/orders/[id]/product-reviews` | GET | [medusa-store/apps/backend/src/api/store/orders/[id]/product-reviews/route.ts](../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/product-reviews/route.ts) |
| `/store/orders/[id]/timeline` | GET | [medusa-store/apps/backend/src/api/store/orders/[id]/timeline/route.ts](../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/timeline/route.ts) |
| `/store/payment-collections/[id]/payment-sessions` | POST | [medusa-store/apps/backend/src/api/store/payment-collections/[id]/payment-sessions/route.ts](../medusa-store/apps/backend/src/api/store/payment-collections/%5Bid%5D/payment-sessions/route.ts) |
| `/store/product-reviews/purchases` | GET | [medusa-store/apps/backend/src/api/store/product-reviews/purchases/route.ts](../medusa-store/apps/backend/src/api/store/product-reviews/purchases/route.ts) |
| `/store/product-reviews` | GET, POST | [medusa-store/apps/backend/src/api/store/product-reviews/route.ts](../medusa-store/apps/backend/src/api/store/product-reviews/route.ts) |
| `/store/seo/pages` | GET | [medusa-store/apps/backend/src/api/store/seo/pages/route.ts](../medusa-store/apps/backend/src/api/store/seo/pages/route.ts) |
| `/store/seo` | GET | [medusa-store/apps/backend/src/api/store/seo/route.ts](../medusa-store/apps/backend/src/api/store/seo/route.ts) |
| `/store/shipping-demo/dhl` | GET | [medusa-store/apps/backend/src/api/store/shipping-demo/dhl/route.ts](../medusa-store/apps/backend/src/api/store/shipping-demo/dhl/route.ts) |
| `/store/shipping-demo/orders/[id]` | GET | [medusa-store/apps/backend/src/api/store/shipping-demo/orders/[id]/route.ts](../medusa-store/apps/backend/src/api/store/shipping-demo/orders/%5Bid%5D/route.ts) |

## 4. 自定义模块、模型与迁移

12 个模块，32 个自定义模型实体；sku-numbering/order-numbering 主要在数据库迁移中实现，并非普通实体 CRUD。

| 模块 | models 内定义的实体 | 迁移文件数 |
| --- | --- | --- |
| [medusa-store/apps/backend/src/modules/after-sales/index.ts](../medusa-store/apps/backend/src/modules/after-sales/index.ts) | `after_sales_request` | 3 |
| [medusa-store/apps/backend/src/modules/buyback/index.ts](../medusa-store/apps/backend/src/modules/buyback/index.ts) | `buyback_request`, `buyback_photo`, `buyback_price`, `buyback_device_link` | 4 |
| [medusa-store/apps/backend/src/modules/customer-notifications/index.ts](../medusa-store/apps/backend/src/modules/customer-notifications/index.ts) | `customer_notice` | 1 |
| [medusa-store/apps/backend/src/modules/devices/index.ts](../medusa-store/apps/backend/src/modules/devices/index.ts) | `device`, `device_source`, `device_variant`, `device_supply`, `device_assignment`, `device_event` | 7 |
| [medusa-store/apps/backend/src/modules/mail/index.ts](../medusa-store/apps/backend/src/modules/mail/index.ts) | `mail_account`, `mail_message`, `mail_contact`, `mail_campaign`, `mail_delivery`, `mail_event`, `mail_template` | 4 |
| [medusa-store/apps/backend/src/modules/mollie/index.ts](../medusa-store/apps/backend/src/modules/mollie/index.ts) |  | 0 |
| [medusa-store/apps/backend/src/modules/mollie-settings/index.ts](../medusa-store/apps/backend/src/modules/mollie-settings/index.ts) | `mollie_config` | 1 |
| [medusa-store/apps/backend/src/modules/order-numbering/index.ts](../medusa-store/apps/backend/src/modules/order-numbering/index.ts) |  | 3 |
| [medusa-store/apps/backend/src/modules/product-review/index.ts](../medusa-store/apps/backend/src/modules/product-review/index.ts) | `product_review` | 3 |
| [medusa-store/apps/backend/src/modules/seo/index.ts](../medusa-store/apps/backend/src/modules/seo/index.ts) | `seo_setting`, `seo_page`, `seo_source`, `seo_crawler_visit`, `seo_traffic_visit`, `seo_provider`, `seo_monitor`, `seo_run`, `seo_evidence`, `seo_submission` | 1 |
| [medusa-store/apps/backend/src/modules/sku-numbering/index.ts](../medusa-store/apps/backend/src/modules/sku-numbering/index.ts) |  | 2 |
| [medusa-store/apps/backend/src/modules/webhook-notifications/index.ts](../medusa-store/apps/backend/src/modules/webhook-notifications/index.ts) | `webhook_endpoint` | 1 |

## 5. 自定义 Admin 页面

- [medusa-store/apps/backend/src/admin/routes/device-scan/inspection/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/inspection/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/intake/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/intake/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/lookup/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/lookup/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/orders/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/orders/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/prices/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/prices/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/recycled/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/recycled/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/requests/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/requests/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/returned/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/returned/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/sources/page.tsx](../medusa-store/apps/backend/src/admin/routes/device-scan/sources/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/mail/page.tsx](../medusa-store/apps/backend/src/admin/routes/mail/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/citation-monitor/page.tsx](../medusa-store/apps/backend/src/admin/routes/seo/citation-monitor/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/crawler-logs/page.tsx](../medusa-store/apps/backend/src/admin/routes/seo/crawler-logs/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/page.tsx](../medusa-store/apps/backend/src/admin/routes/seo/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/providers/page.tsx](../medusa-store/apps/backend/src/admin/routes/seo/providers/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/traffic-sources/page.tsx](../medusa-store/apps/backend/src/admin/routes/seo/traffic-sources/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/buy-policies/page.tsx](../medusa-store/apps/backend/src/admin/routes/settings/buy-policies/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/customer-service/page.tsx](../medusa-store/apps/backend/src/admin/routes/settings/customer-service/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/mailboxes/page.tsx](../medusa-store/apps/backend/src/admin/routes/settings/mailboxes/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/mollie/page.tsx](../medusa-store/apps/backend/src/admin/routes/settings/mollie/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/warehouse-access/page.tsx](../medusa-store/apps/backend/src/admin/routes/settings/warehouse-access/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/webhook-notifications/page.tsx](../medusa-store/apps/backend/src/admin/routes/settings/webhook-notifications/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/shipping-demo/page.tsx](../medusa-store/apps/backend/src/admin/routes/shipping-demo/page.tsx)

## 6. Admin widgets

- [medusa-store/apps/backend/src/admin/widgets/mail-topbar.tsx](../medusa-store/apps/backend/src/admin/widgets/mail-topbar.tsx)
- [medusa-store/apps/backend/src/admin/widgets/order-after-sales.tsx](../medusa-store/apps/backend/src/admin/widgets/order-after-sales.tsx)
- [medusa-store/apps/backend/src/admin/widgets/order-devices.tsx](../medusa-store/apps/backend/src/admin/widgets/order-devices.tsx)
- [medusa-store/apps/backend/src/admin/widgets/order-shipping.tsx](../medusa-store/apps/backend/src/admin/widgets/order-shipping.tsx)
- [medusa-store/apps/backend/src/admin/widgets/product-reviews.tsx](../medusa-store/apps/backend/src/admin/widgets/product-reviews.tsx)
- [medusa-store/apps/backend/src/admin/widgets/selectable-messages.tsx](../medusa-store/apps/backend/src/admin/widgets/selectable-messages.tsx)

## 7. 工作流（含 hooks 和 steps）

- [medusa-store/apps/backend/src/workflows/buyback-catalog.ts](../medusa-store/apps/backend/src/workflows/buyback-catalog.ts)
- [medusa-store/apps/backend/src/workflows/buyback.ts](../medusa-store/apps/backend/src/workflows/buyback.ts)
- [medusa-store/apps/backend/src/workflows/cancel-order-with-reason.ts](../medusa-store/apps/backend/src/workflows/cancel-order-with-reason.ts)
- [medusa-store/apps/backend/src/workflows/create-after-sales.ts](../medusa-store/apps/backend/src/workflows/create-after-sales.ts)
- [medusa-store/apps/backend/src/workflows/customer-notifications.ts](../medusa-store/apps/backend/src/workflows/customer-notifications.ts)
- [medusa-store/apps/backend/src/workflows/customer-service.ts](../medusa-store/apps/backend/src/workflows/customer-service.ts)
- [medusa-store/apps/backend/src/workflows/device-operations.ts](../medusa-store/apps/backend/src/workflows/device-operations.ts)
- [medusa-store/apps/backend/src/workflows/device-photos.ts](../medusa-store/apps/backend/src/workflows/device-photos.ts)
- [medusa-store/apps/backend/src/workflows/device-refund.ts](../medusa-store/apps/backend/src/workflows/device-refund.ts)
- [medusa-store/apps/backend/src/workflows/device-source.ts](../medusa-store/apps/backend/src/workflows/device-source.ts)
- [medusa-store/apps/backend/src/workflows/device-supplier-account.ts](../medusa-store/apps/backend/src/workflows/device-supplier-account.ts)
- [medusa-store/apps/backend/src/workflows/enroll-all-device-stock.ts](../medusa-store/apps/backend/src/workflows/enroll-all-device-stock.ts)
- [medusa-store/apps/backend/src/workflows/expire-buyback-photos.ts](../medusa-store/apps/backend/src/workflows/expire-buyback-photos.ts)
- [medusa-store/apps/backend/src/workflows/hooks/device-cart-stock.ts](../medusa-store/apps/backend/src/workflows/hooks/device-cart-stock.ts)
- [medusa-store/apps/backend/src/workflows/hooks/variant-sku-inventory.ts](../medusa-store/apps/backend/src/workflows/hooks/variant-sku-inventory.ts)
- [medusa-store/apps/backend/src/workflows/mail-command.ts](../medusa-store/apps/backend/src/workflows/mail-command.ts)
- [medusa-store/apps/backend/src/workflows/mail-webhook.ts](../medusa-store/apps/backend/src/workflows/mail-webhook.ts)
- [medusa-store/apps/backend/src/workflows/mobile-intake.ts](../medusa-store/apps/backend/src/workflows/mobile-intake.ts)
- [medusa-store/apps/backend/src/workflows/process-mail-queue.ts](../medusa-store/apps/backend/src/workflows/process-mail-queue.ts)
- [medusa-store/apps/backend/src/workflows/product-reviews.ts](../medusa-store/apps/backend/src/workflows/product-reviews.ts)
- [medusa-store/apps/backend/src/workflows/reconcile-device-stock.ts](../medusa-store/apps/backend/src/workflows/reconcile-device-stock.ts)
- [medusa-store/apps/backend/src/workflows/registration-email.ts](../medusa-store/apps/backend/src/workflows/registration-email.ts)
- [medusa-store/apps/backend/src/workflows/resume-mollie-session.ts](../medusa-store/apps/backend/src/workflows/resume-mollie-session.ts)
- [medusa-store/apps/backend/src/workflows/save-buy-policies.ts](../medusa-store/apps/backend/src/workflows/save-buy-policies.ts)
- [medusa-store/apps/backend/src/workflows/save-mollie-config.ts](../medusa-store/apps/backend/src/workflows/save-mollie-config.ts)
- [medusa-store/apps/backend/src/workflows/seo-command.ts](../medusa-store/apps/backend/src/workflows/seo-command.ts)
- [medusa-store/apps/backend/src/workflows/steps/mail-command.ts](../medusa-store/apps/backend/src/workflows/steps/mail-command.ts)
- [medusa-store/apps/backend/src/workflows/steps/process-mail-queue.ts](../medusa-store/apps/backend/src/workflows/steps/process-mail-queue.ts)
- [medusa-store/apps/backend/src/workflows/unsubscribe-mail.ts](../medusa-store/apps/backend/src/workflows/unsubscribe-mail.ts)
- [medusa-store/apps/backend/src/workflows/update-after-sales.ts](../medusa-store/apps/backend/src/workflows/update-after-sales.ts)
- [medusa-store/apps/backend/src/workflows/update-shipment-tracking.ts](../medusa-store/apps/backend/src/workflows/update-shipment-tracking.ts)
- [medusa-store/apps/backend/src/workflows/warehouse-access.ts](../medusa-store/apps/backend/src/workflows/warehouse-access.ts)
- [medusa-store/apps/backend/src/workflows/webhook-notifications.ts](../medusa-store/apps/backend/src/workflows/webhook-notifications.ts)

## 8. 定时任务

- [medusa-store/apps/backend/src/jobs/expire-buyback-photos.ts](../medusa-store/apps/backend/src/jobs/expire-buyback-photos.ts)
- [medusa-store/apps/backend/src/jobs/process-mail-queue.ts](../medusa-store/apps/backend/src/jobs/process-mail-queue.ts)
- [medusa-store/apps/backend/src/jobs/seo-monitor.ts](../medusa-store/apps/backend/src/jobs/seo-monitor.ts)

## 9. 事件订阅

- [medusa-store/apps/backend/src/subscribers/customer-notifications.ts](../medusa-store/apps/backend/src/subscribers/customer-notifications.ts)
- [medusa-store/apps/backend/src/subscribers/iphone-product-images.ts](../medusa-store/apps/backend/src/subscribers/iphone-product-images.ts)
- [medusa-store/apps/backend/src/subscribers/registration-email.ts](../medusa-store/apps/backend/src/subscribers/registration-email.ts)
- [medusa-store/apps/backend/src/subscribers/webhook-notifications.ts](../medusa-store/apps/backend/src/subscribers/webhook-notifications.ts)

## 10. 当前框架耦合的直接证据

以下仅统计显式导入 Next 的 98 个文件；动态导入、间接依赖和 Medusa SDK 依赖需要继续审查。

- [medusa-store/apps/storefront/src/app/[countryCode]/(checkout)/checkout/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28checkout%29/checkout/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(checkout)/not-found.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28checkout%29/not-found.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/about/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/about/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/addresses/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/addresses/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/orders/details/[id]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/orders/details/%5Bid%5D/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/orders/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/orders/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@dashboard/profile/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40dashboard/profile/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/account/@login/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/account/%40login/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/blog/[slug]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/blog/%5Bslug%5D/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/blog/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/blog/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/cart/not-found.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/cart/not-found.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/cart/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/cart/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/categories/[...category]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/categories/%5B...category%5D/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/collections/[handle]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/collections/%5Bhandle%5D/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/help/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/help/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/layout.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/layout.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/not-found.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/not-found.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/order/[id]/confirmed/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/order/%5Bid%5D/confirmed/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/products/[handle]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/products/%5Bhandle%5D/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/reviews/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/reviews/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/requests/[id]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/requests/%5Bid%5D/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/sell/requests/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/sell/requests/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/shipping-demo/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/shipping-demo/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/store/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/store/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/ui/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/ui/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(main)/verify-account/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28main%29/verify-account/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(payment)/payment-result/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28payment%29/payment-result/page.tsx)
- [medusa-store/apps/storefront/src/app/[countryCode]/(sell-flow)/sell/iphone/[...steps]/page.tsx](../medusa-store/apps/storefront/src/app/%5BcountryCode%5D/%28sell-flow%29/sell/iphone/%5B...steps%5D/page.tsx)
- [medusa-store/apps/storefront/src/app/api/mollie-return/route.ts](../medusa-store/apps/storefront/src/app/api/mollie-return/route.ts)
- [medusa-store/apps/storefront/src/app/api/payment-return/route.ts](../medusa-store/apps/storefront/src/app/api/payment-return/route.ts)
- [medusa-store/apps/storefront/src/app/layout.tsx](../medusa-store/apps/storefront/src/app/layout.tsx)
- [medusa-store/apps/storefront/src/app/not-found.tsx](../medusa-store/apps/storefront/src/app/not-found.tsx)
- [medusa-store/apps/storefront/src/lib/data/cart.ts](../medusa-store/apps/storefront/src/lib/data/cart.ts)
- [medusa-store/apps/storefront/src/lib/data/cookies.ts](../medusa-store/apps/storefront/src/lib/data/cookies.ts)
- [medusa-store/apps/storefront/src/lib/data/customer.ts](../medusa-store/apps/storefront/src/lib/data/customer.ts)
- [medusa-store/apps/storefront/src/lib/data/locale-actions.ts](../medusa-store/apps/storefront/src/lib/data/locale-actions.ts)
- [medusa-store/apps/storefront/src/lib/data/onboarding.ts](../medusa-store/apps/storefront/src/lib/data/onboarding.ts)
- [medusa-store/apps/storefront/src/lib/data/seo.ts](../medusa-store/apps/storefront/src/lib/data/seo.ts)
- [medusa-store/apps/storefront/src/middleware.ts](../medusa-store/apps/storefront/src/middleware.ts)
- [medusa-store/apps/storefront/src/modules/account/components/account-nav/index.tsx](../medusa-store/apps/storefront/src/modules/account/components/account-nav/index.tsx)
- [medusa-store/apps/storefront/src/modules/account/components/ui/index.tsx](../medusa-store/apps/storefront/src/modules/account/components/ui/index.tsx)
- [medusa-store/apps/storefront/src/modules/account/components/verify-account/index.tsx](../medusa-store/apps/storefront/src/modules/account/components/verify-account/index.tsx)
- [medusa-store/apps/storefront/src/modules/account/use-account-copy.ts](../medusa-store/apps/storefront/src/modules/account/use-account-copy.ts)
- [medusa-store/apps/storefront/src/modules/cart/components/cart-line/index.tsx](../medusa-store/apps/storefront/src/modules/cart/components/cart-line/index.tsx)
- [medusa-store/apps/storefront/src/modules/categories/templates/index.tsx](../medusa-store/apps/storefront/src/modules/categories/templates/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/addresses/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/addresses/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/billing_address/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/billing_address/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/checkout-progress/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/checkout-progress/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/country-select/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/country-select/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/discount-code/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/discount-code/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/payment/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/payment/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/payment-button/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/payment-button/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/payment-container/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/payment-container/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/payment-result/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/payment-result/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/payment-test/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/payment-test/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/review/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/review/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/shipping/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/shipping/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/components/shipping-address/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/components/shipping-address/index.tsx)
- [medusa-store/apps/storefront/src/modules/checkout/templates/checkout-summary/index.tsx](../medusa-store/apps/storefront/src/modules/checkout/templates/checkout-summary/index.tsx)
- [medusa-store/apps/storefront/src/modules/common/components/brand-logo/index.tsx](../medusa-store/apps/storefront/src/modules/common/components/brand-logo/index.tsx)
- [medusa-store/apps/storefront/src/modules/common/components/localized-client-link/index.tsx](../medusa-store/apps/storefront/src/modules/common/components/localized-client-link/index.tsx)
- [medusa-store/apps/storefront/src/modules/common/components/seo-data/index.tsx](../medusa-store/apps/storefront/src/modules/common/components/seo-data/index.tsx)
- [medusa-store/apps/storefront/src/modules/home/components/blog-section/index.tsx](../medusa-store/apps/storefront/src/modules/home/components/blog-section/index.tsx)
- [medusa-store/apps/storefront/src/modules/home/components/home-carousel/index.tsx](../medusa-store/apps/storefront/src/modules/home/components/home-carousel/index.tsx)
- [medusa-store/apps/storefront/src/modules/home/components/series-gallery/index.tsx](../medusa-store/apps/storefront/src/modules/home/components/series-gallery/index.tsx)
- [medusa-store/apps/storefront/src/modules/layout/components/cart-dropdown/index.tsx](../medusa-store/apps/storefront/src/modules/layout/components/cart-dropdown/index.tsx)
- [medusa-store/apps/storefront/src/modules/layout/components/country-select/index.tsx](../medusa-store/apps/storefront/src/modules/layout/components/country-select/index.tsx)
- [medusa-store/apps/storefront/src/modules/layout/components/language-select/index.tsx](../medusa-store/apps/storefront/src/modules/layout/components/language-select/index.tsx)
- [medusa-store/apps/storefront/src/modules/layout/components/region-language/index.tsx](../medusa-store/apps/storefront/src/modules/layout/components/region-language/index.tsx)
- [medusa-store/apps/storefront/src/modules/layout/components/search/hit.tsx](../medusa-store/apps/storefront/src/modules/layout/components/search/hit.tsx)
- [medusa-store/apps/storefront/src/modules/layout/components/store-header/index.tsx](../medusa-store/apps/storefront/src/modules/layout/components/store-header/index.tsx)
- [medusa-store/apps/storefront/src/modules/notifications/notification-bell.tsx](../medusa-store/apps/storefront/src/modules/notifications/notification-bell.tsx)
- [medusa-store/apps/storefront/src/modules/order/templates/order-completed-template.tsx](../medusa-store/apps/storefront/src/modules/order/templates/order-completed-template.tsx)
- [medusa-store/apps/storefront/src/modules/products/components/grade-guide/index.tsx](../medusa-store/apps/storefront/src/modules/products/components/grade-guide/index.tsx)
- [medusa-store/apps/storefront/src/modules/products/components/image-gallery/index.tsx](../medusa-store/apps/storefront/src/modules/products/components/image-gallery/index.tsx)
- [medusa-store/apps/storefront/src/modules/products/components/product-actions/index.tsx](../medusa-store/apps/storefront/src/modules/products/components/product-actions/index.tsx)
- [medusa-store/apps/storefront/src/modules/products/components/product-onboarding-cta/index.tsx](../medusa-store/apps/storefront/src/modules/products/components/product-onboarding-cta/index.tsx)
- [medusa-store/apps/storefront/src/modules/products/components/related-products/index.tsx](../medusa-store/apps/storefront/src/modules/products/components/related-products/index.tsx)
- [medusa-store/apps/storefront/src/modules/products/components/thumbnail/index.tsx](../medusa-store/apps/storefront/src/modules/products/components/thumbnail/index.tsx)
- [medusa-store/apps/storefront/src/modules/products/templates/index.tsx](../medusa-store/apps/storefront/src/modules/products/templates/index.tsx)
- [medusa-store/apps/storefront/src/modules/sell/components/buyback-requests.tsx](../medusa-store/apps/storefront/src/modules/sell/components/buyback-requests.tsx)
- [medusa-store/apps/storefront/src/modules/sell/components/buyback-submission.tsx](../medusa-store/apps/storefront/src/modules/sell/components/buyback-submission.tsx)
- [medusa-store/apps/storefront/src/modules/sell/components/sell-flow.tsx](../medusa-store/apps/storefront/src/modules/sell/components/sell-flow.tsx)
- [medusa-store/apps/storefront/src/modules/sell/components/sell-landing.tsx](../medusa-store/apps/storefront/src/modules/sell/components/sell-landing.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/pagination/index.tsx](../medusa-store/apps/storefront/src/modules/store/components/pagination/index.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/refinement-list/index.tsx](../medusa-store/apps/storefront/src/modules/store/components/refinement-list/index.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-hits/index.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-hits/index.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-hits/price.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-hits/price.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-refinements/current-refinements.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-refinements/current-refinements.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-refinements/index.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-refinements/index.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-refinements/mobile-filters.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-refinements/mobile-filters.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-refinements/price-range.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-refinements/price-range.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-refinements/refinement-group.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-refinements/refinement-group.tsx)
- [medusa-store/apps/storefront/src/modules/store/components/store-search-box/index.tsx](../medusa-store/apps/storefront/src/modules/store/components/store-search-box/index.tsx)
- [medusa-store/apps/storefront/src/modules/store/templates/index.tsx](../medusa-store/apps/storefront/src/modules/store/templates/index.tsx)

## 11. 参考项目的代表性文件

路径根目录：`C:/Users/Administrator/Desktop/other/iphone0902`。这里只读参考，不表示已复制或审计全部实现。

| 目的 | 文件 |
| --- | --- |
| 启动、依赖构造、任务和优雅停止 | `backend/cmd/server/main.go` |
| Gin 路由分组 | `backend/internal/routes/routes.go` |
| 数据库、连接池和 Redis | `backend/internal/db/db.go` |
| JWT 与退出机制 | `backend/internal/auth/jwt.go` |
| 菜单权限与默认拒绝 | `backend/internal/middleware/permission.go` |
| 用户/角色/权限模型 | `backend/internal/models/rbac.go` |
| React 后台外壳 | `backend/web/src/App.tsx` |
| API 客户端与统一反馈 | `backend/web/src/api/client.ts` |
| 文件选择器 | `backend/web/src/components/FilePicker.tsx` |
| 表格列宽 | `backend/web/src/components/ResizableTable.tsx` |
| 构建发布：含服务器 npm build，需要调整 | `backend/internal/handlers/build.go` |
| 现有 Astro 配置：路由和样式不能照搬 | `frontend/astro.config.mjs` |
