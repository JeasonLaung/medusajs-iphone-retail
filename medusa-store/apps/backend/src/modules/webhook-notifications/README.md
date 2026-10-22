# Webhook notifications

Admin API (requires an authenticated admin):

- `GET /admin/webhook-notifications` lists channels without URLs or credentials.
- `POST /admin/webhook-notifications` creates a channel.
- `POST /admin/webhook-notifications/:id` updates a channel.
- `DELETE /admin/webhook-notifications/:id` deletes a channel.
- `POST /admin/webhook-notifications/:id/test` sends a test using saved settings, including disabled channels.

Create/update fields:

```json
{
  "name": "Operations",
  "provider": "generic",
  "url": "https://automation.example.com/webhook/store-events",
  "enabled": true,
  "language": "en",
  "events": ["order.placed", "order.canceled", "payment.captured", "shipment.created"],
  "keyword": "Store"
}
```

Providers: `feishu` (also Lark), `wecom`, `dingtalk`, `slack`, `mattermost`, `generic`.
Optional `secret` enables Feishu/DingTalk signing. Optional `token` sets Bearer authorization for generic requests. Omit secret/token/url to preserve existing values; an empty secret/token clears it. A provider change requires a new URL and clears omitted credentials. Set `keyword` to a robot's configured security keyword if needed.

URLs and credentials use AES-256-GCM with `WEBHOOK_ENCRYPTION_KEY`, falling back to a non-default `COOKIE_SECRET`. Keep the key stable and provide the same key on all instances. Only public HTTPS destinations on port 443 are accepted. DNS is checked and pinned per connection; redirects are refused.

Generic POST body: `event`, `resource_id`, `text`, `timestamp` (ISO 8601), with `notification` containing `title`, `language`, `lines` and `links` for rich business messages. Use an n8n published production Webhook node with POST enabled, optionally with Bearer header authentication. Feishu uses native `post` rich text with clickable links; other providers include links in their text. Nextcloud Talk and Matrix need an adapter because their native APIs are not compatible with this JSON body.

Notifications read the associated order and include its persisted order number (falling back to `display_id`), email, recipient, shipping address, item names/configurations/SKUs/quantities/unit prices, order total, cumulative captured/refunded amounts, states and creation time. Payment events include the payment amount and cumulative captures/refunds on that payment; totals are never represented as the amount of an individual transaction. After-sales notifications also include request type, status, time, reason and requested SKUs/quantities. Fulfillment events include available tracking numbers and valid HTTP(S) tracking links. Messages for payments without a linked order show payment details and link to the order list.

Set `WEBHOOK_ADMIN_URL` to the publicly accessible Admin origin or full `/app` URL. Links fall back to `MEDUSA_BACKEND_URL`, then the local development backend. Links open the existing order details page, where after-sales can be processed, and retain normal Admin authentication. Set `WEBHOOK_TIME_ZONE` for notification timestamps (default `Asia/Shanghai`). Feishu messages are shortened within its 20 KB limit while retaining action links; long orders can be viewed in full in Admin. The test action sends a rich connection notice with the Admin link, without reading or sending a real order.

Supported subscriptions:

- Orders: `order.placed`, `order.canceled`, `order.completed`.
- Payments: `payment.captured`, `payment.refunded` (payment IDs for both events).
- Fulfillment: `order.fulfillment_created`, `order.fulfillment_canceled`, `shipment.created`, `delivery.created` (fulfillment IDs).
- Customer after-sales requests, including returns, exchanges and repairs: `after_sales.created`, `after_sales.processing`, `after_sales.completed`, `after_sales.rejected` (after-sales request IDs).
- Native Admin after-sales operations: `order.return_requested`, `order.return_received`, `order.exchange_created`, `order.claim_created` (return, exchange or claim IDs).

The event catalog is shared by the API, subscriber and Admin form in `src/lib/webhook-events.ts`. Custom after-sales workflows use grouped `emitEventStep` events that are released only after successful workflow completion. Saving an unchanged status emits no event. Device refunds already use Medusa's `refundPaymentWorkflow`, so they use the same `payment.refunded` subscription without an additional refund notification.

Existing channels keep their selected subscriptions when code is updated; select new events and save to enable them. Disabled and unsubscribed channels are skipped. Events are attempted once per channel, with a 10-second timeout. Failures are recorded as `last_status: failed`; there is no persistent delivery queue or automatic retry. Event-bus redelivery can repeat a notification; delivery is not guaranteed exactly once. Do not rely on this as the sole record of orders or payments. Business detail loading is read-only and happens only for subscribed, enabled channels; customer information and webhook credentials are not written to delivery logs.

References:
- https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot
- https://developer.work.weixin.qq.com/document/path/91770
- https://open.dingtalk.com/document/robots/customize-robot-security-settings
- https://docs.mattermost.com/developers/integrate/webhooks/incoming
- https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.webhook/
