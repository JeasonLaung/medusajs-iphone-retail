# PayPal 钱包前台至订单确认

2026-11-03，承接原前台 PayPal 钱包的服务端创建订单、SDK批准、服务端扣款及成单交互。原 Medusa UI 插件绑定旧接口，因此在现有 Astro 付款/复核组件内对接 Gin；保留原布局、金额读取和 Mollie 入口。PayPal Card、webhook、真实 PayPal 沙箱和整期 C01—C11 尚未验收。

backend `.env` 可显式设置 `PAYPAL_WALLET_ENABLED=true`，并配置 `PAYPAL_CLIENT_ID`、`PAYPAL_CLIENT_SECRET`、`PAYPAL_MERCHANT_ID`、`PAYPAL_ENVIRONMENT=sandbox`、`PAYPAL_INTENT=CAPTURE`（或原授权模式 `AUTHORIZE`）。同样需要现有支付开关、独立加密 keyring、已启用 provider 及地区绑定；配置加载本身不写门禁或请求渠道。默认关闭，关闭时不读 PayPal 凭证。主本机没有配置 PayPal，继续现有 Mollie 测试付款。本任务没有新增后台密钥表单。

私有 payment-options 仅向当前购物车投影 SDK 所需的公开 client ID、币种和 intent/environment；secret、token、渠道完整响应均不返回。冻结后从原加密请求识别渠道并提供同一 SDK 配置；当前收据锁需要普通事务，MySQL READ ONLY 不允许该锁，接口仍不修改业务数据。不支持的钱包币种不影响 Mollie 可用性。

前台选择 PayPal 后，原付款意图记录额外保存固定 provider ID。创建失败、丢响应、刷新仍使用同一 UUID、原版本和渠道，不允许切换到 Mollie 重发。SDK `createOrder` 返回正式接口给定的原订单 ID；`onApprove` 先核对同一订单，再用空 JSON 调用正式私有 `paypal-settlement`。服务端独立核验并决定扣款/授权。取消、SDK错误及错误订单回调不触发扣款。独立“已批准付款？查询并继续”只查询渠道，不发起新扣款。成功转原 payment-result，再由既有正式完成接口及确认页读取结果；浏览器回调不能直接记实收。新增 `/api/paypal-return` 复用原只读回跳逻辑。

验证使用固定地址/配送完成检查点，没有重跑选品、账号或履约。运行：

```powershell
node retail/scripts/test-context-payment-joint.mjs --paypal-wallet
node retail/scripts/test-context-payment-joint.mjs --scenario continuous --matrix
```

Windows 使用 Node ≥22.23.4；本次使用已有 Node 24 和 Chromium。运行器冻结源码，在自有目录构建 Astro，使用自有 MySQL 9.6/Redis，实际 Gin 私有 HTTP、持有凭证、CSRF 和业务事务。只模拟外部 PayPal SDK 与渠道，未使用真实 PayPal 凭证或修改主订单。报告见 [联合证据](paypal-wallet-native-report.json)。

- owner `46319a590392`：中英×390/1440 四组实际页面；创建响应丢失后刷新恢复，SDK取消/错误订单均零扣款，扣款响应丢失后正式查询恢复，成单响应丢失后关闭购物车收据恢复。每组 CREATE=1、settlement=1、订单/完成收据/实收capture各1，原 UUID、精确158.75 EUR及真实provider不变；确认页显示 PayPal、清理原记录且无水平溢出。最终四订单、四capture、四收据和四邮件出站记录；没有实际发信。
- owner `702cdbc9b0df`：原 Mollie 连续保存/恢复中英×390/1440四组回归通过，受保护库存/订单/实收/邮件事实不变。
- 前端261项单测通过，Astro check 0 error/0 warning（29条既有提示）；cart、checkoutconfig、payments、identity、cmd/api局部Go测试通过。未启用独立SQL环境的普通Go测试不充当联合证据。

首次隔离运行 `8b85af6609b3` 暴露 READ ONLY 事务拒绝收据锁，修正后从相同固定检查点重跑通过。失败日志保留于忽略目录。两次成功运行均核验冻结源码、清理自有进程/容器/网络/匿名卷；真实 PayPal 沙箱、卡字段及 webhook 继续作为主链路待办。

主预览经既有 start-retail.ps1 受控更新后，9100 health、9200 Admin、9300商店均200，近期日志正常。无购物车凭证访问结账页按原逻辑404；带凭证的付款页/确认页由上述隔离联合覆盖。旧Medusa端口保持关闭。
