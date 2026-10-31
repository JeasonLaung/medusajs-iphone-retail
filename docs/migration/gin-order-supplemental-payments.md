# 原订单正差额补款指令

此页记录正差额在线补款的持久指令阶段。0087 默认关闭补款；当前已有内部 Prepare/Read/Cancel、单次创建/GET恢复、paid通知原子消费和独立Worker；生产CLI已通过本地TLS夹具四独立进程恢复验证。当前另已接入顾客会话下的私有准备/查询/取消HTTP及Astro桥接白名单；本人补款上下文、安全checkout链接、双语订单入口、付款回跳与双付款展示现已通过SQL及独立夹具浏览器验证。本人收款站内通知已接入唯一现金事务，当前本人HTTP收件/已读及独立弹窗夹具已验证；补款再退款trace及实际客户/独立CLI连续联合仍待。不能以本阶段关闭 C07/C09 或整站迁移清单。

## 不可变指令与来源

新指令绑定新账号及授权版本、店铺、原订单及当前版本、原已完成 checkout、唯一未付集合、独立新付款会话、完整两位小数金额及币种。UUID 重放需匹配原输入指纹和原加密请求；同一未付集合及订单最多一条未决指令。没有自动启用或按邮箱继承历史订单。

`orderpayments.Principal` 是内部可信参数，不能从浏览器 JSON 解码。当前数据库账号必须存在、启用、已验证且授权版本一致；新账号的 `customer_order_ownership` 是归属来源。顾客协调器现复用已有账号事务：先解析Customer realm的Redis会话，锁定SQL账号，再次解析Redis会话并核对当前密码/启用/已验证/auth_version摘要，最后由服务器读取账号代次并在同一事务调用补款参与者。内部参数仍不是浏览器提供的身份。

原 checkout receipt、原 attempt 与已完成购物车提供静态成单证据；原购物车的匿名能力撤销、代次轮换或软删除不会撤销已附加的新账号订单归属。新补款不重新打开或挂接购物车付款集合，不需要旧购物车凭证。Mollie 创建请求继续携带原 cart_id 作为渠道元数据，同时使用新 session_id 与独立原订单回跳参数。

Prepare 在订单锁内调用当前完整欠款校验，核对原渠道及地区允许的已启用 Mollie provider，拒绝订单级未决退款、其他未决补款、未知历史会话、任何已有付款（包括已软删除付款）及购物车付款集合借用。旧终态会话必须属于本集合的已知终态指令；只改本地状态不足以授权新收费。

完整请求使用现有 payment effect journal 的行绑定加密，scope 为原订单而非已完成购物车；本地会话 data/context 也加密。Prepare 不调用渠道，不创建 payment/capture、现金交易、额外集合或库存事实。

## 顾客私有 HTTP

生产API将默认关闭的ORDER_PAYMENTS配置传入现有customer handler，要求店铺和回跳origin与顾客配置一致；支付keyring与顾客keyring各自使用，不混用。沿用现有同源检查、桥接签名、Customer HttpOnly cookie、CSRF、private/no-store和no-referrer。新增接口不调用Run或渠道，worker负责发送和恢复。

- `POST /api/store/customer/orders/:id/payments`：严格字符串JSON字段operation_id、expected_version、collection_id、amount、currency_code及可选previous_operation_id；订单只取路径，不接收account/auth_version/session/provider或浏览器paid字段。expected_version必须为规范正整数字符串；202只表示本地指令已持久化。
- `GET /api/store/customer/orders/:id/payments/:operation_id`：当前合法本人会话读取最小本地回执；不返回远端ID、支付session、加密信封或购物车身份；checkout_url仅为经保护创建回执且当前版本/完整欠款仍一致的安全Mollie HTTPS链接，否则为null，也不查询渠道。账号代次更新后需重新登录。
- `POST /api/store/customer/orders/:id/payments/:operation_id/cancel`：正文只含独立取消operation_id，原补款UUID取路径；与当前会话复核、会话/指令状态及审计回执同一SQL事务。关闭SQL补款gate仍可查询/取消未发送指令。

新增 `GET /api/store/customer/orders/:id/payments` 返回本人当前版本、币种、精确欠款、历史回执及可选next同意快照；next不是收费授权，准备及首次POST仍重新验证。next只在开关、金额、集合、前驱和退款/付款占用均允许时出现。

Astro customerBridge同步放行这四种规范路径，付款UUID须为规范v4；只转发当前Customer cookie，旧cart、Admin及Authorization不进入请求。准备只接受202，查询/取消只接受200，拒绝浏览器run或provider动作路径和全部查询参数。沿用现有签名、受限响应及cookie处理。12项实际Node桥接/签名测试及Astro check通过，随后新增的页面和回跳验收见下文。

不存在或不属于本人订单/指令均返回404，撤销/禁用/未验证/旧代次会话返回401，CSRF/Origin失败拒绝；金额/版本等状态变更返回409，依赖或写入失败返回503。私有组拒绝这些路径的全部查询参数。PrepareTx、ReadTx、CancelTx均保留在调用者事务中，不用另一个池绕过账号锁。

新HTTP测试给人工SQL新账号设置测试密码，通过实际customer.Login取得Redis会话；账号建立/verified和归属仍为SQL夹具，不宣称真实注册、验证邮件或浏览器成单。六个HTTP prepare并发返回同一指令；真实SQL账号锁等待期间撤销Redis会话会被第二次解析拒绝。另覆盖Admin realm/跨账号、严格JSON、Origin/CSRF、禁用/未验证/旧代次、取消末尾SQL触发器整事务回滚和关闭SQLgate后的读取/取消重放。report-order-payment-customer-http.py从完整23组SQL和完整源码指纹派生专门报告。

## 明确取消及新尝试

只允许取消本账号仍拥有订单的未发送 pending 指令，并要求原 effect 仍 pending、尚未认领。取消会同时更新本地会话/指令并写入独立 UUID 的取消回执。开关关闭时仍允许当前合法账号读取和取消未发送指令；账号撤销不授予取消权限。已认领、已发送或不确定结果必须走后续查询恢复，不能本地取消后重新收费。

失败/取消/过期后的新尝试需要新 UUID 和明确前驱；每个前驱只能有一个后继，历史输入和会话不覆盖。新尝试重新核对当前账号授权版本及当前欠款。已验收本地取消及创建适配器明确无远端效果的 failed 前驱链。权威GET也能消费 failed/canceled/expired；这些终态后的完整客户新尝试联合仍待，不能凭本地会话状态推断远端终态。

## 版本与退款互斥

待付、外部不确定、授权中及已 paid 但尚未本地应用的指令持续占用订单金额版本；即使关闭补款开关，也不能通过原生 AdvanceVersionTx 绕过占用。未发送取消释放该占用。后续 paid 应用必须在唯一现金事务成功后释放占用。

退款运行时升级到 v4；新退款及首次 POST 同样检查补款占用。补款 Prepare 检查订单级未决退款。已发送退款继续原有观察/本地应用恢复，不因新增占用变成第二次 POST。

0087 将原生订单运行时提升到 v2，旧订单写入/导入代码必须拒绝执行；历史 source schema plan 及 source model 仍为 v1，不重写0016或源证据。当前历史 order importer 检查 native v2。退款 v4 保留原 enabled 值，不自动开启。应用启动不执行 DDL；本批仅向自有隔离回归库应用迁移，没有改动原服务或其他本机数据组合。

## 单次创建与恢复

Run只接受普通数据库池，在真实提交effect claim及dispatched_at之后才POST。首次发送重新核对当前账号/授权代次/已验证、明确归属、原成单trace、当前版本/精确欠款、地区启用provider及原加密待付会话。任何丢响应、panic、取消或未知结果只走GET discovery；通用effect的failed可重试语义不能授权同一补款UUID再次POST。明确无远端效果的创建失败，才以同事务failed/error释放占用，顾客须明确前驱创建新UUID。

已发送恢复不依赖当前账号、旧cart匿名能力或旧cart当前attempt。授权后的远端结果即使请求断开仍以WithoutCancel短事务保存。RecordSessionCreation和Mollie reference同事务；晚期reference失败回滚整份创建结果。创建结果先提交，再单独enqueue通知，因此关闭通知接收不能丢掉已经取得的创建证据；恢复时读取已保存结果，不必再次访问渠道。创建响应或discovery中的paid仍不记现金。

## 权威paid的唯一现金消费

Consume只接受私有通知attempt UUID，从原Mollie worker的加密GET观察取得事实，复核完整order/op/session/request digest、远端identity/金额/币种及保护创建回执。authorized不算实收；旧open不回退authorized，任何迟到未付观察不回退已paid/applied。

权威paid以同一事务产生确定ID的payment、capture、原订单正交易，完成原未付集合并将会话标captured，重算当前汇总、生成唯一本人站内通知、设置applied并确认渠道通知消费。任何末尾失败都回滚全部本地现金，保留未消费的加密paid证据供无网络重试；不新建订单、重开cart或改库存/设备。原生CashSummary独立于可变订单命令资格，取消订单仍能记入已收到现金，不会把订单改回pending；原金额变更/退款入口继续原有取消限制。

若版本、精确欠款、集合来源或原始金额被带外修改，保留paid证据待核对，不私自改指令金额或抹平差额。已软删除订单的创建/恢复证据仍保存；当前金额投影不能读取软删除订单时，现金消费继续待人工恢复核对，不自动取消软删除。终态后出现矛盾paid也保留证据，不能静默覆盖明确后继。

## 独立worker

process-order-payments需明确ORDER_PAYMENTS_ENABLED=true、原checkout/keyring/商户配置、MOLLIE_NOTIFICATIONS_ENABLED=true及已应用完整迁移；默认false。API启动不执行DDL、不打开SQLgate，也不启动worker。无参数持续运行，--once执行一批后退出；日志仅固定说明和工作计数，不打印商户/付款数据。当前已验证配置拒绝边界及四次独立CLI进程恢复；常驻生命周期尚未实际验收。

Worker分别有界扫描10条创建/已保存结果、10条专属订单通知GET、10条未消费观察；未知创建每进程每分钟至多一次discovery，并有独立轮转游标，坏记录不能永远挡住下一笔。普通checkout/其他店铺的引用不能占用补款批次。MySQL拥有claim、lease、不可变证据与唯一现金；内存游标和节流不授予POST。六个独立Worker实例的并发创建与消费、空任务和跨店过滤已经过真实SQL验证。关闭补款SQLgate阻止首次发送，已发送的查询和记账仍保留。

## SQL 验收边界

测试沿用实际原生购物车付款成单，再在可回滚事务中调用原生换货/收货，得到原实收158.75、当前164.75、应补6.00。账号及订单附加归属是明确的人工 SQL 夹具，不是实际注册/登录或浏览器验收。

校验原 UUID 的六次顺序重放、不可改金额/币种/版本/集合、其他账号/店铺拒绝、账号禁用/未验证/授权代次变更拒绝、当前新代次能读/取消旧未发送指令、唯一未决指令、明确前驱及禁止分叉。实际 SQL 触发器分别在 effect/session 已写后和取消状态已写后拒绝最终 INSERT，整条指令及取消都完整回滚；另验证外层调用者回滚。

占用测试核对关闭开关仍阻止版本推进、成功取消后可以推进、已认领 effect 不能取消、补款和退款双向互斥。另撤销并轮换旧购物车能力、软删除原购物车与历史 cart link，当前本人仍可新建/读取/取消补款。最终仍只有原一笔现金付款和原购物车付款集合，原购物车保持 completed。

`report-order-payments.py` 要求23组完整 SQL、唯一指令标记和资源清理回执，生成带源码/日志指纹的独立报告。它明确标记：未做并发补款验收、真实顾客会话、渠道调用、现金消费或浏览器/CLI。现有四新客户负差额退款联合仅用于回归原退款业务，不代表正差额补款已经可用。

## 持续交付边界

新增SQL执行测试另行完成一笔真实原生checkout，再提交真实原生换货/收货形成6.00差额；新账号ownership、设备收回事实及商户是人工夹具。创建并发、租约期间不POST/不discovery、首次权限/金额/开关/provider复核、丢响应GET、创建末尾回滚及关闭inbox仍保存回执均通过。消费验证实际notification claim/GET/encryption路径、六并发、授权不收款、信封损坏/跨店/带外金额拒绝、现金末尾及通知ACK末尾回滚、无网络重应用、撤销账号/旧cart退役/订单取消后实收、迟到观察及金额版本释放。不把这些SQL事实报告为实际顾客注册登录、生产CLI、浏览器补款或真实Mollie。

report-order-payment-runtime.py从完整23组SQL、唯一执行/现金标记与清理回执派生新报告，并拒绝测试后源码/迁移漂移。原order-payment-commands-report记录只prepare/cancel的独立回滚场景；当前runtime报告记录新的提交/入账测试，两者验收边界不同。

下一步继续四新客户正差额、本人付款通知至替换出库的实际浏览器/独立CLI连续联合。补款payment后续退款必须扩展可信trace及新支付数据的order-payment-v1保护用途，不能只认原checkout receipt.payment_id。真实渠道/邮件、常驻进程及其余C01–C11继续开放。

## 本人订单页面与只读付款回跳

本人订单详情在原付款/配送/商品区保持原结构，追加补款区，使用现有主题、金额格式和按钮/Tag/反馈组件，支持中文/英文和手机/桌面。完整欠款与金额/币种/集合/版本/明确前驱取自本人上下文；点击确认才用新UUID准备该完整金额。请求丢失或服务端结果不明时保留同一个UUID及正文，只重试同一请求；页面重载从本人完整历史恢复活动指令，活动指令没有第二份next同意。明确409后仅在成功重读上下文时重新展示当前可同意金额。取消请求也保留独立UUID，不调用远端DELETE。

创建/不确定/授权/paid未应用均显示准备或核对状态，只有applied=true才显示“补款已收妥”。页面只GET轮询已存在活动指令，后台仍负责创建、观察及现金事务。当前原付款158.75和补款6.00分别展示，当前订单164.75与现金入账后待补0保持区分；原付款不改写成164.75。普通无差额且无历史订单保持原布局，未配置/不适用补款功能不会新增错误块；需要补款的订单保留明确失败及重试反馈。

checkout_url只从同一支付effect的加密成功回执解出，复核请求/远端identity、金额币种和metadata，再限制为无账号信息、无fragment、长度受限且443端口的Mollie HTTPS域名。当前订单版本、唯一未付集合或完整欠款变化即不再提供链接；授权/终态/已应用也不提供链接。读取不POST、不记账、不以创建返回的paid判定成功。

原 `/api/mollie-return` 保持购物车路径；只有带order_id/order_payment_id的补款回跳转入新分支，严格核对恰好三个规范参数。原cart_id仅作为格式校验的历史元数据，不解析其bearer cookie，不调用cart或重新checkout。当前Customer会话只读本人付款回执及订单，按保存的locale/配送国家303至原本人订单详情补款区域。跨站渠道导航预期存在，但不把其Origin当同源写入；后续本人读取仍走服务器签名桥接。撤销/非本人/坏参数拒绝，浏览器status=paid等额外字段不能改变付款事实。

新增SQL定向和完整23组回归覆盖当前6.00同意、活动指令无第二同意、事务内人工保护创建回执解出的URL、旧版本隐藏链接、损坏信封拒绝，以及唯一现金后当前代次可读三份历史（取消/失败/已付款）、零欠款及不再提供链接。URL夹具事务回滚，没有额外provider POST。独立正式Astro构建+签名桥接的本地API/渠道浏览器夹具覆盖en/zh×390/1440、丢响应同UUID、刷新页面恢复、取消后明确前驱、安全结账链接、无cart bearer的只读回跳、paid未应用仅核对中、应用后显示收妥、原158.75和新6.00分别展示、无重读循环/溢出。原购物车支付/稳定回跳与普通本人订单浏览器回归继续通过，截图人工核对。

`report-order-payment-presentation.py` 绑定同版完整SQL源码和独立前端源码/构建/24项Node测试/新旧浏览器记录；主动拒绝测试后源码漂移。该报告明确区分两组证据：SQL使用专有真实MySQL/Redis和人工账号归属/设备/渠道；页面使用本地API/渠道夹具。尚未把四个实际注册客户、原生换机、独立CLI、收款通知、补款后替换出库接成连续联合，不能用本批完成C07/C09或C01–C11。真实渠道/邮件、常驻worker及补款再退款trace继续开放。


### 独立生产 CLI 的进程恢复证据（2026-10-31）

`ORDER_PAYMENT_CLI_REHEARSAL=1 python3 retail/scripts/test-admin-auth.py --cart-runtime-only` 现在会单独编译 Linux `cmd/process-order-payments`，与测试 runner 一起放入无外网的自有 MySQL/Redis 容器。四次 `--once` 是四个独立进程，使用显式人工配置和专有 CA；HTTPS CONNECT 夹具只允许 `api.mollie.com:443`，没有调用真实商户。测试沿另一个已提交原生 checkout/正差额换货订单的精确 6.00 指令执行，不重开旧购物车。

第一进程在持久 claim 后仅一次 POST，远端夹具保存付款后断开响应，并拒绝该进程的恢复 GET，进程失败退出且保留 uncertain。第二进程只 GET 列表及独立付款，再取得加密通知证据；通知最后 ACK 的 SQL 故障使 payment/capture/正交易/汇总/applied 全部回滚，保护创建与 paid 证据保留。随后撤销账号、退役 cart、关闭新发送 gate 并取消订单，第三进程在禁止网络的夹具状态下本地重应用：总实收164.75、欠款0，各现金行和 ACK 只一份，订单仍取消。第四进程是空批次，没有额外 GET/POST。正式 CLI 日志不输出测试密钥、远端付款标识或指令 UUID。

报告 `order-payment-cli-report.json` 绑定生产二进制摘要、测试前冻结/结束复核的后端源码指纹、CLI SQL 记录和完整23组 SQL 记录。该验收证明独立生产进程恢复，不等于四实际新客户浏览器/CLI正差额连续联合；账号归属、商品、设备收回和 provider 仍含人工夹具。该轮未验收本人付款通知；本轮新增通知证据见下文。补款再退款 trace、实际新客户至替换出库的联合、真实渠道/邮件及常驻 worker 生命周期继续未完成。上一轮浏览器夹具证据与本轮 CLI 证据不能拼接成已经通过的连续链。


### 本人补款站内通知与现金同事务（2026-10-31）

显式0093扩展既有customer_account_notices.kind约束，保留所有原通知类型，不做启动DDL。收到并验证paid后，以确定的本地payment ID生成`payment.received:pay_supp_<operation UUID>`事件；订单号为reference、原订单为resource_id、action为空，不包含金额、远端付款/会话ID或支付载荷。复用现有明确customer_order_ownership和已启用/已验证新账号收件规则，不能以旧订单customer_id或匹配email授予通知。缺少合格收件人时不阻断已收到现金的恢复，也不在以后补发或自动绑定。

事件与payment/capture/正交易/集合/汇总/applied及渠道通知ACK同事务。专有SQL注入通知INSERT末尾失败及最终ACK失败，验证现金与站内通知一起回滚，保留加密paid证据供本地重试。六并发消费者只一份事件，迟到open不重复通知。禁用/未验证/无明确归属均不收件；当前代次真实Login+Redis会话可通过既有私有HTTP列表读取并CSRF标记已读，其他账号不能读取该事件或修改其已读状态，重复已读保持原时间，已撤销会话拒绝。

商城复用原通知铃铛/弹窗、排版、分页、未读角标、已读/重试与订单详情导航，仅新增英文“Your supplemental payment has been recorded”和中文“您的补款已记账”。无颜色、字体、布局或交互重做。浏览器以明确合成API验证中英、移动390、原分页/单条及全部已读/错误保留/重试和规范本人订单详情跳转，浏览器错误0；这不是实际Gin/SQL付款通知浏览器联合。新报告`order-payment-notice-report.json`分别绑定完整SQL、禁用收件人四独立CLI恢复和弹窗夹具，不把独立证据拼接为四新客户连续链。

0093序号避开工作区并行商品迁移0088–0092；本批隔离基线是已提交0001–0087加0093，共88个显式SQL文件，并行商品迁移不在本批验收/提交范围。相应测试计数按该隔离基线更新；工作区含并行迁移时的总计数另外保留。下一项仍是实际新客户正差额6.00至本人通知和替换出库连续联合，以及补款实收再退款可信trace。


### 四新客户正差额补款至替换出库连续联合（2026-10-31）

新增独立 `CHECKOUT_EXCHANGE_PAYMENT=1` 联合模式，沿用购物注册/验证/登录、商品加购、地址物流、原158.75付款成单及明确本人归属、Admin配机与原发货、客户换机请求、实际设备扫码收回和原生替换。仅在专有夹具把替换适用税率从25调整30，使同一订单原生形成当前164.75和6.00欠款；每个场景结束恢复25，后一个客户的原付款仍为158.75。没有直接插入订单、指令、cash、ownership、通知或替换出库事实。

替换欠款时先核对真实打包context的paid=false、设备选择及打包按钮禁用，再用当前revision/实际替换设备提交打包并要求409。随后同一实际Customer账号在本人订单页面明确同意完整6.00，实际签名HTTP准备指令；丢弃已成功202响应后重试同一UUID/正文，刷新只恢复原指令。顾客旧cart cookie已清除。独立编译的生产`process-order-payments --once`由专有runner分进程调用，使用本地CA/CONNECT渠道夹具；远端保存首份open付款后丢弃响应并禁止该进程发现，新进程仅GET找回受保护结账链接。其余三笔同样各唯一POST。

只替换外部Mollie hosted页面，不替换商城/管理业务响应。顾客在该页完成夹具付款后，生产webhook处理器接收实际form并持久排入已知reference的inbox；独立CLI进行权威GET和唯一paid现金消费。只读同源回跳沿实际顾客会话返回保存locale的本人订单锚点，不POST或依赖旧cart。页面仅在applied后显示补款收妥，保留原158.75和当前164.75；核对真实本人payment.received事件后继续原打包、仓库发货、原设备拍照检查和售后完成。最终每单五份本人通知均已读，原与替换追踪保留。

运行：在不承载开发服务的隔离检出中执行 `CHECKOUT_PRODUCT_BROWSE=1 CHECKOUT_FRESH_CART=1 CHECKOUT_ACCOUNTS=1 CHECKOUT_FULFILLMENT=1 CHECKOUT_EXCHANGE=1 CHECKOUT_EXCHANGE_PAYMENT=1 python3 retail/scripts/test-customer-orders-live.py --checkout`。报告为`shopping-exchange-payment-joint-report.json`，编译前冻结后端/迁移与Admin/Astro源码指纹，结束再次核对，另记真实生产CLI二进制摘要；源码漂移拒绝生成验收证据。中英×390/1440使用四个不同实际注册账号和四份各自原订单，不拼接旧UI/CLI报告。独立数据库/Redis、库存和人工渠道/验证邮件意图均为自有测试资源，清理完成才写报告。

此模式验证本地连续业务链，不代表真实商户付款/邮件送达、常驻worker生命周期或全商城交付。`order-payment-v1`实收再退款可信trace及C01–C11其余项仍待完成；历史notice/CLI/presentation报告中的未联合标志保留其当时范围，不能改写成该轮已验收。

最终88文件快照：正差额四客户联合162.49秒、原负差额退款联合200.37秒，均浏览器错误0并清理完资源；完整23组SQL、Go全包及相关vet通过。正差额报告1232份、负差额报告150份、完整SQL442份源码指纹逐项匹配；只验收隔离的0001–0087加0093，不包含并行商品/评价迁移。
