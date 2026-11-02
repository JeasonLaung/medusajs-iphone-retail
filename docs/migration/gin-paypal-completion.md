# PayPal 已扣款订单成单事务

2026-11-03，继续暂存中的 C06 PayPal 成单改动，保留原 stash 不删除。本项补创建→认证查询/扣款之后的本地订单、库存预留、实收与通知事务，不把钱包/卡SDK或真实沙箱标为完成。

原成单参与者仅支持 Mollie 创建响应ID，并将所有新账务写为 Mollie provider。现在从加密 CREATE 请求及收据确定原渠道：Mollie沿用原ID/观察校验；PayPal钱包和卡使用原专用订单ID读取及完整观察核验。仅已扣款paid证据可进入原事务，仅AUTHORIZED或顶层COMPLETED不代替完整实收。支付行provider_id来自原加密请求，远程证据重新以该本地支付ID封存，库存、精确金额、促销、订单编号、收据、关闭购物车和邮件出站仍沿原事务一起提交。

当前公开complete请求依旧严格空JSON。省略的渠道在购物车所属锁内从原冻结命令解析；浏览器不能选择渠道、远程订单或金额。内部显式指定错误渠道仍拒绝。没有新增迁移、启用PayPal主配置或更换Mollie测试配置。

## 验收

```powershell
go -C retail/backend test ./...
go -C retail/backend vet ./internal/cart ./internal/payments ./internal/identity
& 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe' retail/scripts/test-context-payment-joint.mjs --paypal-checkout
```

最终 owner `0b45030e98ff`，独立MySQL 9.6原生外键/Redis、真实Gin HTTP与业务事务通过。完整105项显式迁移；钱包/卡各CAPTURE与AUTHORIZE四组：两笔扣款各158.75 EUR、各唯一2.5单位库存预留；授权不记现金/不成单。末尾收据故障使订单、库存、账务、出站、订单编号和购物车版本全表指纹回滚。六并发只创建一个订单，已关闭购物车正式HTTP多次恢复同结果，不增加扣款或账务。Origin/CSRF/凭证/客户端注入字段拒绝，错误渠道拒绝，加密支付证据绑定真实渠道及本地支付行。原创建和扣款八场景亦通过，全部12个子测试通过。

渠道为人工适配器，本项没有调用PayPal真实服务、没有执行PayPal Astro付款页面。独立容器、网络和卷已清理，工作区与冻结源码SHA一致，见[报告](paypal-completion-report.json)。早期运行因旧104迁移断言、缺少专有编号初始化、错误预留owner列等测试准备失败已修正；另一次运行时修改夹具导致冻结源码比对拒绝，该轮不算通过。最终从头独立运行通过，未绕过冻结校验。

原Mollie连续上下文保存/替换/恢复专项 `--scenario continuous` 以 owner `a832d2001cec` 单独通过真实Astro/Gin/MySQL/Redis浏览器联合，冻结源码和清理复核通过，报告包含该轮状态及恢复结果。使用现有Node24及Chromium；早期Node22版本低于Astro要求、默认Chromium路径不存在的运行未算通过。API健康、Admin、商店及商品路由均200，近期日志无持续服务异常。无购物车凭证直接访问checkout返回404保持原语义，不当作故障或验收付款成功。完整交易大流程仍按用户要求留到总任务结束执行一次。后续仍需原SDK钱包/卡、client配置、webhook、未知创建发现及PayPal退款/真实沙箱等，C06与C01—C11保持开放。
