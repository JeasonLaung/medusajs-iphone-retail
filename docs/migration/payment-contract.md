# 支付与订单完成基线

日期：2026-10-27。覆盖 T002/T005/T007/T035/T036 的第一轮源码核对；尚无 Gin 支付实现或外部沙箱验证。

## 证据

项目源码：`backend/medusa-config.ts`、`backend/src/api/payment-guards.ts`、`backend/src/modules/mollie/service.ts`、`backend/src/workflows/resume-mollie-session.ts`、`backend/src/api/store/payment-collections/[id]/payment-sessions/route.ts`、`backend/src/lib/order-device-payment.ts`；前台 `storefront/src/lib/data/payment.ts`、`storefront/src/lib/util/payment-session.ts`、`storefront/src/app/api/mollie-return/route.ts`。此处 backend/storefront 均位于 `medusa-store/apps/`。

PayPal 使用本机已安装 `@easypayment/medusa-payment-paypal@1.3.2`；包 exports 实际指向 `.medusa/server/src`。本轮展开的是该包 `src/api/store/paypal/{config,webhook}/route.ts`、`src/modules/paypal/webhook-processor.ts` 和 `src/jobs/paypal-webhook-retry.ts`，仍需核对对应编译产物及项目补丁的差异后冻结运行契约，不以包名/README 代替实际运行代码。

## 方法选择与生产边界

- 前台按 region 请求可用 provider，生产过滤 system provider。PayPal 还需取得 client_id、币种支持与 enabled 开关；高级卡付款另需 client_token。失败时展示不可用状态，不能偷偷退回手工付款。
- 生产服务端同时拒绝手工 provider 的选择、mark-as-paid、授权、扣款和带活跃手工 session 的购物车完成。开发模式允许不构成生产功能。
- 当前已配置 PayPal、PayPal Card 与 Mollie；Stripe 依赖存在不能证明启用。
- 设备配货要求所有 payment collection 均为 completed；authorized 不能视作已收款。

## Mollie

| 场景 | 已核对实际规则 | 新实现约束 |
| --- | --- | --- |
| 创建 | session_id 写进 provider metadata；cart_id、回跳地址和 attempt key 绑定该次支付 | 先持久化稳定操作键，超时重试复用键；不靠一次 HTTP 请求存活保证唯一 |
| 回跳地址 | 仅配置的 storefront origin、`/api/mollie-return`、匹配 cart_id；禁止用户名密码 | 保留旧链接入口；新 session 的标识方案与兼容层需明确 |
| 金额变更 | 查询 provider；paid/authorized 禁止改价；未付旧支付取消后以新 attempt 创建 | 精确十进制比较和格式化，替换结果持久化；旧代码 Number/toFixed 的浮点实现不照搬 |
| 取消 | 已付/授权拒绝；终态取消成功返回；open 且不可取消时保留远端信息 | 不能将仍可能收款的远端尝试遗忘；纳入后续对账 |
| session 恢复 | 先刷新全部 Mollie sessions；多个 authorized 拒绝；优先已付，否则选择金额币种匹配的活跃 session | 多标签/重试只能选一个可处理 session；已付金额不匹配拒绝，不能新建另一笔掩盖异常 |
| Webhook | 请求只接受合法 tr_ ID，使用商户凭据重新读取 provider 状态；metadata 定位 payses_ | 不信请求提交的 status/amount；同时核对商户、session、币种、金额与尝试归属 |
| 状态 | paid→successful；authorized→authorized；canceled→canceled；expired/failed→failed；pending→pending；open→not_supported | 保留授权/实收区别，乱序通知不能回退已确认事实 |
| 退款 | 以原 payment ID 发起部分退款，保存 last_refund，不覆盖原支付 ID | 旧包装层未显式传稳定退款幂等键；新系统须补持久化退款指令、远端结果恢复，不能据此声称旧代码具备全链路退款幂等 |

`/api/mollie-return` GET 只重定向到本地 payment-result 页面，不能因浏览器已返回就显示付款成功。POST 校验 Origin、购物车 cookie/支付记忆或已登录归属后，向后端尝试完成订单；查询参数的 cart_id 本身不能赋予权限。结果分别为 invalid / error / failed / pending / confirmed；成功清除当前购物车 cookie，支付记忆保留七天，缓存维护失败不能掩盖已成功的订单。

旧账号不迁移，因此旧认证 cookie 不应成为新系统登录身份。若将来切换时存在在途支付，必须给出显式的短期恢复/对账策略；不能以同邮箱新账号替代原主体验证。

## PayPal 插件源码揭示的边界（待编译产物确认）

- `/hooks/paypal/webhook` 与旧 `/store/paypal/webhook` 共用 handler，前者避免 store publishable-key 中间件阻挡第三方通知。兼容入口不可漏。
- 结构校验包含 event ID/type、transmission time，默认绝对时间差不超过 60 分钟。通过商户环境对应 webhook ID 和 PayPal 验签接口验证；保留 raw body，证书 URL 限制到指定 PayPal HTTPS 主机。
- transmission ID 与 event ID 去重；已验签事件先持久化，再异步消费并快速 ACK。异步队列失败由持久记录恢复，不能只放 Redis 或内存。
- 重试间隔 2/10/30/60/120 分钟，初次加五次重试，共六次。默认超过五分钟仍 processing 的记录可恢复；每次扫描分别最多取 100 个 failed 和 100 个 processing 候选。失败原因不可重试或次数耗尽则 dead_letter。
- 原任务使用 PostgreSQL advisory lock，且失败可退为无锁执行。MySQL 迁移须重写事件认领、租约和事务，不复制无锁退化路径；外部 API 调用不占长事务。
- captured 不能被晚到的 authorized 覆盖。部分退款合并 refund ID 历史但保持付款状态；全额退款/撤销另记事实。退款 resource ID 不能误写成 capture ID。
- 已扣款但浏览器未完成购物车时，后台通知负责补订单。需要同 cart 唯一成单约束，而不是每个调用入口独立保证。
- 原源码用浮点及 0.005 容差判定部分退款，金额未知时可能沿用全额退款处理；这是待纠正的实现风险，不能作为精确金额契约直接继承。

## 固定验收规格

| 编号 | 用例 | 预期 |
| --- | --- | --- |
| P01 | 生产直接调用手工选择/mark-as-paid/授权/扣款/完成 | 全部拒绝，库存与订单无变化 |
| P02 | 双标签同时创建/恢复同一购物车支付 | 稳定尝试键，单一可处理支付；不重复扣款 |
| P03 | 金额变化、已付后改价、多个已付 session | 未付可替换，后两者拒绝并可对账 |
| P04 | 伪造 webhook status/amount，跨商户/币种/会话 | 不产生付款或订单事实 |
| P05 | 同事件重放、不同 transmission 同事件、乱序 authorized | 幂等、无状态回退 |
| P06 | 验签成功落库后队列失败、worker 中途退出 | 可恢复且每次业务副作用只执行一次 |
| P07 | 浏览器先返回、通知先到、浏览器永不返回 | pending 可重查，实收唯一成单 |
| P08 | 无 cookie 的 cart_id、非归属客户、恶意 Origin | 不显示私有订单且不完成别人购物车 |
| P09 | 同购物车 completion 并发、超时后重试 | 单订单、单库存预留/扣减，可返回已成功结果 |
| P10 | 两次部分退款、重复指令、远端成功本地超时 | 原支付 ID 保留，精确累计不超实收，重试不重复退款 |
| P11 | 授权成功但未实收时请求设备配货 | 拒绝；确认实收后才允许 |
| P12 | 多 worker 同时恢复失效租约、次数耗尽 | 单次认领，dead-letter 可查可审计 |

以上P01—P12仍为完整集成规格，尚未在新系统运行。已有[原生保护基线](native-guard-contract.md)对实际生产付款guard运行人工端口样例，覆盖手工选择/授权/扣款/完成和provider列表；不证明库存、订单或provider无副作用。下一步继续核对 PayPal create/capture/授权与编译产物、原生购物车合计及库存预留，再形成完整 Gin API 定义。
