# 源码证据追踪表

由 `retail/scripts/inventory-business.py` 生成。只读扫描源码，不加载环境变量或连接数据库。

本表记录静态证据及任务去向，**不代表运行验证、权限审计或迁移完成**。动态调用、间接封装、插件和 Medusa 内置 API 需另行核对；未找到调用方不表示功能可删除。

## 自定义接口

全局中间件与路由内校验共同决定权限；所有接口的数据范围仍待 T004 审计。任务为领域承接范围，具体 HTTP/动作契约由 T007 定义。

| 旧路由 | 方法与证据 | 直接工作流依赖 | 承接任务 |
| --- | --- | --- | --- |
| `/admin/buy-policies` | [medusa-store/apps/backend/src/api/admin/buy-policies/route.ts:5](../../medusa-store/apps/backend/src/api/admin/buy-policies/route.ts#L5) GET、[medusa-store/apps/backend/src/api/admin/buy-policies/route.ts:11](../../medusa-store/apps/backend/src/api/admin/buy-policies/route.ts#L11) POST | `../../../workflows/save-buy-policies` | T058, T059 |
| `/admin/buyback/[id]/photos/[photo_id]` | [medusa-store/apps/backend/src/api/admin/buyback/[id]/photos/[photo_id]/route.ts:5](../../medusa-store/apps/backend/src/api/admin/buyback/%5Bid%5D/photos/%5Bphoto_id%5D/route.ts#L5) GET | 需追踪委托/模块调用 | T050, T051, T052, T053 |
| `/admin/buyback/[id]` | [medusa-store/apps/backend/src/api/admin/buyback/[id]/route.ts:8](../../medusa-store/apps/backend/src/api/admin/buyback/%5Bid%5D/route.ts#L8) GET、[medusa-store/apps/backend/src/api/admin/buyback/[id]/route.ts:13](../../medusa-store/apps/backend/src/api/admin/buyback/%5Bid%5D/route.ts#L13) POST | `../../../../workflows/buyback` | T050, T051, T052, T053 |
| `/admin/buyback` | [medusa-store/apps/backend/src/api/admin/buyback/route.ts:6](../../medusa-store/apps/backend/src/api/admin/buyback/route.ts#L6) GET | 需追踪委托/模块调用 | T050, T051, T052, T053 |
| `/admin/buyback-catalog` | [medusa-store/apps/backend/src/api/admin/buyback-catalog/route.ts:7](../../medusa-store/apps/backend/src/api/admin/buyback-catalog/route.ts#L7) GET、[medusa-store/apps/backend/src/api/admin/buyback-catalog/route.ts:11](../../medusa-store/apps/backend/src/api/admin/buyback-catalog/route.ts#L11) POST | `../../../workflows/buyback-catalog` | T050, T051, T052, T053 |
| `/admin/buyback-prices` | [medusa-store/apps/backend/src/api/admin/buyback-prices/route.ts:8](../../medusa-store/apps/backend/src/api/admin/buyback-prices/route.ts#L8) GET、[medusa-store/apps/backend/src/api/admin/buyback-prices/route.ts:15](../../medusa-store/apps/backend/src/api/admin/buyback-prices/route.ts#L15) POST | `../../../workflows/buyback` | T050, T051, T052, T053 |
| `/admin/custom` | [medusa-store/apps/backend/src/api/admin/custom/route.ts:3](../../medusa-store/apps/backend/src/api/admin/custom/route.ts#L3) GET | 需追踪委托/模块调用 | T002 |
| `/admin/customer-service` | [medusa-store/apps/backend/src/api/admin/customer-service/route.ts:6](../../medusa-store/apps/backend/src/api/admin/customer-service/route.ts#L6) GET、[medusa-store/apps/backend/src/api/admin/customer-service/route.ts:11](../../medusa-store/apps/backend/src/api/admin/customer-service/route.ts#L11) POST | `../../../workflows/customer-service` | T058, T059 |
| `/admin/devices/import-template` | [medusa-store/apps/backend/src/api/admin/devices/import-template/route.ts:8](../../medusa-store/apps/backend/src/api/admin/devices/import-template/route.ts#L8) GET | 需追踪委托/模块调用 | T040, T041, T042, T043, T048 |
| `/admin/devices/mobile-intake` | [medusa-store/apps/backend/src/api/admin/devices/mobile-intake/route.ts:5](../../medusa-store/apps/backend/src/api/admin/devices/mobile-intake/route.ts#L5) POST | `../../../../workflows/mobile-intake` | T040, T041, T042, T043, T048 |
| `/admin/devices/photos` | [medusa-store/apps/backend/src/api/admin/devices/photos/route.ts:6](../../medusa-store/apps/backend/src/api/admin/devices/photos/route.ts#L6) POST | `../../../../workflows/device-photos` | T040, T041, T042, T043, T048 |
| `/admin/devices/refunds` | [medusa-store/apps/backend/src/api/admin/devices/refunds/route.ts:8](../../medusa-store/apps/backend/src/api/admin/devices/refunds/route.ts#L8) POST | `../../../../workflows/device-refund` | T047 |
| `/admin/devices` | [medusa-store/apps/backend/src/api/admin/devices/route.ts:26](../../medusa-store/apps/backend/src/api/admin/devices/route.ts#L26) GET、[medusa-store/apps/backend/src/api/admin/devices/route.ts:390](../../medusa-store/apps/backend/src/api/admin/devices/route.ts#L390) POST | `../../../workflows/device-operations`、`../../../workflows/device-source` | T040, T041, T042, T043, T048 |
| `/admin/devices/supplier-accounts` | [medusa-store/apps/backend/src/api/admin/devices/supplier-accounts/route.ts:23](../../medusa-store/apps/backend/src/api/admin/devices/supplier-accounts/route.ts#L23) GET、[medusa-store/apps/backend/src/api/admin/devices/supplier-accounts/route.ts:44](../../medusa-store/apps/backend/src/api/admin/devices/supplier-accounts/route.ts#L44) POST | `../../../../workflows/device-supplier-account` | T014, T015, T042 |
| `/admin/devices/variants` | [medusa-store/apps/backend/src/api/admin/devices/variants/route.ts:8](../../medusa-store/apps/backend/src/api/admin/devices/variants/route.ts#L8) GET | 需追踪委托/模块调用 | T040, T041, T042, T043, T048 |
| `/admin/inventory-items` | [medusa-store/apps/backend/src/api/admin/inventory-items/route.ts:5](../../medusa-store/apps/backend/src/api/admin/inventory-items/route.ts#L5) GET | 需追踪委托/模块调用 | T031, T042 |
| `/admin/mail/commands` | [medusa-store/apps/backend/src/api/admin/mail/commands/route.ts:8](../../medusa-store/apps/backend/src/api/admin/mail/commands/route.ts#L8) POST | `../../../../workflows/mail-command` | T055, T056 |
| `/admin/mail/messages/[id]/attachments/[attachment_id]` | [medusa-store/apps/backend/src/api/admin/mail/messages/[id]/attachments/[attachment_id]/route.ts:11](../../medusa-store/apps/backend/src/api/admin/mail/messages/%5Bid%5D/attachments/%5Battachment_id%5D/route.ts#L11) GET | 需追踪委托/模块调用 | T055, T056 |
| `/admin/mail/messages/[id]` | [medusa-store/apps/backend/src/api/admin/mail/messages/[id]/route.ts:8](../../medusa-store/apps/backend/src/api/admin/mail/messages/%5Bid%5D/route.ts#L8) GET | 需追踪委托/模块调用 | T055, T056 |
| `/admin/mail` | [medusa-store/apps/backend/src/api/admin/mail/route.ts:11](../../medusa-store/apps/backend/src/api/admin/mail/route.ts#L11) GET | 需追踪委托/模块调用 | T055, T056 |
| `/admin/mollie` | [medusa-store/apps/backend/src/api/admin/mollie/route.ts:10](../../medusa-store/apps/backend/src/api/admin/mollie/route.ts#L10) GET、[medusa-store/apps/backend/src/api/admin/mollie/route.ts:21](../../medusa-store/apps/backend/src/api/admin/mollie/route.ts#L21) POST | `../../../workflows/save-mollie-config` | T035, T036 |
| `/admin/orders/[id]/after-sales` | [medusa-store/apps/backend/src/api/admin/orders/[id]/after-sales/route.ts:8](../../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/after-sales/route.ts#L8) GET、[medusa-store/apps/backend/src/api/admin/orders/[id]/after-sales/route.ts:14](../../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/after-sales/route.ts#L14) POST | `../../../../../workflows/update-after-sales` | T045, T046, T047, T049 |
| `/admin/orders/[id]/cancel` | [medusa-store/apps/backend/src/api/admin/orders/[id]/cancel/route.ts:6](../../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/cancel/route.ts#L6) POST | `../../../../../workflows/cancel-order-with-reason` | T034, T037, T038, T039, T044 |
| `/admin/orders/[id]/fulfillments/[fulfillment_id]/cancel` | [medusa-store/apps/backend/src/api/admin/orders/[id]/fulfillments/[fulfillment_id]/cancel/route.ts:5](../../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/fulfillments/%5Bfulfillment_id%5D/cancel/route.ts#L5) POST | `../../../../../../../workflows/device-operations` | T034, T037, T038, T039, T044 |
| `/admin/orders/[id]/fulfillments/[fulfillment_id]/shipments` | [medusa-store/apps/backend/src/api/admin/orders/[id]/fulfillments/[fulfillment_id]/shipments/route.ts:9](../../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/fulfillments/%5Bfulfillment_id%5D/shipments/route.ts#L9) POST | `@medusajs/medusa/core-flows`、`../../../../../../../workflows/device-operations` | T034, T037, T038, T039, T044 |
| `/admin/orders/[id]/fulfillments/[fulfillment_id]/tracking` | [medusa-store/apps/backend/src/api/admin/orders/[id]/fulfillments/[fulfillment_id]/tracking/route.ts:5](../../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/fulfillments/%5Bfulfillment_id%5D/tracking/route.ts#L5) POST | `../../../../../../../workflows/update-shipment-tracking` | T034, T037, T038, T039, T044 |
| `/admin/orders/[id]` | [medusa-store/apps/backend/src/api/admin/orders/[id]/route.ts:5](../../medusa-store/apps/backend/src/api/admin/orders/%5Bid%5D/route.ts#L5) GET | 需追踪委托/模块调用 | T034, T037, T038, T039, T044 |
| `/admin/orders` | [medusa-store/apps/backend/src/api/admin/orders/route.ts:5](../../medusa-store/apps/backend/src/api/admin/orders/route.ts#L5) GET | 需追踪委托/模块调用 | T034, T037, T038, T039, T044 |
| `/admin/product-reviews/[id]` | [medusa-store/apps/backend/src/api/admin/product-reviews/[id]/route.ts:5](../../medusa-store/apps/backend/src/api/admin/product-reviews/%5Bid%5D/route.ts#L5) POST | `../../../../workflows/product-reviews` | T054 |
| `/admin/product-reviews` | [medusa-store/apps/backend/src/api/admin/product-reviews/route.ts:6](../../medusa-store/apps/backend/src/api/admin/product-reviews/route.ts#L6) GET | 需追踪委托/模块调用 | T054 |
| `/admin/seo/[resource]` | [medusa-store/apps/backend/src/api/admin/seo/[resource]/route.ts:7](../../medusa-store/apps/backend/src/api/admin/seo/%5Bresource%5D/route.ts#L7) GET | 需追踪委托/模块调用 | T058, T059 |
| `/admin/seo/commands` | [medusa-store/apps/backend/src/api/admin/seo/commands/route.ts:4](../../medusa-store/apps/backend/src/api/admin/seo/commands/route.ts#L4) POST | `../../../../workflows/seo-command` | T058, T059 |
| `/admin/shipping-demo/dhl` | [medusa-store/apps/backend/src/api/admin/shipping-demo/dhl/route.ts:4](../../medusa-store/apps/backend/src/api/admin/shipping-demo/dhl/route.ts#L4) GET | 需追踪委托/模块调用 | T059 |
| `/admin/stock-locations/[id]` | [medusa-store/apps/backend/src/api/admin/stock-locations/[id]/route.ts:5](../../medusa-store/apps/backend/src/api/admin/stock-locations/%5Bid%5D/route.ts#L5) GET | 需追踪委托/模块调用 | T031, T042 |
| `/admin/stock-locations` | [medusa-store/apps/backend/src/api/admin/stock-locations/route.ts:5](../../medusa-store/apps/backend/src/api/admin/stock-locations/route.ts#L5) GET | 需追踪委托/模块调用 | T031, T042 |
| `/admin/warehouse-access/me` | [medusa-store/apps/backend/src/api/admin/warehouse-access/me/route.ts:5](../../medusa-store/apps/backend/src/api/admin/warehouse-access/me/route.ts#L5) GET | 需追踪委托/模块调用 | T015, T042 |
| `/admin/warehouse-access` | [medusa-store/apps/backend/src/api/admin/warehouse-access/route.ts:9](../../medusa-store/apps/backend/src/api/admin/warehouse-access/route.ts#L9) GET、[medusa-store/apps/backend/src/api/admin/warehouse-access/route.ts:19](../../medusa-store/apps/backend/src/api/admin/warehouse-access/route.ts#L19) POST | `../../../workflows/warehouse-access` | T015, T042 |
| `/admin/webhook-notifications/[id]` | [medusa-store/apps/backend/src/api/admin/webhook-notifications/[id]/route.ts:3](../../medusa-store/apps/backend/src/api/admin/webhook-notifications/%5Bid%5D/route.ts#L3) POST、[medusa-store/apps/backend/src/api/admin/webhook-notifications/[id]/route.ts:7](../../medusa-store/apps/backend/src/api/admin/webhook-notifications/%5Bid%5D/route.ts#L7) DELETE | `../../../../workflows/webhook-notifications` | T057 |
| `/admin/webhook-notifications/[id]/test` | [medusa-store/apps/backend/src/api/admin/webhook-notifications/[id]/test/route.ts:3](../../medusa-store/apps/backend/src/api/admin/webhook-notifications/%5Bid%5D/test/route.ts#L3) POST | `../../../../../workflows/webhook-notifications` | T057 |
| `/admin/webhook-notifications` | [medusa-store/apps/backend/src/api/admin/webhook-notifications/route.ts:6](../../medusa-store/apps/backend/src/api/admin/webhook-notifications/route.ts#L6) GET、[medusa-store/apps/backend/src/api/admin/webhook-notifications/route.ts:11](../../medusa-store/apps/backend/src/api/admin/webhook-notifications/route.ts#L11) POST | `../../../workflows/webhook-notifications` | T057 |
| `/mail/unsubscribe/[token]` | [medusa-store/apps/backend/src/api/mail/unsubscribe/[token]/route.ts:4](../../medusa-store/apps/backend/src/api/mail/unsubscribe/%5Btoken%5D/route.ts#L4) GET、[medusa-store/apps/backend/src/api/mail/unsubscribe/[token]/route.ts:18](../../medusa-store/apps/backend/src/api/mail/unsubscribe/%5Btoken%5D/route.ts#L18) POST | `../../../../workflows/unsubscribe-mail` | T055, T056 |
| `/mail/webhooks/[account_id]` | [medusa-store/apps/backend/src/api/mail/webhooks/[account_id]/route.ts:8](../../medusa-store/apps/backend/src/api/mail/webhooks/%5Baccount_id%5D/route.ts#L8) POST | `../../../../workflows/mail-webhook` | T055, T056 |
| `/mobile-intake/devices/[id]` | [medusa-store/apps/backend/src/api/mobile-intake/devices/[id]/route.ts:4](../../medusa-store/apps/backend/src/api/mobile-intake/devices/%5Bid%5D/route.ts#L4) DELETE | `../../../../workflows/mobile-intake` | T040, T041, T042, T043, T048 |
| `/mobile-intake/photos` | [medusa-store/apps/backend/src/api/mobile-intake/photos/route.ts:6](../../medusa-store/apps/backend/src/api/mobile-intake/photos/route.ts#L6) POST | `../../../workflows/device-photos` | T040, T041, T042, T043, T048 |
| `/mobile-intake` | [medusa-store/apps/backend/src/api/mobile-intake/route.ts:6](../../medusa-store/apps/backend/src/api/mobile-intake/route.ts#L6) GET、[medusa-store/apps/backend/src/api/mobile-intake/route.ts:12](../../medusa-store/apps/backend/src/api/mobile-intake/route.ts#L12) POST | `../../workflows/mobile-intake` | T040, T041, T042, T043, T048 |
| `/mobile-intake/variants` | [medusa-store/apps/backend/src/api/mobile-intake/variants/route.ts:7](../../medusa-store/apps/backend/src/api/mobile-intake/variants/route.ts#L7) GET | 需追踪委托/模块调用 | T040, T041, T042, T043, T048 |
| `/store/buy-policies` | [medusa-store/apps/backend/src/api/store/buy-policies/route.ts:4](../../medusa-store/apps/backend/src/api/store/buy-policies/route.ts#L4) GET | 需追踪委托/模块调用 | T058, T059 |
| `/store/buyback/estimate` | [medusa-store/apps/backend/src/api/store/buyback/estimate/route.ts:8](../../medusa-store/apps/backend/src/api/store/buyback/estimate/route.ts#L8) POST | 需追踪委托/模块调用 | T050, T051, T052, T053 |
| `/store/buyback/prices` | [medusa-store/apps/backend/src/api/store/buyback/prices/route.ts:5](../../medusa-store/apps/backend/src/api/store/buyback/prices/route.ts#L5) GET | 需追踪委托/模块调用 | T050, T051, T052, T053 |
| `/store/buyback/requests/[id]/photos/[photo_id]` | [medusa-store/apps/backend/src/api/store/buyback/requests/[id]/photos/[photo_id]/route.ts:5](../../medusa-store/apps/backend/src/api/store/buyback/requests/%5Bid%5D/photos/%5Bphoto_id%5D/route.ts#L5) GET | 需追踪委托/模块调用 | T050, T051, T052, T053 |
| `/store/buyback/requests/[id]/photos` | [medusa-store/apps/backend/src/api/store/buyback/requests/[id]/photos/route.ts:5](../../medusa-store/apps/backend/src/api/store/buyback/requests/%5Bid%5D/photos/route.ts#L5) POST | `../../../../../../workflows/buyback` | T050, T051, T052, T053 |
| `/store/buyback/requests/[id]` | [medusa-store/apps/backend/src/api/store/buyback/requests/[id]/route.ts:6](../../medusa-store/apps/backend/src/api/store/buyback/requests/%5Bid%5D/route.ts#L6) GET、[medusa-store/apps/backend/src/api/store/buyback/requests/[id]/route.ts:10](../../medusa-store/apps/backend/src/api/store/buyback/requests/%5Bid%5D/route.ts#L10) POST | `../../../../../workflows/buyback` | T050, T051, T052, T053 |
| `/store/buyback/requests` | [medusa-store/apps/backend/src/api/store/buyback/requests/route.ts:9](../../medusa-store/apps/backend/src/api/store/buyback/requests/route.ts#L9) GET、[medusa-store/apps/backend/src/api/store/buyback/requests/route.ts:16](../../medusa-store/apps/backend/src/api/store/buyback/requests/route.ts#L16) POST | `../../../../workflows/buyback` | T050, T051, T052, T053 |
| `/store/custom` | [medusa-store/apps/backend/src/api/store/custom/route.ts:3](../../medusa-store/apps/backend/src/api/store/custom/route.ts#L3) GET | 需追踪委托/模块调用 | T002 |
| `/store/customer-service` | [medusa-store/apps/backend/src/api/store/customer-service/route.ts:4](../../medusa-store/apps/backend/src/api/store/customer-service/route.ts#L4) GET | 需追踪委托/模块调用 | T058, T059 |
| `/store/notifications` | [medusa-store/apps/backend/src/api/store/notifications/route.ts:7](../../medusa-store/apps/backend/src/api/store/notifications/route.ts#L7) GET、[medusa-store/apps/backend/src/api/store/notifications/route.ts:17](../../medusa-store/apps/backend/src/api/store/notifications/route.ts#L17) POST | `../../../workflows/customer-notifications` | T057 |
| `/store/orders/[id]/after-sales` | [medusa-store/apps/backend/src/api/store/orders/[id]/after-sales/route.ts:9](../../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/after-sales/route.ts#L9) GET、[medusa-store/apps/backend/src/api/store/orders/[id]/after-sales/route.ts:20](../../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/after-sales/route.ts#L20) POST | `../../../../../workflows/create-after-sales` | T045, T046, T047, T049 |
| `/store/orders/[id]/devices` | [medusa-store/apps/backend/src/api/store/orders/[id]/devices/route.ts:6](../../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/devices/route.ts#L6) GET | 需追踪委托/模块调用 | T040, T041, T042, T043, T048 |
| `/store/orders/[id]/product-reviews` | [medusa-store/apps/backend/src/api/store/orders/[id]/product-reviews/route.ts:6](../../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/product-reviews/route.ts#L6) GET | 需追踪委托/模块调用 | T054 |
| `/store/orders/[id]/timeline` | [medusa-store/apps/backend/src/api/store/orders/[id]/timeline/route.ts:7](../../medusa-store/apps/backend/src/api/store/orders/%5Bid%5D/timeline/route.ts#L7) GET | 需追踪委托/模块调用 | T034, T037, T038, T039, T044 |
| `/store/payment-collections/[id]/payment-sessions` | [medusa-store/apps/backend/src/api/store/payment-collections/[id]/payment-sessions/route.ts:6](../../medusa-store/apps/backend/src/api/store/payment-collections/%5Bid%5D/payment-sessions/route.ts#L6) POST | `@medusajs/medusa/core-flows`、`../../../../../workflows/resume-mollie-session` | T002 |
| `/store/product-reviews/purchases` | [medusa-store/apps/backend/src/api/store/product-reviews/purchases/route.ts:14](../../medusa-store/apps/backend/src/api/store/product-reviews/purchases/route.ts#L14) GET | 需追踪委托/模块调用 | T054 |
| `/store/product-reviews` | [medusa-store/apps/backend/src/api/store/product-reviews/route.ts:8](../../medusa-store/apps/backend/src/api/store/product-reviews/route.ts#L8) GET、[medusa-store/apps/backend/src/api/store/product-reviews/route.ts:25](../../medusa-store/apps/backend/src/api/store/product-reviews/route.ts#L25) POST | `../../../workflows/product-reviews` | T054 |
| `/store/seo/pages` | [medusa-store/apps/backend/src/api/store/seo/pages/route.ts:3](../../medusa-store/apps/backend/src/api/store/seo/pages/route.ts#L3) GET | 需追踪委托/模块调用 | T058, T059 |
| `/store/seo` | [medusa-store/apps/backend/src/api/store/seo/route.ts:4](../../medusa-store/apps/backend/src/api/store/seo/route.ts#L4) GET | 需追踪委托/模块调用 | T058, T059 |
| `/store/shipping-demo/dhl` | [medusa-store/apps/backend/src/api/store/shipping-demo/dhl/route.ts:4](../../medusa-store/apps/backend/src/api/store/shipping-demo/dhl/route.ts#L4) GET | 需追踪委托/模块调用 | T059 |
| `/store/shipping-demo/orders/[id]` | [medusa-store/apps/backend/src/api/store/shipping-demo/orders/[id]/route.ts:6](../../medusa-store/apps/backend/src/api/store/shipping-demo/orders/%5Bid%5D/route.ts#L6) GET | 需追踪委托/模块调用 | T059 |

## 前台原生 SDK 调用候选

包括可能未使用的导出函数；调用是否可达及参数、错误分支需人工追踪。直接 `/store/*` 请求另见迁移盘点，不能只覆盖 SDK 方法。

| SDK 表达式 | 源码 |
| --- | --- |
| `sdk.store.customer.retrieve` | [medusa-store/apps/storefront/src/app/api/mollie-return/route.ts:61](../../medusa-store/apps/storefront/src/app/api/mollie-return/route.ts#L61) |
| `sdk.store.cart.complete` | [medusa-store/apps/storefront/src/app/api/mollie-return/route.ts:83](../../medusa-store/apps/storefront/src/app/api/mollie-return/route.ts#L83) |
| `sdk.store.region.list` | [medusa-store/apps/storefront/src/app/sitemap.xml/route.ts:9](../../medusa-store/apps/storefront/src/app/sitemap.xml/route.ts#L9) |
| `sdk.store.product.list` | [medusa-store/apps/storefront/src/app/sitemap.xml/route.ts:15](../../medusa-store/apps/storefront/src/app/sitemap.xml/route.ts#L15) |
| `sdk.store.collection.list` | [medusa-store/apps/storefront/src/app/sitemap.xml/route.ts:21](../../medusa-store/apps/storefront/src/app/sitemap.xml/route.ts#L21) |
| `sdk.store.cart.create` | [medusa-store/apps/storefront/src/lib/data/cart.ts:83](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L83) |
| `sdk.store.cart.update` | [medusa-store/apps/storefront/src/lib/data/cart.ts:97](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L97) |
| `sdk.store.cart.update` | [medusa-store/apps/storefront/src/lib/data/cart.ts:116](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L116) |
| `sdk.store.cart.createLineItem` | [medusa-store/apps/storefront/src/lib/data/cart.ts:155](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L155) |
| `sdk.store.cart.updateLineItem` | [medusa-store/apps/storefront/src/lib/data/cart.ts:211](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L211) |
| `sdk.store.cart.deleteLineItem` | [medusa-store/apps/storefront/src/lib/data/cart.ts:238](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L238) |
| `sdk.store.cart.addShippingMethod` | [medusa-store/apps/storefront/src/lib/data/cart.ts:261](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L261) |
| `sdk.store.payment.initiatePaymentSession` | [medusa-store/apps/storefront/src/lib/data/cart.ts:294](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L294) |
| `sdk.store.cart.update` | [medusa-store/apps/storefront/src/lib/data/cart.ts:316](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L316) |
| `sdk.store.cart.complete` | [medusa-store/apps/storefront/src/lib/data/cart.ts:453](../../medusa-store/apps/storefront/src/lib/data/cart.ts#L453) |
| `sdk.auth.verification.request` | [medusa-store/apps/storefront/src/lib/data/customer.ts:38](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L38) |
| `sdk.store.customer.update` | [medusa-store/apps/storefront/src/lib/data/customer.ts:89](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L89) |
| `sdk.auth.register` | [medusa-store/apps/storefront/src/lib/data/customer.ts:113](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L113) |
| `sdk.auth.login` | [medusa-store/apps/storefront/src/lib/data/customer.ts:160](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L160) |
| `sdk.store.customer.retrieve` | [medusa-store/apps/storefront/src/lib/data/customer.ts:204](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L204) |
| `sdk.store.customer.create` | [medusa-store/apps/storefront/src/lib/data/customer.ts:214](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L214) |
| `sdk.auth.login` | [medusa-store/apps/storefront/src/lib/data/customer.ts:225](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L225) |
| `sdk.auth.verification.confirm` | [medusa-store/apps/storefront/src/lib/data/customer.ts:261](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L261) |
| `sdk.auth.logout` | [medusa-store/apps/storefront/src/lib/data/customer.ts:269](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L269) |
| `sdk.store.cart.transferCart` | [medusa-store/apps/storefront/src/lib/data/customer.ts:295](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L295) |
| `sdk.store.customer.createAddress` | [medusa-store/apps/storefront/src/lib/data/customer.ts:327](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L327) |
| `sdk.store.customer.deleteAddress` | [medusa-store/apps/storefront/src/lib/data/customer.ts:346](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L346) |
| `sdk.store.customer.updateAddress` | [medusa-store/apps/storefront/src/lib/data/customer.ts:391](../../medusa-store/apps/storefront/src/lib/data/customer.ts#L391) |
| `sdk.store.cart.update` | [medusa-store/apps/storefront/src/lib/data/locale-actions.ts:52](../../medusa-store/apps/storefront/src/lib/data/locale-actions.ts#L52) |
| `sdk.store.cart.retrieve` | [medusa-store/apps/storefront/src/lib/data/mollie.ts:17](../../medusa-store/apps/storefront/src/lib/data/mollie.ts#L17) |
| `sdk.store.order.requestTransfer` | [medusa-store/apps/storefront/src/lib/data/orders.ts:90](../../medusa-store/apps/storefront/src/lib/data/orders.ts#L90) |
| `sdk.store.order.acceptTransfer` | [medusa-store/apps/storefront/src/lib/data/orders.ts:106](../../medusa-store/apps/storefront/src/lib/data/orders.ts#L106) |
| `sdk.store.order.declineTransfer` | [medusa-store/apps/storefront/src/lib/data/orders.ts:115](../../medusa-store/apps/storefront/src/lib/data/orders.ts#L115) |
| `sdk.store.product.list` | [medusa-store/apps/storefront/src/lib/data/sell-models.ts:12](../../medusa-store/apps/storefront/src/lib/data/sell-models.ts#L12) |
| `sdk.store.category.list` | [medusa-store/apps/storefront/src/lib/data/store-search.ts:77](../../medusa-store/apps/storefront/src/lib/data/store-search.ts#L77) |
| `sdk.store.product.list` | [medusa-store/apps/storefront/src/lib/search-client.ts:77](../../medusa-store/apps/storefront/src/lib/search-client.ts#L77) |

## 一个接口中的多个命令

这里只列验证器里的 action；operation、问卷分支与工作流内部状态仍要细分，不能以一个 POST 算一个功能。

| 验证器 | action |
| --- | --- |
| [medusa-store/apps/backend/src/api/admin/devices/validators.ts](../../medusa-store/apps/backend/src/api/admin/devices/validators.ts) | activate, assign, cancel_fulfillment, create_fulfillment, delete, enroll, fulfill, inspect, intake, make_available, order_assign, receive, release, repair_ship, retire, save_allocation, ship, source, supplier_assign, supply, transfer_return, update |
| [medusa-store/apps/backend/src/api/admin/mail/validators.ts](../../medusa-store/apps/backend/src/api/admin/mail/validators.ts) | campaign, contacts, delete_account, delete_template, messages, save_account, save_campaign, save_draft, save_template, send, unsubscribe_contact |
| [medusa-store/apps/backend/src/api/buyback-pricing-validators.ts](../../medusa-store/apps/backend/src/api/buyback-pricing-validators.ts) | adjust, reference, save |
| [medusa-store/apps/backend/src/api/buyback-validators.ts](../../medusa-store/apps/backend/src/api/buyback-validators.ts) | accept_offer, approve_shipping, cancel, decline_offer, final_offer, link_device, mark_paid, mark_received, mark_returned, mark_returning, mark_shipped, request_return, review_photo, submit_photos |
| [medusa-store/apps/backend/src/api/seo-validators.ts](../../medusa-store/apps/backend/src/api/seo-validators.ts) | delete, google-test, indexnow, run, save, scan |

## admin_pages（23）

- [medusa-store/apps/backend/src/admin/routes/device-scan/inspection/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/inspection/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/intake/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/intake/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/lookup/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/lookup/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/orders/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/orders/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/prices/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/prices/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/recycled/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/recycled/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/requests/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/requests/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/returned/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/returned/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/device-scan/sources/page.tsx](../../medusa-store/apps/backend/src/admin/routes/device-scan/sources/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/mail/page.tsx](../../medusa-store/apps/backend/src/admin/routes/mail/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/citation-monitor/page.tsx](../../medusa-store/apps/backend/src/admin/routes/seo/citation-monitor/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/crawler-logs/page.tsx](../../medusa-store/apps/backend/src/admin/routes/seo/crawler-logs/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/page.tsx](../../medusa-store/apps/backend/src/admin/routes/seo/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/providers/page.tsx](../../medusa-store/apps/backend/src/admin/routes/seo/providers/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/seo/traffic-sources/page.tsx](../../medusa-store/apps/backend/src/admin/routes/seo/traffic-sources/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/buy-policies/page.tsx](../../medusa-store/apps/backend/src/admin/routes/settings/buy-policies/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/customer-service/page.tsx](../../medusa-store/apps/backend/src/admin/routes/settings/customer-service/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/mailboxes/page.tsx](../../medusa-store/apps/backend/src/admin/routes/settings/mailboxes/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/mollie/page.tsx](../../medusa-store/apps/backend/src/admin/routes/settings/mollie/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/warehouse-access/page.tsx](../../medusa-store/apps/backend/src/admin/routes/settings/warehouse-access/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/settings/webhook-notifications/page.tsx](../../medusa-store/apps/backend/src/admin/routes/settings/webhook-notifications/page.tsx)
- [medusa-store/apps/backend/src/admin/routes/shipping-demo/page.tsx](../../medusa-store/apps/backend/src/admin/routes/shipping-demo/page.tsx)

## widgets（6）

- [medusa-store/apps/backend/src/admin/widgets/mail-topbar.tsx](../../medusa-store/apps/backend/src/admin/widgets/mail-topbar.tsx)
- [medusa-store/apps/backend/src/admin/widgets/order-after-sales.tsx](../../medusa-store/apps/backend/src/admin/widgets/order-after-sales.tsx)
- [medusa-store/apps/backend/src/admin/widgets/order-devices.tsx](../../medusa-store/apps/backend/src/admin/widgets/order-devices.tsx)
- [medusa-store/apps/backend/src/admin/widgets/order-shipping.tsx](../../medusa-store/apps/backend/src/admin/widgets/order-shipping.tsx)
- [medusa-store/apps/backend/src/admin/widgets/product-reviews.tsx](../../medusa-store/apps/backend/src/admin/widgets/product-reviews.tsx)
- [medusa-store/apps/backend/src/admin/widgets/selectable-messages.tsx](../../medusa-store/apps/backend/src/admin/widgets/selectable-messages.tsx)

## workflows（29）

- [medusa-store/apps/backend/src/workflows/buyback-catalog.ts](../../medusa-store/apps/backend/src/workflows/buyback-catalog.ts)
- [medusa-store/apps/backend/src/workflows/buyback.ts](../../medusa-store/apps/backend/src/workflows/buyback.ts)
- [medusa-store/apps/backend/src/workflows/cancel-order-with-reason.ts](../../medusa-store/apps/backend/src/workflows/cancel-order-with-reason.ts)
- [medusa-store/apps/backend/src/workflows/create-after-sales.ts](../../medusa-store/apps/backend/src/workflows/create-after-sales.ts)
- [medusa-store/apps/backend/src/workflows/customer-notifications.ts](../../medusa-store/apps/backend/src/workflows/customer-notifications.ts)
- [medusa-store/apps/backend/src/workflows/customer-service.ts](../../medusa-store/apps/backend/src/workflows/customer-service.ts)
- [medusa-store/apps/backend/src/workflows/device-operations.ts](../../medusa-store/apps/backend/src/workflows/device-operations.ts)
- [medusa-store/apps/backend/src/workflows/device-photos.ts](../../medusa-store/apps/backend/src/workflows/device-photos.ts)
- [medusa-store/apps/backend/src/workflows/device-refund.ts](../../medusa-store/apps/backend/src/workflows/device-refund.ts)
- [medusa-store/apps/backend/src/workflows/device-source.ts](../../medusa-store/apps/backend/src/workflows/device-source.ts)
- [medusa-store/apps/backend/src/workflows/device-supplier-account.ts](../../medusa-store/apps/backend/src/workflows/device-supplier-account.ts)
- [medusa-store/apps/backend/src/workflows/enroll-all-device-stock.ts](../../medusa-store/apps/backend/src/workflows/enroll-all-device-stock.ts)
- [medusa-store/apps/backend/src/workflows/expire-buyback-photos.ts](../../medusa-store/apps/backend/src/workflows/expire-buyback-photos.ts)
- [medusa-store/apps/backend/src/workflows/mail-command.ts](../../medusa-store/apps/backend/src/workflows/mail-command.ts)
- [medusa-store/apps/backend/src/workflows/mail-webhook.ts](../../medusa-store/apps/backend/src/workflows/mail-webhook.ts)
- [medusa-store/apps/backend/src/workflows/mobile-intake.ts](../../medusa-store/apps/backend/src/workflows/mobile-intake.ts)
- [medusa-store/apps/backend/src/workflows/process-mail-queue.ts](../../medusa-store/apps/backend/src/workflows/process-mail-queue.ts)
- [medusa-store/apps/backend/src/workflows/product-reviews.ts](../../medusa-store/apps/backend/src/workflows/product-reviews.ts)
- [medusa-store/apps/backend/src/workflows/reconcile-device-stock.ts](../../medusa-store/apps/backend/src/workflows/reconcile-device-stock.ts)
- [medusa-store/apps/backend/src/workflows/registration-email.ts](../../medusa-store/apps/backend/src/workflows/registration-email.ts)
- [medusa-store/apps/backend/src/workflows/resume-mollie-session.ts](../../medusa-store/apps/backend/src/workflows/resume-mollie-session.ts)
- [medusa-store/apps/backend/src/workflows/save-buy-policies.ts](../../medusa-store/apps/backend/src/workflows/save-buy-policies.ts)
- [medusa-store/apps/backend/src/workflows/save-mollie-config.ts](../../medusa-store/apps/backend/src/workflows/save-mollie-config.ts)
- [medusa-store/apps/backend/src/workflows/seo-command.ts](../../medusa-store/apps/backend/src/workflows/seo-command.ts)
- [medusa-store/apps/backend/src/workflows/unsubscribe-mail.ts](../../medusa-store/apps/backend/src/workflows/unsubscribe-mail.ts)
- [medusa-store/apps/backend/src/workflows/update-after-sales.ts](../../medusa-store/apps/backend/src/workflows/update-after-sales.ts)
- [medusa-store/apps/backend/src/workflows/update-shipment-tracking.ts](../../medusa-store/apps/backend/src/workflows/update-shipment-tracking.ts)
- [medusa-store/apps/backend/src/workflows/warehouse-access.ts](../../medusa-store/apps/backend/src/workflows/warehouse-access.ts)
- [medusa-store/apps/backend/src/workflows/webhook-notifications.ts](../../medusa-store/apps/backend/src/workflows/webhook-notifications.ts)

## jobs（3）

- [medusa-store/apps/backend/src/jobs/expire-buyback-photos.ts](../../medusa-store/apps/backend/src/jobs/expire-buyback-photos.ts)
- [medusa-store/apps/backend/src/jobs/process-mail-queue.ts](../../medusa-store/apps/backend/src/jobs/process-mail-queue.ts)
- [medusa-store/apps/backend/src/jobs/seo-monitor.ts](../../medusa-store/apps/backend/src/jobs/seo-monitor.ts)

## subscribers（4）

- [medusa-store/apps/backend/src/subscribers/customer-notifications.ts](../../medusa-store/apps/backend/src/subscribers/customer-notifications.ts)
- [medusa-store/apps/backend/src/subscribers/iphone-product-images.ts](../../medusa-store/apps/backend/src/subscribers/iphone-product-images.ts)
- [medusa-store/apps/backend/src/subscribers/registration-email.ts](../../medusa-store/apps/backend/src/subscribers/registration-email.ts)
- [medusa-store/apps/backend/src/subscribers/webhook-notifications.ts](../../medusa-store/apps/backend/src/subscribers/webhook-notifications.ts)

## 自定义模块

| 模块 | 实体 |
| --- | --- |
| [medusa-store/apps/backend/src/modules/after-sales/index.ts](../../medusa-store/apps/backend/src/modules/after-sales/index.ts) | after_sales_request |
| [medusa-store/apps/backend/src/modules/buyback/index.ts](../../medusa-store/apps/backend/src/modules/buyback/index.ts) | buyback_request, buyback_photo, buyback_price, buyback_device_link |
| [medusa-store/apps/backend/src/modules/customer-notifications/index.ts](../../medusa-store/apps/backend/src/modules/customer-notifications/index.ts) | customer_notice |
| [medusa-store/apps/backend/src/modules/devices/index.ts](../../medusa-store/apps/backend/src/modules/devices/index.ts) | device, device_source, device_variant, device_supply, device_assignment, device_event |
| [medusa-store/apps/backend/src/modules/mail/index.ts](../../medusa-store/apps/backend/src/modules/mail/index.ts) | mail_account, mail_message, mail_contact, mail_campaign, mail_delivery, mail_event, mail_template |
| [medusa-store/apps/backend/src/modules/mollie/index.ts](../../medusa-store/apps/backend/src/modules/mollie/index.ts) | Provider / 数据库编号迁移；需单独审查 |
| [medusa-store/apps/backend/src/modules/mollie-settings/index.ts](../../medusa-store/apps/backend/src/modules/mollie-settings/index.ts) | mollie_config |
| [medusa-store/apps/backend/src/modules/order-numbering/index.ts](../../medusa-store/apps/backend/src/modules/order-numbering/index.ts) | Provider / 数据库编号迁移；需单独审查 |
| [medusa-store/apps/backend/src/modules/product-review/index.ts](../../medusa-store/apps/backend/src/modules/product-review/index.ts) | product_review |
| [medusa-store/apps/backend/src/modules/seo/index.ts](../../medusa-store/apps/backend/src/modules/seo/index.ts) | seo_setting, seo_page, seo_source, seo_crawler_visit, seo_traffic_visit, seo_provider, seo_monitor, seo_run, seo_evidence, seo_submission |
| [medusa-store/apps/backend/src/modules/sku-numbering/index.ts](../../medusa-store/apps/backend/src/modules/sku-numbering/index.ts) | Provider / 数据库编号迁移；需单独审查 |
| [medusa-store/apps/backend/src/modules/webhook-notifications/index.ts](../../medusa-store/apps/backend/src/modules/webhook-notifications/index.ts) | webhook_endpoint |
