# PayPal 卡支付至订单确认

2026-11-03，承接原 `@easypayment/medusa-paypal-ui` 的 CardFields 表单和服务端 client token 依赖，扩展现有 Astro 付款组件及 Gin 原付款命令。原卡支付标题、付款复核布局、条款和账单地址继续复用；钱包和 Mollie 保留。真实 PayPal 沙箱、3DS 验证、webhook及整期 C01—C11仍未完成。

backend `.env` 新增独立 `PAYPAL_CARD_ENABLED=false`。显式开启时使用已有 `PAYPAL_CLIENT_ID`、`PAYPAL_CLIENT_SECRET`、`PAYPAL_MERCHANT_ID`、`PAYPAL_ENVIRONMENT`、`PAYPAL_INTENT` 注册原 `pp_paypal_card_paypal_card`；不隐式开启钱包，也不修改数据库 provider/region绑定。主本机没有开启 PayPal，继续 Mollie。

付款方式读取与原创建命令现支持卡渠道，浏览器持久记录绑定原卡渠道/UUID/版本；刷新恢复不得切换钱包或 Mollie重发。原钱包组件内按卡模式加载官方 [CardFields SDK v5](https://developer.paypal.com/v5/expanded/integrate)，保留既有v5集成方式。姓名、卡号、有效期、CVV由 SDK 托管，表单是否有效由 SDK 判断；账单地址由原结账资料传给 SDK，业务API不接收卡号/CVV。姓名和卡号占整行，日期和安全码并排。

`POST /api/store/carts/:id/paypal-client-token` 严格空JSON，沿用原持有凭证、Origin、CSRF、购物车授权及限流；只有已激活的卡付款可请求。事务内验证原冻结请求后提交，再调用固定 PayPal OAuth及 `/v1/identity/generate-token`。仅返回短期浏览器token，不返回OAuth token/secret，不建立付款或修改金额；不持久化该token。接口错误不回显渠道响应。实现依据 [PayPal原集成说明](https://developer.paypal.com/platforms/checkout/advanced/integrate/)。

SDK卡表单仅在有效时提交；`createOrder`返回原正式订单ID，批准回调核对同一订单后调用既有服务端扣款命令。取消、无效表单、错误订单回调不发款；SDK资格不符或加载失败显示原付款错误及重新加载入口，同一冻结付款保持原恢复身份。扣款事实仍由Gin核验，“已批准付款？查询并继续”只观察远端；SDK成功回调不直接记实收。结果页、唯一成单、库存预留、实收及通知出站记录使用原事务。授权模式沿用后端授权结果，不以授权代替实收。

独立子流程命令：

```powershell
node retail/scripts/test-context-payment-joint.mjs --paypal-card
node retail/scripts/test-context-payment-joint.mjs --paypal-wallet
```

使用Node≥22.23.4，本次为已有Node24/Chromium。运行器自有源码副本和构建目录、MySQL9.6/Redis及实际Gin/Astro，不构建或测试改写主数据。固定地址/配送检查点避免每轮完整购物。只模拟外部SDK和PayPal渠道，因此不代表真实银行卡/3DS沙箱已通过。证据见 [联合报告](paypal-card-native-report.json)。

- 卡支付 owner `7984ec2eb3e1`：中英×390/1440四组原付款选择→卡表单→正式扣款→确认页。未开始付款时请求token409、夹带金额400、错误CSRF403、其他车404，均未调用token渠道。每组只生成一次浏览器token。
- SDK取消、错误订单和无效表单均零扣款；随后有效表单提交传递原DK账单地址并使用原订单。创建、扣款、成单的第一次响应分别丢失，刷新或正式查询恢复。每车远端CREATE=1、settlement=1，本地订单/capture/完成收据各1，158.75 EUR及卡provider保持；原付款记录最终清理，确认页显示原卡支付名称。四订单、四capture、四通知出站记录，无实际发信；无水平溢出。
- 钱包 owner `c6f4eac8f54e`：原四组联合重新通过，验证共享SDK/组件扩展未破坏钱包付款及三处丢响应恢复。
- 前端262项单测通过，Astro check 0 error/0 warning、29既有提示。payments/cart/checkoutconfig/identity/cmd-api局部Go测试通过；token适配器单测核对固定端点、OAuth隔离、无卡数据输入、拒绝私有/无效token、无订单/扣款调用。

首轮 `29a992dbd7e7` 浏览器断言误用卡方式名称，未改产品文案，脚本改用原名称后重跑通过。成功运行冻结源码校验及自有进程/容器/网络/匿名卷清理通过。公开报告不包含密钥或浏览器token；原始隔离日志、表单及确认页截图保存在忽略目录。

主本机通过既有 start-retail.ps1受控更新，9100 health、9200 Admin、9300商店均200，近期API/前台日志正常。未带购物车凭证访问结账页按原行为404；实际带凭证卡支付/确认页由隔离联合覆盖。旧Medusa服务未启动。
