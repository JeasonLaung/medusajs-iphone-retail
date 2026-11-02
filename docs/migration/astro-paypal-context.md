# PayPal 付款上下文保存、恢复与登录接管

2026-11-03，C02/C06 原付款连续性接续。复用现有国家/语言弹窗、付款恢复记录和客户登录接管；未改页面样式、付款方式名称或创建命令的版本规则。本轮验证金额及币种不变时保留原 PayPal 钱包/卡付款，C01—C11 整体仍开放。

修复三处实际衔接：

- 国家/语言保存前重放原创建命令时传递持久记录内的原 PayPal provider，并接受 `sdk_ready`。保留原创建 UUID 和输入版本，不以购物车的新版本重新创建付款。
- 保存/恢复及登录接管完成后，根据服务端明确返回的付款绑定及同版本付款方式读取，重建原 provider/UUID/版本记录。已知 PayPal 的渠道投影缺失或混入另一渠道时拒绝覆盖，保留恢复指令。
- Gin 恢复事务的最后一次正常消费者校验，从已认证的原始或补偿创建请求读取 provider，避免省略渠道后默认 Mollie 而返回409。原事务权限、金额、版本和会话校验继续执行。

独立固定检查点命令：

```powershell
node retail/scripts/test-context-payment-joint.mjs --paypal-context
node retail/scripts/test-context-payment-joint.mjs --scenario continuous --matrix
```

要求 Node≥22.23.4；本次使用现有 Node24 与 Chromium。运行器冻结独立源码副本并在副本构建 Astro，使用独立 MySQL9.6/Redis、实际 Gin/Astro 接口和浏览器。外部 PayPal SDK/渠道为人工替身，不使用真实支付密钥，不代表真实 PayPal 沙箱或3DS验收。恢复用私有 fixture 调用正式 `StageContextPayment` 固定零外部效果的未完成检查点；随后通过正式恢复接口完成及重试，没有用SQL伪造成功状态。

证据见 [联合报告](paypal-context-native-report.json)：

- owner `d121dacf1a2c`，12组：钱包/卡 × 保存/恢复/首次登录接管 × 英文390/中文1440。保存及恢复首次成功响应丢失，刷新后沿用原指令；登录接管同样丢失成功响应并刷新恢复。URL查询及锚点、原付款方式名称均保留，重新进入付款页仍显示原钱包或卡表单。
- 每车原创建输入版本4、激活版本5、完成后的有效版本6；原付款UUID/provider保持，CREATE=1、settlement=0、Mollie CREATE/DELETE=0、有效会话=1、金额158.75 EUR。登录组唯一客户归属及接管收据各1。库存、预留、订单、实收及通知出站表的全行指纹未变。
- owner `4e934f080425`，原 Mollie 连续保存/恢复的中英×390/1440四组回归通过，覆盖共享记录重建和补偿创建校验。
- 前端265项单测、Astro check 0 error/0 warning及29项既有提示；payments/cart/checkoutconfig/identity/cmd-api Go局部测试通过。独立源码冻结校验、自有进程/容器/网络/匿名卷清理通过。截图及原始日志保存在忽略目录。

首轮 `5dc7a7fea17e` 暴露正式恢复校验遗漏provider的409，修复后重跑。第二轮 `135d78dfdec0` 的外部替身iframe默认尺寸造成窄屏溢出，按托管字段容器设置替身尺寸后重跑；产品样式未变。

通过现有 `scripts/start-retail.ps1` 受控加载最终后端；9100 health、9200 Admin及9300/gb-en/store均200。无购物车凭证访问结账按原行为404；实际带凭证付款页由上述联合验证。近期API、前台和worker日志未见新增异常。旧Medusa9001保持停止。

金额或币种变化时的 PayPal 会话退役、外部实际取消/查询恢复、真实 PayPal 沙箱、3DS和webhook仍待完成。主本机继续Mollie，未开启PayPal。本轮不能作为上述剩余能力或C06整体验收依据。
