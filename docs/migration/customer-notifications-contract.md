# 新客户站内通知迁移契约

范围为原前台页头铃铛、未读数、分页、单条/全部已读及查看详情；保留原 DOM、CSS 和中英文文案。这份契约不宣称功能已实现，也不代替通知邮箱任务。

## 已核对来源和当前缺口

原 `subscribers/customer-notifications.ts` 监听 shipment.created、delivery.created、order.canceled、after_sales.processing、after_sales.completed、after_sales.rejected。售后事件按 `event:request_id` 去重，发货按原履约资源去重；资源详情指向订单。`workflows/update-after-sales.ts` 同状态重存不发事件。`workflows/customer-notifications.ts` 的 buyback.updated 对同一回收申请合并一条未读提醒，并在与已读操作竞争时保留新提醒。

原 `api/store/notifications/route.ts` 每页20条，created_at/id倒序，返回 notifications/count/unread/offset/limit；POST空对象全部已读，传id单条已读，写入本身限定当前客户和read_at为空。`modules/notifications/notification-bell.tsx` 每30秒在可见页面刷新、窗口focus刷新、打开抽屉刷新，保留失败重试、分页、未读标识和查看详情后跳转。当前 Astro `gin-header.tsx` 对登录客户传 notification:null，当前 Gin customer 路由、customer bridge白名单与customer client均无通知操作。不能以现有订单进度页面代替通知入口。

## 身份、数据与事件边界

只向新 customer_accounts 主体投递。旧 customer_notice、历史 customer_id、订单邮箱或匹配邮箱不能授权新用户读取；旧账号不迁移。订单通知从 customer_order_ownership 的有效新客户归属解析，不直接使用可能仍为历史主体的 orders.customer_id。无有效新归属时不为该订单自动创建可读通知，也不能通过通知建立订单归属。

新通知持久表需保存 customer_id、event_key、kind、resource_id、reference、action、read_at和时间，并提供事件唯一性、客户时间和客户未读索引。旧通知若后续保留，应与新收件箱物理或明确主体隔离；本阶段不导入旧通知。

售后状态更新、人工包裹发货与对应新通知应在同一个数据库事务内提交；晚期通知/操作收据失败须全部回滚，原请求重放不重新通知，同状态重存不重复提醒。对新账户的读取/已读沿用 addressTransaction 的账户锁、锁后 Redis 会话复核、enabled/verified/auth_version保护。消息只提供业务参考号和受允许的事件/action，不包含地址、邮箱、支付标识、设备成本或内部metadata。

## HTTP与前端

采用 `/api/store/customer/notifications` GET/POST，复用现有客户会话、签名 bridge、可信 Origin、CSRF和私有缓存。GET只允许单个offset十进制整数0..100000，不能让offset影响账户过滤；POST严格接受{}或{id:string}，拒绝未知/重复/null和query字段。单条跨客户id不能修改记录或暴露另一主体。全部已读的更新条件包含当前账户和read_at IS NULL，重复调用保持幂等。

Astro沿用现有 customerClient 和桥接层，直接对Gin通信。铃铛组件迁移原弹窗结构、样式、轮询和文案；退出/换账户/分页请求竞争不得将上一账户或过期页面结果写回。查看详情只组装站内 `/{segment}/account/orders/{id}` 或 buyback路径，resource_id编码且不得解释任意URL。失败保持上次可信数据并展示原有错误状态。

## 验收证据

需要真实 MySQL/Redis 验证新主体隔离、排序分页和全局未读数、单条/全部已读、同状态无通知、命令重试去重、并发状态变化/已读、撤权/禁用/旧会话、晚期失败整体回滚。浏览器需运行真实Gin链路，验证发货与售后生成消息、页头未读徽标、读后未读数、详情跳转、20条分页、中英文及390px页面。邮件发送另按现有邮箱配置、持久队列和发送结果验证，不把站内通知成功当作邮箱已发送。


## 2026-10-30 实现边界

0072 新建 customer_account_notices，仅保存新主体收件箱；没有复制旧账号或旧通知。查询/已读接入现有 Gin customer 私有路由、会话复核与 CSRF，Astro customer bridge 与 client 已放行严格通知路径。页头登录态改为保留实际客户ID并给铃铛设置key，账号切换会卸载旧消息状态；通知轮询使用请求代次，避免旧分页或卸载后响应覆盖当前页面。DOM、样式和中英文文案来自原通知组件，导航改为 Astro 站内页面。

已接入的业务事件为售后 processing/completed/rejected（仅状态实际变化时）和人工包裹 shipment.created。消息与业务/收据在同一事务，既有请求重放不重复投递。其他已定义kind的消费文案保留，但 delivery.created、order.canceled 与 buyback.updated 的新业务写入入口仍待迁移，不宣称已经生成这些通知。收件箱不是邮件队列，没有因此发送邮件。

收件人通过当前新订单归属及有效新账户解析；没有归属则不投递。事件唯一键为kind:业务资源ID，资源链接仍为所属订单。消息不携带原邮箱、客户内部字段、支付或设备成本。新表故意不通过客户外键在事件提交时取得账户共享锁，避免后台先锁订单与客户先锁账户的读取形成锁顺序反转；客户有效性由归属查询和每次私有读取认证保证，账号未提供物理删除入口。

验证包含真实MySQL的23条消息分页、未读计数、并发单条已读、全部已读、跨账号无操作、禁用/匿名拒绝、事件无归属不投递及去重、调用方回滚和晚期插入失败回滚，以及签名HTTP的Origin/CSRF/严格JSON/query校验。原 Ant 换机发货真实浏览器联合进一步核对两次发货和两次售后状态实际生成四条新客户通知。Astro铃铛浏览器使用合成API单独验证交互，不能代替完整Astro→Gin收件箱联合验收；后者、邮件发送与回收未读合并继续待办。
