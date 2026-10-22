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

Generic POST body: `event`, `resource_id`, `text`, `timestamp` (ISO 8601). Use an n8n published production Webhook node with POST enabled, optionally with Bearer header authentication. Slack/Mattermost use `{ "text": "..." }`. Nextcloud Talk and Matrix need an adapter because their native APIs are not compatible with this JSON body.

Events are attempted once per channel, with a 10-second timeout. Failures are recorded as `last_status: failed`; there is no persistent delivery queue or automatic retry. Do not rely on this as the sole record of orders or payments. Messages include only the event label and resource ID, without customer details. `payment.captured` resource IDs refer to payments; `shipment.created` IDs refer to fulfillments.

References:
- https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot
- https://developer.work.weixin.qq.com/document/path/91770
- https://open.dingtalk.com/document/robots/customize-robot-security-settings
- https://docs.mattermost.com/developers/integrate/webhooks/incoming
- https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.webhook/
