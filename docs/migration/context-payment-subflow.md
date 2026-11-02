# 付款设置保存/恢复：独立子流程验证

2026-11-02。遵照用户最新要求：子任务只运行受影响的接口、状态和页面；完整交易链路留在大任务结束时统一执行。不会为每次局部修改重跑注册→下单→付款→配货→售后→通知全链。

## 固定流程

`TestOwnedContextPaymentBrowser` 使用独立空MySQL 9.6数据库及Redis，应用103迁移一次，载入嵌入的人工基础SQL。商品、库存、价格、税、配送和付款提供方是固定测试基线，没有真实账号、订单、密钥或付款。生产迁移、购物车命令及HTTP处理器保持原样。

控制接口仅编译在Go测试二进制中，以随机密钥保护，不进入应用路由：

| 接口 | 用途 |
| --- | --- |
| `POST /__fixture/new` | 通过现有命令建立一个可结账购物车，返回测试专用possession/CSRF |
| `POST /__fixture/price` | 改变专用商品价格，触发金额变化 |
| `POST /__fixture/original-price` | 恢复专用商品原价格，让同一付款可继续保存设置 |
| `POST /__fixture/mode/:mode` | 人工适配器切换正常、未知删除、未知创建、取消或已付观察结果 |
| `GET /__fixture/facts` | 查询版本、冻结、收据、CREATE/DELETE/观察次数及保护表指纹 |
| `POST /__fixture/case-done` / `finish` | 记录完成的分项数量并结束自有进程 |

浏览器从原结账页面点击准备付款，通过原地区/语言抽屉保存或恢复；写请求实际经过Astro→Gin→MySQL/Redis。故障注入只在真实请求提交成功后丢弃浏览器响应。外部支付由人工适配器承担，不调用真实渠道。

运行需要Go、Node满足前台package.json、Docker已有`mysql:9.6`/`redis:7-alpine`、已安装前台和后台npm依赖以及Playwright浏览器：

```sh
# 日常单场景；另有 changed / restore / delete-unknown / create-unknown / paid
node retail/scripts/test-context-payment-joint.mjs --scenario unchanged
# 本项全部业务状态，默认英文390px，独立于完整交易回归
node retail/scripts/test-context-payment-joint.mjs
# 替代会话已付款/仅授权恢复（不进入订单、配货等后续流程）
node retail/scripts/test-context-payment-joint.mjs --scenario replacement-paid
node retail/scripts/test-context-payment-joint.mjs --scenario replacement-authorized
# 已绑定同客户重登/匿名已有付款金额变化协调
node retail/scripts/test-context-payment-joint.mjs --scenario same-account
node retail/scripts/test-context-payment-joint.mjs --scenario unbound-account
# 匿名已有付款首次绑定：相同精确金额保留，低于分的变化进入取消协调
node retail/scripts/test-context-payment-joint.mjs --scenario unbound-unchanged
node retail/scripts/test-context-payment-joint.mjs --scenario unbound-precision
# 同一购物车连续保存两次→取消后恢复→替代付款继续保存→查询付款
node retail/scripts/test-context-payment-joint.mjs --scenario continuous
# 跨币种：相同数字总额也要取消旧币种付款；失败后恢复原币种
node retail/scripts/test-context-payment-joint.mjs --scenario currency-save --scenario currency-restore
# 旧付款退出后：原购物车数量→优惠→新付款（各命令均注入丢响应）
node retail/scripts/test-context-payment-joint.mjs --scenario retired-edits
# 一次选择多个受影响的场景，共用一次编译及隔离服务
node retail/scripts/test-context-payment-joint.mjs --scenario unbound-account --scenario unbound-delete-unknown
# 仅需要检查本项语言/尺寸组合时使用，仍非完整交易链路
node retail/scripts/test-context-payment-joint.mjs --scenario restore --matrix
```

可选环境变量 `CONTEXT_BROWSER_EXECUTABLE` 指向本机合适的Chrome/Chromium。测试隔离副本构建，不影响应用dev服务；MySQL仅Unix socket、Redis仅容器回环，Gin测试端口只发布宿主127.0.0.1。Docker Desktop用自有bridge发布测试HTTP（internal网络不能发布端口），无真实provider网络代码/凭据。容器、网络和匿名数据卷在退出时按所有权标签清理。源码、二进制、截图及日志在Git忽略的`logs/gin-migration/context-payment-*`内，结束时复核冻结源码指纹。

## 本次证据

[去敏报告](context-payment-subflow-report.json)：六状态的英文390px浏览器与原生SQL通过，浏览器异常0，冻结源码核对和自有资源清理通过。

- 金额不变：丢成功响应后刷新原UUID重放，原CREATE输入版本4/原cart版本5不变，effective版本6；一条保存收据，无DELETE、无额外CREATE。
- 金额变化：仅一次DELETE、保存一条收据、退休付款绑定，刷新不重复删除。
- 恢复原设置：DELETE后价格再次漂移导致保存失败，人工点击恢复并丢恢复成功响应；刷新重试仍只一次替代CREATE，绑定原恢复UUID和输入版本5，无目标保存收据。
- 未知DELETE：刷新重试只做观察，确认取消后完成保存，DELETE始终一次。
- 未知CREATE：刷新恢复仍不重复CREATE，查询确认后接替代付款，CREATE合计2次（首次付款+替代），DELETE一次。
- 已付原会话：DELETE未知后观察已付，保存/恢复均不释放原冻结，不产生目标保存收据或替代CREATE。

六场景均核对库存、预留、订单、实收和通知表指纹不变。相关前端21项局部测试、Go identity包编译/普通测试通过。专用SQL fixture执行并不依赖完整CartRuntime回归。

本次没有运行中英×390/1440矩阵或完整业务链；跨币种/精确小数、替代会话已付款恢复、账户归属与跨设备合并等仍按交付清单继续。本项不宣称C01—C11全部完成。9001/8000/主预览原本无监听，此项不恢复私钥或启动主业务实例。

## 替代会话已付款/仅授权恢复（2026-11-02 后续任务）

未知CREATE经渠道查询确认替代会话已付款或已授权时，旧激活逻辑只接受pending/requires_more，导致恢复始终冲突。现在允许认证过的authorized创建结果接回原购物车的付款绑定；paid与authorized在渠道适配器中均映射为此本地会话状态。它们可能已经没有checkout链接，因此该分支不再要求收银台URL。

恢复只释放本次设置操作的冻结并保留付款绑定，不保存用户取消的目标设置，不将授权当实收，也不依据创建结果直接生成订单。现有付款查询仍须通过服务端渠道读取、金额/币种/购物车/会话校验及加密证据落库，才能区分paid_pending_order与authorized。再次准备付款返回requires_review，不返回旧收银台链接或额外CREATE；订单完成仍由现有独立入口处理。

[本项去敏报告](replacement-payment-recovery-report.json)记录局部浏览器与原生SQL验证：新增两个场景均经历未知CREATE、刷新发现、恢复成功响应丢失及同UUID重放，最终版本6、无目标保存收据、两次CREATE（原始+替代）、一次DELETE。付款查询分别断言paid_pending_order/authorized，再次准备仍无额外CREATE。六个已有场景同时回归；库存、预留、订单、实收、通知指纹全部不变。未在本项执行订单完成或完整业务链，中英/尺寸矩阵及真实渠道沙箱仍待后续验收。

## 同一购物车连续保存与恢复（2026-11-02 后续任务）

`--scenario continuous` 从固定购物车版本4准备原付款，随后始终使用同一持有凭证和同一购物车。两次语言保存均丢成功响应、刷新后沿用原操作重放：版本5→6→7，原CREATE输入4/原cart5不变，独立effective版本分别6、7，没有删除或新建付款。第三次保存时改变测试价格，旧付款取消后再次报价漂移使目标无法提交；显式恢复原设置并丢恢复成功响应，刷新按同编号完成恢复，版本7→8，新CREATE输入7/cart8，原attempt保留。将测试价格恢复为原价后再次保存语言，版本8→9，继续保留替代CREATE与effective9。最终实际付款状态POST返回版本9/pending，未生成订单。

[连续联合报告](context-payment-continuous-report.json)绑定本轮实际二进制与原始报告摘要：中英×390/1440四组通过，无浏览器异常。每组四个独立上下文操作，各操作内的重试和恢复使用相同UUID/原版本/目标；三条目标保存收据、两次CREATE合计、一次DELETE。库存、预留、订单、实收及通知全表指纹不变，冻结源码复核及自有容器/网络/数据卷清理完成。Go identity/cart测试、identity vet与两份脚本语法检查通过；没有改生产业务逻辑，无需整站构建或完整订单链。

脚本现在可重复传入`--scenario`，选中的子场景去重后共用一次构建和同一套独立服务，各场景仍建立自己的购物车，避免每个分项重复初始化。`continuous`刻意不新建中间购物车，证明有效版本与替代付款能够连续衔接。跨币种、后续商品/优惠变化及C01—C11其他交付终点继续开放；完整业务链按用户要求在大任务结束后统一执行。

## 跨币种保存与恢复（2026-11-02 后续任务）

固定基线新增独立加拿大/CAD地区及对应公开价格。原购物车含EUR商品、25%税及配送，总额158.75；目标CAD地区重选商品价158.75，自动税关闭且原配送不再适用，最终总额数字仍为158.75。此构造用于确认币种变化不能被相同数字掩盖，未使用汇率或将原付款换算成新币种。

`currency-save`经原地区抽屉选择Canada，取消旧EUR付款后保存CAD上下文并丢成功响应，刷新同UUID恢复。最终版本6、付款集合CAD/158.75、原EUR会话软删除且无有效会话、checkout退出、清理原CREATE日志、一条保存收据；不会自动发起新币种收款。`currency-restore`在取消后改变目标CAD报价，阻止不一致目标提交；原EUR购物车/金额保持，再显式恢复原付款并丢恢复成功响应，刷新沿用同一编号激活EUR替代会话。最终版本6、集合EUR/158.75、一个有效EUR会话、原CREATE4/cart5保留、替代CREATE5/cart6，无目标保存收据，也未切换页面地区。

[跨币种联合报告](context-payment-currency-report.json)记录中英×390/1440的两个新场景及连续保存回归，共12组实际Astro/Gin/MySQL/Redis浏览器通过。数据库核对集合原始金额/币种和有效会话数；适配器记录证明DELETE收到原EUR/158.75请求，恢复的CREATE也是原EUR/158.75，没有错误币种调用。每组原/替代CREATE及DELETE次数有界，库存/预留/订单/实收/通知指纹不变，源码冻结复核与资源清理完成。14项相关前端测试、Go cart/identity测试、identity vet和脚本语法检查通过。没有新增迁移或修改生产业务逻辑，未调用真实渠道、未进行完整订单链。该证据覆盖两种固定币种的已有付款上下文，不代表所有支付渠道、所有币种精度或C01—C11全部验收；后续商品/优惠变更及正式环境组合仍需继续。
