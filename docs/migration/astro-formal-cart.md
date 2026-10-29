# 正式 Astro 加购与购物车

2026-10-29，C06 局部交付，结账/支付和全站仍未验收。

正式商品页 `/{countryCode}/products/{handle}` 启用已有 Gin 加购协调器，正式 `/{countryCode}/cart` 挂载原购物车组件。SSR 只读取公开地区/商品/页面数据，不读取或序列化客户购物车；浏览器凭 HttpOnly possession Cookie 恢复私有购物车。初始化错误保留错误，不能当成空购物车或自动新建。商品配置仍可独立切换。

Astro 与 Gin 必须配置相同 `STOREFRONT_ORIGIN`（不带末尾 `/`）。未配置或请求 origin 不匹配时不启用商品加购和购物车命令。`GIN_API_ORIGIN` 是服务器私有固定目标。Astro 仅允许本机 Host，正式域名支持仍在后续部署范围，本次不操作服务器。

固定私有转接白名单：GET cart/current、cart snapshot、amounts、view；POST cart-sessions、carts、line-items 新增/绝对数量/零值删除。拒绝查询参数、任意路径、其他方法及跨来源请求，写入必须有匹配 Origin。只转发当前 HTTP/HTTPS 模式下的四个购物车 Cookie、CSRF 与幂等键，丢弃管理员/Medusa/客户认证及所有转发头；固定公开 Host 与 Origin 保留 Gin 自身校验。Node fetch 实测忽略自定义 Host，因此使用固定 URL 的原生 http/https 传输，禁止重定向和自动重试。

返回 `private, no-store`，限制请求 16 KiB、响应 4 MiB、传输 11 秒。只允许创建 scope/cart 时返回对应成对 Cookie；校验名称、64位十六进制值、Path=/、SameSite=Lax、Secure/HttpOnly、重复属性及 Domain 禁令。保留有业务意义的失败状态和受限 request-id/Retry-After，统一隐藏依赖错误正文。失败或丢响应不自动重发，沿用已有非凭证浏览器意图及明确恢复按钮。

购物车当前商品库存观察使用独立只读 `/api/store/products/{id}`，只接受 region_id/quantity，不转发私有信息。库存观察失败保留已存金额和移除能力。当前 SKU 数量修改、删除与恢复后刷新页头数量；跨标签和重新激活页面时也重新观察。新客户端数量使用整数，结算金额与实际库存仍由 Gin/MySQL 决定。

正式路由沿用原布局、商品配置、数量控件、汇总与错误交互，不增加演示价格或假结账入口。购物车仍是草稿；`checkout=false`，地址/促销/运费/税费重算、客户合并、预留、成单、支付、确认与通知继续按交付清单完成。客户认证和持久地区/语言切换仍待迁移，地区不一致的旧车明确失败而非静默换区。

验证使用独占人工 MySQL/Redis 和构建后的正式 Astro Node 服务，通过浏览器访问 Gin，不使用测试 Vite 代理作为正式挂载证据；对应报告为 `astro-formal-cart-report.json`。原组件布局对照继续由 `astro-runtime-report.json` 覆盖，不能将草稿购物车测试当作在线付款验收。
