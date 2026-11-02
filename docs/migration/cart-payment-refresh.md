# 购物车支付刷新决策与准备

2026-10-29：新增`cart.PlanPaymentRefresh`及`PreparePaymentRefresh`，承接原refresh-payment-collection工作流的决策和私有准备阶段。它们尚不执行支付渠道调用、删除本地session或更新集合，不能将返回计划当作刷新完成。

原流程的shouldExecute取决于输入：提供cart对象时看是否有payment_collection，否则看cart_id。金额使用原raw精确比较；金额相同则只判断币种，金额不同则使用shouldExecute。需要变更且集合存在时，准备更新原集合的amount/currency并删除其有效session；不创建缺失集合。Plan实现这几个分支，保留有效会话顺序，验证ID、币种、非负精确金额及最多100会话，错误不返回部分计划。

`capture-cart-payment-refresh.cjs`从当前安装的原工作流中提取shouldExecute、should-update及两个准备transform回调，以原MathBN对15组人工输入执行：有cart/只有cart_id/无输入、无集合、相同金额、等价scale/exponent、币种变化、超出scalar小数位的差异、大数精确相等、零总额、无session等。该脚本不运行工作流、不连接应用或支付渠道；Go纯测试逐组比较结果，另测非法金额/币种/重复或超限session。

Prepare在调用方事务重查当前cart授权/version、payment schema，读取税/运费/促销后的实际金额。按固定顺序锁定cart_payment_collections、有效payment_collections及payment_sessions。链接或集合软删不参与，多有效集合拒绝，超过1000链接/100会话拒绝，不截断。scalar/raw成对解析，坏raw不回退；输入固定采用已获取的cart分支。返回目标集合ID、准确新金额/币种、需要处理的旧session IDs及决策。数据仅供内部支付协调，不可直接返回浏览器或记录日志。

## 验证

原15组回调对照及Go非法输入测试通过。隔离CartRuntime人工MySQL验证无集合不创建、币种/精确raw变化生成计划、等价金额不使session失效、有效session保留而软删session排除、多集合/坏raw/schema关闭拒绝。调用后原集合币种不变，后续同一事务仍能读到未删除session；最终整体回滚核对购物车/库存原指纹。autocommit拒绝，授权/version由既有LockForWrite执行。

cart-runtime及全量报告cart_promotion_amounts增加payment_refresh_preparation_without_mutation，记录实现、原对照采集脚本及fixture SHA。fixture需原脚本--check复核，报告SHA必须同时匹配。

## 后续必须实现

原delete-payment-sessions实际调用provider.deleteSession，删除失败会验证结果并尝试重建已经删除的会话；不能用SQL软删代替外部会话失效。后续需要显式的持久操作状态、provider适配和幂等/重试/补偿，在本地数据与外部副作用间保持可恢复状态，并在最终金额/币种一致后完成购物车命令。尚未启用任何新支付provider调用，也未声明支付刷新或checkout完成。T032/T033未验收。

## 支付副作用的持久执行记录（0033）

新增私有 `payments.StageEffect / ClaimEffect / RecordEffect` 与 `payment_effect_journal`，保存 scope/operation/动作/资源、不可变请求 SHA256、当前尝试 UUID、次数、租约和状态。支持 delete_session 与补偿用 create_session，二者独立绑定。请求摘要必须涵盖实际 provider、完整出站参数、源快照和创建幂等键；记录不存令牌或渠道 payload，也不授予购物车或支付权限。所有入口要求调用方真实事务，重查支付 schema，任何错误须回滚。

Stage 相同输入重放，变更摘要拒绝。Claim 在 MySQL 行锁内一次授予一个尝试，必须提交成功后才可在事务外调用 provider；租约使用数据库 UTC 时钟，10秒至5分钟。只有 pending 或明确未产生副作用的 failed 可抢占，最多100次。inflight 到期只转 uncertain，禁止再次派发。超时/断连/取消或渠道错误含义不清时必须记录 uncertain，不能当作 failed。未知状态由原尝试的迟到确定响应或渠道核实解决；当前尝试 UUID 防止旧响应覆盖重试。成功重放不重复派发，成功不能改为失败。

隔离 MySQL 验证8个独立事务争抢只授予1次派发、请求摘要变更拒绝、明确失败重试与旧尝试隔离、结果未知不重试、过期租约保持原尝试、迟到确定响应解决、成功重放、调用方回滚、创建/删除记录独立及 schema/事务约束。报告新增 payment_effects。33个版本化迁移及报告 SHA 是本轮证据。

该表只保存当前尝试与累计次数，不是完整尝试审计日志，也不是可独立执行的任务队列。尚未接入渠道适配器或外部调用；协调器还必须在 Stage 的同一事务持久保存受保护的恢复快照、绑定当前授权/cart版本，并完成 provider 幂等/核实、部分删除补偿、集合更新和最终提交。仅有 receipt 或 Dispatch=true 不能认定支付刷新完成；T032/T033仍未验收。

本轮验收证据：`go test ./...`通过；隔离数据库完整23组回归通过，33迁移及成功重放均验证；admin-auth-report的267项实现SHA匹配。旧9001 health/Admin、8000商品页及购物车页均HTTP 200，近期服务日志无新增错误（既有PayPal seller nonce配置提示保留）。这些证据不包含真实provider或checkout验收。

## 加密恢复请求快照（0034）

新增可空 request_envelope LONGBLOB，允许旧0033低层 receipt 保留无快照状态，但无快照不能通过新API获得派发。`StageSessionRequest`在同一调用方事务保存完整provider/collection/local session/精确amount/currency/data/context和receipt；Digest由版本化规范编码自动计算，已有非零Digest必须匹配。data为JSON对象，context允许null或对象；保留JSON原始大整数和小数、对象顺序，压缩空白，不用float解码；所有层级重复key、无效UTF8、超64层及超1MiB整体快照拒绝。金额复用精确totals，无SQL量化。

复用protected AES-GCM Ring和独立业务密钥。认证上下文绑定purpose/table/column及scope/operation/kind/resource/request SHA的无歧义哈希；跨操作或资源替换密文拒绝。新私有`ReadSessionRequest`需真实事务、支付schema及journal行锁，解密、摘要与规范版本逐项核验。`ClaimSessionRequest`先完成同样核验，再抢占派发；密钥不可用、密文损坏、缺快照不推进attempt。未知结果可恢复原请求用于核实，不授予重试。密钥轮换保留旧解密key，重放保留原密文，不自动改写或修补。

隔离MySQL用人工token验证密文中无明文、精确金额/JSON大数恢复、新事务重启读取、密钥轮换重放、缺key/摘要变更/篡改/跨操作替换拒绝、未知结果读取而不派发、低层无快照receipt拒绝，以及触发器模拟快照写入失败与后续命令失败均整体回滚。报告增加payment_requests；纯Go测试补JSON嵌套重复key/边界/非法输入。

这一步完成的是单个渠道动作的恢复材料，不是完整支付协调器。服务端仍须绑定当前cart授权、generation/version与集合刷新目标，持久任务顺序及补偿状态，接入真实适配器、创建响应保存/恢复、部分删除补偿和最终集合更新。SessionID为本地稳定请求标识，创建适配器须使用它作为幂等键及data.session_id，禁止用每次变化的attempt UUID。未导入旧登录态、未调用真实渠道，T032/T033未验收。

0034验证结果：`go test ./...`与隔离MySQL/Redis完整23组回归通过；34迁移成功执行，admin-auth-report的273项实现SHA匹配。旧health/Admin/商品/购物车入口均200，服务日志仍只有既有PayPal seller nonce提示。请求恢复为人工fixture验证，不包含真实渠道调用或完整checkout。

## 购物车支付刷新持久协调（0035）

`cart.StagePaymentRefresh`在调用方真实事务中重查cart.access及独立默认关闭的cart_payment_refresh_runtime开关、当前store/channel/generation/version，复用PreparePaymentRefresh取得最终商品/税/运费/优惠后的金额。它锁定有效集合和至多100个session，从新运行时认证密文读取data/context，自动生成0034恢复请求，保存原计划、集合/session源指纹、effect keys及manifest SHA，并将operation UUID绑定cart_ownership。缺集合或金额/币种未变时只记录completed no-op，不冻结、不增加版本。阶段错误必须整体回滚；manifest插入失败不会留下已准备的效果记录。

冻结经过所有现有lockCurrent UPDATE及LockForWrite入口，阻止另一个操作、商品增加/数量变更、重定价、税/运费/优惠写入。SHARE检查及金额/页面只读保持可用。冻结没有自动过期：进程中断或provider结果未知时，不能仅凭时间恢复写入。只有同一个私有支付operation能再次取得写锁，且每次都校验保存的generation/version、目标计划、源指纹和冻结指针。

新增payments.SealSessionFields/OpenSessionFields，只处理新运行时session data/context，AAD绑定session/provider/collection/精确amount/currency及具体列。保留SQL NULL context，不接受明文或历史导入purpose作为运行时替代。Stage不从浏览器接收provider参数，也不因同邮箱或历史ID授予归属。加载密文前限制单字段2MiB、单操作合计8MiB，再验证单个请求1MiB上限，不截断。集合已完成/已授权/已扣款、session已授权/扣款、有authorized_at或有效payments记录均拒绝刷新；远端状态仍需真实adapter核实。

`ClaimPaymentRefreshSession`绑定指定有效session及保存请求，重查后授予commit后才有效的dispatch；`RecordPaymentRefreshSession`绑定当前operation及attempt后保存经adapter核实的结果。两者不自行调用渠道。只有每个delete effect均succeeded，`CompletePaymentRefresh`才能在同一事务内更新集合精确raw金额/币种、软删原有效session、将cart版本加1、解除冻结并完成操作。任何一步失败整体回滚；provider成功记录本身保留，用于重试本地完成。completed重放不再次改价/删除/加版本，仍检查当前归属和能力。

隔离MySQL人工流程串联：新匿名cart→加密运行时session→stage→两次claim→人工明确结果登记→complete。验证默认关闭、无集合no-op、已付/有效payment拒绝、manifest失败无孤儿receipt、正常写入和另一个operation冲突、读可用、generation/version重查、源/目标/冻结指针漂移拒绝、部分成功及未知结果不完成、不盲重试、attempt匹配、完成最后一步写故障全部回滚、成功金额/session/版本原子变更、重放及撤销归属拒绝。报告新增cart_payment_operations；人工结果登记没有模拟真实支付扣款或调用真实渠道。

当前提供可恢复的内部协调基础，未接入HTTP、worker或真实provider，部分失败补偿/中止解除冻结尚未实现，创建provider响应及重建session也未完成。因此不得开启此能力用于真实购物；生产能力默认关闭，T032/T033及checkout仍未验收。

0035验收证据：`go test ./...`与隔离MySQL/Redis完整23组回归通过，35个迁移均验证。完整报告278项/购物车专项175项SHA匹配；人工provider结果登记仅证明本地协调，不证明真实渠道完成。旧health/Admin/商品/购物车页面均200，日志无新增错误，既有PayPal nonce提示保留。

## 事务外渠道动作执行器

新增 `cart.ExecutePaymentRefreshSession`，仅接受普通DB pool，拒绝外层事务和空适配器配置。在第一个自有事务调用Stage、校验冻结/来源/授权、读取加密请求并认领指定session；服务端allowlist缺provider则整体回滚，不能消耗attempt或触发外部调用。只有claim事务提交成功并且Dispatch=true才调用adapter。外部调用不持有购物车/SQL事务锁，context限20秒，adapter自身也必须为HTTP设置硬超时；不创建可能残留调用的脱离goroutine。

`payments.DeleteAdapters`复制服务端allowlist，新增DeleteSessionAdapter契约，无任何真实渠道默认注册。adapter要核实远端session身份/状态，禁止删除已付session，只能在明确无副作用时返回failed；所有非nil错误、不认识的状态和panic一律变为uncertain，原始错误不记录/返回，避免token等敏感内容泄漏。正常调用不使用attempt UUID当远端幂等键。结果返回的Dispatch永远false，不能拿执行结果再派发。

第二个独立事务记录已经授权的精确attempt结果。用户取消、源数据或归属在调用期间变化，不应丢弃外部事实；用WithoutCancel加5秒保存超时，仅脱离取消保存该事实，不授予新购物车权限。保存失败时返回Called=true及原inflight记录，不谎报失败或自动重试。后续执行看见inflight/uncertain/succeeded不会再调用；失效租约只转uncertain，待渠道核实同attempt后再完成本地刷新。若在调用前已经取消且确定未进入adapter，则记录明确failed。

隔离SQL人工adapter验证：调用时另开事务成功取得同cart锁且读到已提交inflight；两个执行器重叠只调用1次；取消后保存成功事实；调用期间撤销generation仍保存事实，但旧归属不能再次调用/完成；错误内容不泄漏且结果未知不重试；明确失败复用原请求重试；claim写故障不调用；provider成功但结果写故障保持inflight、租约到期uncertain、不重复调用，核实结果后可完成；外层事务和未配置provider拒绝。报告新增cart_payment_execution，pure Go测试覆盖panic/非法结果/错误分类及allowlist复制。

该执行器当前仅在人工adapter测试中使用，未接入worker/HTTP或真实支付渠道；没有实现部分失败补偿、创建响应持久化或session重建。T032/T033及checkout依然未验收，0035开关默认关闭。

执行器验收证据：`go test ./...`、隔离MySQL/Redis完整23组回归通过，完整报告282项源码SHA匹配（35迁移保持）。旧health/Admin/商品/购物车页面均200，近期日志无新增错误，仅既有PayPal seller nonce提示。人工适配器证据不能代替真实渠道沙箱/补偿/checkout验收。

## 支付刷新部分失败的持久补偿（0036）

新增cart_payment_refresh_compensations及加密result_envelope。`StagePaymentCompensation`选择不可逆的补偿分支：重新核对当前归属/generation/version、冻结、原计划和源指纹，锁内检查全部删除效果；inflight/uncertain必须先核实，不能一边重试删除一边补偿。只为succeeded删除分配新本地session ID并原子保存create请求；pending/failed原session保留。新ID、data.session_id、context.idempotency_key及已有mollie_attempt绑定同一个新稳定ID，重试不变，JSON大数不经float。开启补偿后，原删除claim/execute/正常complete均拒绝。读取补偿选择和有效payments使用锁定当前读，避免RR旧快照漏掉已选择分支或已付事实。

`payments.RecordSessionCreation`将经过adapter核实的status/data与精确attempt加密保存，并在同一事务将create receipt标为succeeded。重放核验原密文及同一结果，不覆盖改变后的响应，不修补坏密文；缺key/坏密文/错误attempt拒绝。`ReadSessionCreation`要求succeeded、认证结果及同一个attempt，不能仅凭低层success receipt激活session。可保存authorized/captured等远端事实，但它们不能作为普通未付替代session被补偿完成逻辑激活，需后续人工/渠道核实处理。

`ClaimPaymentCompensationSession`只读保存请求后按原租约规则认领create；不执行真实创建。`CompletePaymentCompensation`仅在全部create都有认证的未付结果后，原子插入加密的新session、软删对应已删除原session、递增cart版本、完成原操作及补偿操作并释放冻结。新session合并原data和渠道返回data，保存新local session_id；原集合金额/币种不变，其他原session不变。最后写失败整体回滚，外部成功结果保留供下一次本地完成。零已删除效果可不建session直接完成取消；补偿成功重放不重复插入或增加版本，仍检查当前归属。

隔离人工MySQL串联三个原session：成功删除A、未知B先拒绝补偿，核实B失败后仅重建A，C未调用保持。验证阶段失败无孤儿create、稳定ID/幂等字段、原流程被阻止、create attempt校验、加密响应与success原子、缺key/变更结果/篡改拒绝、已授权结果不激活、最终写故障完整回滚、新session精确响应恢复、原集合和B/C保持、版本/解除冻结原子、成功重放/撤销归属拒绝，以及零效果取消。报告新增cart_payment_compensation。

本轮接通的是私有持久补偿事务与返回结果保存；尚缺create adapter执行器、真实渠道适配/核实、worker/HTTP入口和完整购物车命令回滚协调。没有调用真实支付渠道；0035能力默认关闭，T032/T033和checkout尚未验收。

独占关系校验：原导入cart_payment_collections只有复合主键，并不保证一个collection只关联一个cart。所有有副作用的刷新/补偿在锁定当前读中要求恰好一个有效cart链接且等于当前授权cart；共享集合拒绝，其他已软删链接不阻挡。人工SQL验证共享集合同时拒绝删除claim和补偿，软删另一链接后才可继续。该保护防止持有一个cart被误当作共享会话的完整操作权限。

0036最终验收：`go test ./...`与隔离MySQL/Redis完整23组回归通过，36迁移验证成功；完整报告286项/专项183项实现SHA匹配，包含共享集合保护。旧health/Admin/商品/购物车页面均HTTP 200，近期日志无新增错误，仅既有PayPal nonce提示。本轮没有真实渠道调用或完整checkout验收。

## 补偿创建会话执行器

新增 `payments.CreateSessionAdapter/CreateAdapters/InvokeCreate` 与 `cart.ExecutePaymentCompensationSession`。服务端allowlist复制配置，不默认注册真实渠道；adapter必须设置HTTP硬超时，核实远端身份/金额/币种并复用持久request.SessionID作为幂等键，不能使用每次改变的attempt UUID或修改本地SQL。明确failed必须没有返回会话且能证明无外部创建；非nil错误、panic、未知状态、非法status/data及矛盾失败结果统一uncertain，原始错误/未验证结果不返回或记录。

执行器只接受普通DB pool。在首个自有事务重查补偿归属/版本/冻结/源计划，认领保存create请求，缺配置整体回滚。提交成功后在事务外调用，context上限20秒；不持有cart/SQL事务锁。第二个独立事务在5秒有界WithoutCancel上下文中记录结果：success调用RecordSessionCreation，将认证密文及succeeded一起提交；failed/uncertain按精确attempt记录。用户取消或归属撤销不丢弃已授权调用的外部事实，也不授予旧归属新dispatch。结果写失败返回Called和原inflight，租约到期只转uncertain，不重复创建。成功重放必须先认证保存的创建结果，裸success不能冒充可恢复结果。

人工MySQL适配器验证：调用期间另开事务取得同cart锁且看到已提交inflight；重叠执行器单次创建；成功重放零调用；取消/撤销后保存结果且撤销归属不可再调；错误未知不重试；明确失败第二次复用相同ID；claim写故障零调用；创建成功但结果写故障不重复，过期uncertain后核实保存结果可完成补偿；已付结果保存却不激活；外层事务和未配置provider拒绝。完整人工链已连接删除成功→持久补偿→创建adapter→加密结果→新session原子激活。报告新增cart_payment_creation_execution，pure Go测试覆盖错误/panic/响应校验和配置复制。

本轮无新增SQL迁移，沿用36迁移。未注册真实支付provider、未接入worker/HTTP或完整购物车命令；真实沙箱/对账、自动恢复授权及业务回滚协调仍缺，T032/T033和checkout未验收。

本轮执行器验收：`go test ./...`、购物车专项与隔离MySQL/Redis完整23组回归通过，36迁移验证成功；完整报告290项/专项187项实现SHA匹配。旧health/Admin/商品/购物车页面均HTTP 200，近期日志无新增错误，仅既有PayPal nonce提示。上述均为人工适配器验证，真实渠道与完整checkout继续待办。

## Mollie 删除渠道适配器（显式配置，尚未注册）

新增 `payments.NewMollieDeleteAdapter`，读取当前渠道状态后再决定是否取消。业务参考 `medusa-store/apps/backend/src/modules/mollie/service.ts` 的 `cancelPayment`；HTTP 合同核对 [Get payment](https://docs.mollie.com/reference/get-payment) 与 [Cancel payment](https://docs.mollie.com/reference/cancel-payment)。原生 Go HTTP 请求固定 `https://api.mollie.com/v2/payments/{id}`，只接收服务端显式 provider ID 和 API key，不读取浏览器、导入配置或环境凭证；禁止重定向，单请求 8 秒、整个读取/取消 15 秒，响应上限 1 MiB，拒绝重复 JSON key。

每次 GET/DELETE 响应都验证远端 ID、本地 session metadata、保存的 cart metadata（存在时）、币种和精确金额，不经 float64。paid/authorized 在 GET 后明确拒绝且不发 DELETE；canceled/expired/failed 已是终态，直接确认。保留原项目 open 且明确 isCancelable=false 的本地丢弃规则，这不代表远端取消成功；启用前仍必须实现旧渠道引用的 webhook/订单核实。pending 不可取消时保留本地会话，缺少 isCancelable 也不猜测。GET 与 DELETE 间付款、DELETE 返回非终态/422/404、限流、网络错误、超时、身份或金额不匹配均进入 uncertain，不自动重试。

模拟 RoundTripper 覆盖正常取消、已付保护、原本地丢弃、处理中会话保护、终态重放、付款竞态、两次响应的五种绑定不匹配，以及重定向/404/429/500/错误/超时/超长/非法/重复 JSON。模拟请求核验固定 origin、鉴权头和硬超时；错误输出保持通用，无真实 Mollie 调用。该适配器尚未注册到应用或 HTTP/worker，也未完成创建渠道、webhook、对账或真实沙箱验收，0035 能力继续默认关闭。

Mollie 删除适配器验证：`go test ./...`、`go vet ./internal/payments` 和隔离购物车专项回归通过；专项报告189项实现SHA匹配（含渠道代码和模拟测试），仍为36迁移。完整23组回归在前一补偿执行器提交1c05471通过，其290项报告保留为该阶段证据；本次不将模拟 HTTP 视为真实支付沙箱验收。

## Mollie 创建渠道适配器（尚未注册）

新增 `NewMollieCreateAdapter(MollieCreateConfig)`，显式配置服务端 provider ID、API key、回跳地址、后端地址、描述及手工扣款开关。对照当前项目 `initiatePayment` 与已安装 upstream 的 `normalizePaymentCreateParams`/七个 services：保留 hosted/ideal/creditcard/bancontact/giftcard/paypal/applepay 及对应原 provider ID，按原路径生成 webhook，默认描述 iPhone retail order、默认 captureMode automatic。私有 context、旧远端 payment ID、浏览器自选 method/URL 不进入渠道请求。不读取环境或导入凭证，不默认注册。

HTTP 固定 POST https://api.mollie.com/v2/payments，复用禁止重定向、8 秒硬超时与 1 MiB 响应界限。请求 Idempotency-Key 固定为持久 SessionID；存在的 data.session_id、data.mollie_attempt、context.idempotency_key 必须一致，不用 journal attempt。回跳沿用 http/https、无凭证、与配置同 origin、/api/mollie-return、cart_id 匹配规则，默认端口按 origin 等价处理。

按既定支付契约纠正原 Number/toFixed 浮点损失：保持两位渠道金额格式，以精确十进制 half-away 结算；购物车 raw/SQL 原值不量化。删除适配器的响应绑定改为复用相同结算格式，避免 1.005 创建为 1.01 后无法核实。大数、进位、指数和 0.005 均有固定测试。其他币种位数/新增渠道策略不在此自动推断，真实渠道支持范围仍需后续验收。

仅接受 201，核对 payment resource、test/live mode、远端 ID、session/cart metadata、精确结算金额/币种与回跳地址。open/pending 保存 pending，paid/authorized 保存 authorized 事实（已有补偿完成逻辑拒绝把已付结果作为未付会话激活）；canceled/expired/failed 按原 update 状态映射。pending 还要求有效 HTTPS Mollie checkout 链接。返回原渠道 JSON 加原 cart_id/redirect_url/session_id，精确大数与私有结果仍由上层加密持久化。

本地校验失败且尚未发 HTTP 是明确 failed；HTTP 非201、错误、超时、身份/金额不匹配、非法/重复/超长响应、非法checkout均 uncertain，不自动重试、换幂等键或返回未验证结果。Mollie 官方 [Create payment](https://docs.mollie.com/reference/create-payment)、[API idempotency](https://docs.mollie.com/reference/api-idempotency)、[Handling errors](https://docs.mollie.com/reference/handling-errors) 为本次核对依据。其幂等缓存只有一小时，不能替代已实现的持久 uncertain 隔离和后续对账；本地仍不自动重发未知请求。

模拟 HTTP 验证七个 provider 与两种扣款方式、完全稳定的重复请求、原请求字段、精确返回 JSON、七种状态映射及25类响应/网络失败；逐项篡改 metadata/amount/currency/mode/resource/redirect/checkout 均拒绝。测试只用内存 RoundTripper，无真实渠道调用。worker/HTTP、商户配置加载、webhook/对账、完整下单仍缺，0035 开关保持关闭。

POST 特别关闭 `http.Request.GetBody`：携带 Idempotency-Key 时 Go transport 对可重放 body 的隐式重试不能绕过 journal；模拟请求断言该重放函数为空。回跳使用原 URL.pathname 对应的 escaped path 校验，编码斜杠不绕过固定路由。原 health/Admin/商品/购物车均HTTP 200，近期日志无新增错误，仅既有PayPal nonce提示。

本轮创建渠道验收证据：最终代码 `go test ./...`、`go vet ./internal/payments`、隔离 MySQL/Redis 完整23组回归和购物车专项均通过；36迁移保持，完整294项/专项191项实现SHA匹配。渠道用例为内存HTTP模拟，未注册或调用真实Mollie，未进行真实沙箱或完整checkout验收。

## Mollie 当前状态读取与未知创建结果恢复

`MollieCreateAdapter.ObserveSession` 接受可信保存请求及候选远端ID，只向固定 Mollie API GET，不 POST/DELETE。候选ID只定位远端对象，不能证明付款或购物车权限；查询复用商户key、8秒超时、禁止重定向和1MiB/重复key防护，响应复用初次创建的resource/test-live/ID/session/cart/精确结算金额/币种/回跳/checkout校验。输出私有 SessionObservation，保留远端status、原webhook动作、精确金额及可加密保存的Session；paid=successful、authorized=authorized明确分开，不能只看二者共用的session authorized状态配货。七种原状态映射有模拟GET测试，跨session/cart/金额/币种/模式/ID/回跳、404、跳转、网络及重复JSON均不返回事实。

实现对照原 `getWebhookActionAndData`，官方 [Webhooks](https://docs.mollie.com/reference/webhooks) 与 [Get payment](https://docs.mollie.com/reference/get-payment)：通知只提供待查ID，渠道当前响应才是状态证据。此处还没有注册webhook入口、ACK、可靠通知队列或订单/支付状态更新，不能把查询帮助函数当作通知流程已迁移。

通用 `SessionObserver` 只允许渠道读取，`CreateAdapters.Observer` 从原服务端allowlist取出可读适配器；`InvokeObservation`复用创建的错误/panic/精确响应校验边界。`cart.ReconcilePaymentCompensationSession`仅接受普通DB pool，首个自有事务重新检查当前购物车归属/generation/version、冻结、源计划和original→replacement关系；要求精确已有attempt。succeeded重放先认证加密结果、不再查询；pending/failed不能通过核实入口获得创建dispatch；inflight以数据库时钟判断租约，未到期不查，到期变uncertain。只有uncertain可在事务提交后做10秒有界GET，任何失败保持unknown，不能凭404或超时标failed再POST。

核实成功后，独立5秒WithoutCancel事务将同一attempt的结果加密保存并原子标succeeded；调用过程中取消/归属撤销也不丢弃已授权尝试的真实渠道结果，不授予新dispatch。结果写失败仍保留uncertain，后续只能再查；补偿最终激活仍须当前归属检查，authorized/paid结果不得激活为普通未付会话。该入口不自动完成补偿、不放开冻结、不创建订单，也没有后台worker授权。

人工MySQL链验证未知创建→GET错误保持uncertain→核实保存原attempt→成功重放无查询→补偿完成，创建次数始终一次；另验证过期inflight只GET、未过期不查询、stale attempt/错误generation/未配置observer/外层事务拒绝、GET无SQL锁、取消后保存、结果写故障保留uncertain及重查恢复、已付核实结果保存而不激活。报告扩展cart_payment_creation_execution；原创建/删除HTTP测试一起回归。没有真实渠道调用；候选ID发现（创建响应丢失且无webhook时）、持久webhook队列/对账授权、商户配置、完整checkout继续待办。

恢复限制：两个并行核实只会并行GET，不新增创建尝试；结果保存仍按原attempt认证并禁止覆盖不同的已保存响应。如果渠道状态在查询间变化而结果冲突，保留冲突供后续付款对账，不覆盖旧creation结果掩盖状态变化。该creation恢复不是付款事件状态机，也不能完成捕获/退款或授予设备配货。

本轮核实链验收：`go test ./...`、`go vet ./internal/payments ./internal/cart`、隔离MySQL/Redis完整23组及购物车专项均通过；36迁移，完整299项/专项196项实现SHA匹配，包含12个新增observer检查字段。原health/Admin/商品/购物车页面均200，近期日志无新增错误，仅既有PayPal nonce提示。没有真实支付调用，完整webhook/对账/checkout仍未验收。

## Mollie 持久通知收件箱与原回调入口（0037）

0037 新增默认关闭的 mollie_notification_runtime、不可覆盖的 mollie_payment_references、代次收件箱 mollie_notification_inbox、加密证据 mollie_notification_observations。RecordSessionCreation 对七种已知Mollie provider，在创建结果/成功receipt同一事务登记 remote ID→精确effect key/digest；参考保存的session/cart metadata与精确结算金额，远端ID及本地session唯一，不按email或外部入参追认。登记失败则创建结果和receipt整体回滚。引用保留原加密请求，旧本地session软删/替换不遗忘可能仍收款的远端尝试。

Gin 注册原七条 `/hooks/payment/{原identifier}_mollie`；默认未配置均503。新增 RouterWithAuthAndWebhooks 显式服务端provider allowlist，原RouterWithAuth行为与购物车配置保持，启动入口未启用通知。接收只允许1KiB内 application/x-www-form-urlencoded 的单个id字段，拒绝重复/额外字段/非法ID；只为已知remote引用入队，未知ID同样200但不新增存储。已知通知必须在入队事务提交后才200，数据库故障503以允许渠道重试；不要求商店浏览器cookie/Origin，不接收或信任通知status/amount。官方依据 [Mollie Webhooks](https://docs.mollie.com/reference/webhooks)：现有通知仅提供资源ID，必须从API取当前状态；未知ID返回相同ACK，通知可多次到达。

每个provider+remote只一条收件箱，但每次通知递增generation。claim保存claimed_generation和UUID租约，8竞争只派发1个；提交后才GET。GET执行期间的新通知递增generation，旧ACK仅推进processed_generation到其认领代次，仍有新代次则pending而非idle。过期lease可新UUID重新GET，旧attempt被隔离；网络失败有一分钟延迟，不当作已付款或ACK处理。DueMollieNotifications按DB时钟返回最多100个待查locators，仍需claim；无Redis唯一事实、无内存任务可靠性假设。

ProcessMollieNotification只接受普通DB pool、显式商户allowlist与business ring。先事务认领，再10秒有界渠道GET，最后5秒WithoutCancel事务持久化完整SessionObservation密文与代次ACK；不发POST/DELETE、不持有SQL锁调用渠道、不更新业务付款/订单/库存。成功查询但结果写失败保留inflight等待租约恢复，失败GET只安排GET重试。观察结果的远端ID、session/cart、精确金额/币种、原动作和session状态再次绑定保存请求，防止错误adapter映射；证据用attempt绑定AEAD，精确JSON不经float，完成重放必须同一结果，错误密钥/篡改不回退。ReadMollieNotificationObservation提供认证私有事实读取，consumed_at继续NULL，不虚称已处理支付或订单。

隔离MySQL+内存HTTP验证：创建引用写故障整体回滚；未知ID不占存储、已知/未知同ACK；严格body、默认/数据库开关关闭；8并发认领单次、8并发通知不丢代次、GET期间新通知保留；回滚/证据写故障无ACK、失败入队HTTP503；过期GET认领恢复/旧attempt拒绝、backoff、idle不重复GET、查询时可另开同记录事务；加密结果读取/篡改拒绝、未标业务消费。报告新增mollie_notifications。

2026-10-30 与当前全部82份迁移一起验证：更新完整迁移计划断言，并补齐隔离创建响应的唯一远端ID、金额/币种与session/cart metadata。Go全包测试、付款/购物车vet、真实MySQL/Redis购物车专项通过。采用当前工作树（包含0037）重新运行新账号购物联合：中英文、390/1440宽度四组真实Astro→Gin→MySQL/Redis购物、模拟支付、成单、账号归属和SQL回归均通过，见 [账号购物报告](shopping-account-joint-report.json)。支付渠道与邮件最终投递仍是隔离替身，未调用真实支付。

当前只有显式配置的通知接收及可调用的worker函数，未启用启动worker/商户配置加载，未实现业务证据消费、支付单调状态转换、成单、退款/拒付或自动对账授权。若创建响应丢失，尚无remote reference的通知仍按未知处理；候选发现和恢复需后续闭合，不能因这个收件箱就开放生产付款。T032/T033与checkout仍未整体验收。

## 2026-11-01 上下文协调接续与来源定位修正

主预览已更新102，见[实际报告](local-preview-102-report.json)，不改变本节已有内部付款协调组件的公开范围。重新执行来源15组回调发现捕获器将pnpm实例目录哈希固化到fixture路径，依赖重装后仅路径变化即报失败。现改为从实际Medusa backend解析其core-flows依赖，fixture使用包内逻辑路径，仍严格核对源文件SHA。重新捕获前后15组输入/输出及源码SHA `6e154a7b4d628a2fff7df1e8df54fe695bdfa40af6fdabe4fc8cf20dc622e064`完全不变，唯一fixture字段变化为source_file；`--check`及现有Go来源对照/非法事实测试通过。原失败日志保存在`logs/gin-migration/main-102-final/payment-oracle-path-failure.log`；未启动应用/数据库或调用渠道。

当前接线缺口已重新确认：SaveContext在重新计价前拒绝任何活跃payment collection，普通写锁也拒绝checkout_attempt_id冻结。已有StagePaymentRefresh按当前持久cart重新计算目标，不能在未保存的地区选择上直接复用；Complete还会独立递增版本。后续须协调目标上下文、原checkout尝试、渠道删除/不确定查询/补偿与单一最终版本，不能简单移除collection判断或解除冻结。原流程金额和币种均相同时保留session，改变其中任一项才删除session并同步集合；这两条都必须保留。已有付款会话上下文切换、付款冻结购物车转移和完整浏览器联合仍未实现，不据此次工具修正关闭这些缺口。

2026-11-01 目标重算已有[内部回滚预览及真实数据库证据](cart-context-payment-preview-report.json)：独立事务核对原激活checkout的加密绑定，在事务内实际应用目标上下文及原顺序报价/配送/税/优惠，生成精确PaymentRefreshPlan后连同所有临时修改完整回滚。金额及币种均不变保留会话，改任一项才计划撤销；新语句复用已通过原预付款浏览器重跑。完整23组102回归220b1cdad9fb、全基础表不变/末尾故障与主数据保持通过。未公开HTTP、未提交新上下文、未解除冻结/执行渠道，后续持久stage/dispatch/reconcile/compensate/finalize仍待；不能将本内部预览当付款刷新闭环。


2026-11-01 已有付款上下文接续：[持久准备与真实联合证据](cart-context-payment-stage-report.json)复用原操作/加密动作表，无新DDL，仅提交target/source和freeze，不改cart/version/原checkout、不认领或调用渠道。修复真实八并发RR当前读与旧快照混用，保留三轮失败证据；专项c363515351b4及全新完整原23组d4cc7037aa68通过，1833源码/二进制/日志SHA核对。原预付款地区/配送/优惠/地址Astro联合cb1a462389b6四组有车+四组关门禁无车、12收据、异常0，三端920/144/645文件核对、自有资源清理。Go/vet通过，主42442/102与299表/215FK/共享86不变，七入口200。未接新HTTP/渠道执行/最终保存；原checkout不可变版本与新有效版本绑定、未知恢复/补偿及C01–C11其余范围继续开放，0103仅已协调预留。


2026-11-01 已有付款上下文接续：[提交认领后的有界执行](cart-context-payment-execution-report.json)复用原执行/取消后事实持久化，保持外部调用期间无SQL锁。真实原生专项677761f57554及完整原23组b840d11908f8通过；验证并发单次、原158.75EUR加密请求、确定失败稳定重试、未知不重删、取消/撤权保留事实、认领/结果写故障与过期租约保护、嵌套/未配置拒绝及原standalone回归。1835源码/二进制/日志SHA、原预付款Astro联合23cef84d56f5四有车+四无车/12收据/异常0、三端922/144/645文件及自有清理核对。Go/vet通过，主42442/102、299表/215FK/共享86不变，七入口200。无新HTTP/真实渠道/主部署，未知核对、补偿、最终保存和有效版本绑定及C01–C11继续开放。


2026-11-01 已有付款上下文接续：[不确定删除只读恢复](cart-context-payment-reconciliation-report.json)绑定原attempt及保存remoteID，活租约不GET，仅terminal unpaid记成功，paid/authorized/open/pending保持冻结，不POST/DELETE。修复首轮339faeb13e78揭示的delete响应与create元数据契约，恢复原加密创建请求并严格对齐身份金额，保留失败证据。专项c8274130f5cc与完整原23组2725668c2708通过；错误身份/金额/权限拒绝、结果写故障可重GET、取消/撤权后已验证事实保存、成功重放无GET及原路径通过。1837源码/binary/日志SHA、原预付款Astro联合499f2e5d7815四有车+四无车/12收据/异常0、三端924/144/645及清理复核。Go/vet通过，主42442/102、299表/215FK/共享86不变，七入口200。尚无独立加密观察全文、新HTTP、补偿或最终保存/有效版本绑定，0103仅预留，全C01–C11继续开放。


2026-11-01 已有付款上下文接续：[原上下文补偿准备](cart-context-compensation-stage-report.json)独立核对原cart/checkout/source，不因新目标报价漂移而阻断恢复；先拒绝未知删除，仅为确认成功删除准备稳定新ID与原加密CREATE请求。补偿提交后禁正常delete stage，无新DDL/渠道调用/业务版本变化。修复首轮5766750ad195的通用request_envelope旧RR快照读取并保留失败；专项9e25d2b2b1a6与完整原23组f1f8bea0eb71通过，六并发唯一/quote drift/末尾回滚/篡改等验证通过。1839源码/binary/日志SHA、原Astro联合0642b3a0516f四有车+四无车/12收据/异常0、三端926/144/645及清理复核，Go/vet通过。主42442/102、299表/215FK/共享86不变，七入口200。替代执行/激活、结果并发绑定、最终保存及新HTTP/UI仍待，0103仅预留，全C01–C11继续开放。


2026-11-01 接续：上下文付款补偿已接私有替代 CREATE 执行与同 attempt 的只读观察/有界发现恢复，复用已有事务外有界调用及结果持久化。实际 MySQL 验证提交 claim 后才调用、并发仅一次创建、明确失败同请求重试、未知创建不再 POST、精确身份/金额验证及丢持久化结果恢复；旧 RR 快照下的加密成功结果读取和幂等记录使用当前锁定读取。完整23组原SQL、Go全包/相关vet和原有地区/配送/优惠/地址四组浏览器回归通过，源码/二进制及隔离清理已核验。详见 [替代创建与恢复报告](cart-context-compensation-execution-report.json)。仍无替代激活、上下文最终保存或新HTTP/UI验收；主预览未更新、未调用真实渠道，0103仅预留，C01—C11继续未完成。


2026-11-01 接续：103新增独立effective_cart_version（NULL保留原绑定），原付款input_version/cart_version/会话/请求摘要不改。私有CompleteContextPayment已原子完成金额和币种不变时的上下文保存，保留原支付会话并释放本次操作冻结；连续两次保存后，原会话pending/paid查询、库存/促销/实收/通知成单及Mollie通知消费仍使用正确当前版本。真实SQL验证六并发唯一保存、最终收据故障全表回滚、旧收据重试、篡改/越权拒绝；102已有记录升级103原字段不变、物理过旧版本拒绝及重放通过。完整23组SQL、Go全包/相关vet和原有前付款四组浏览器组合回归已通过，见 [有效版本与保存报告](cart-context-effective-version-report.json)。新命令尚未接HTTP/Astro；金额改变、替代激活、完整页面联合继续待办。主预览仍102且未更新API，未调用真实渠道，C01—C11未完成。


2026-11-01 接续：私有CompleteContextPaymentCompensation已完成原上下文恢复激活。无成功删除时保留原支付会话并推进独立有效版本；确认删除后，仅从已加密认证成功的未付款CREATE结果创建新的immutable checkout_attempt，保留旧attempt和旧会话历史，原集合金额/币种与购物车上下文不变，不伪造目标地区保存收据。真实MySQL验证每类六并发唯一激活、未创建拒绝、最后一步故障全表回滚、改请求/异主重试拒绝、报价漂移恢复、此前已有有效版本后再替代激活、原会话查询/成单和新会话通知成单/同单重试。完整23组SQL、Go全包/相关vet和原有前付款四组浏览器组合回归通过，见 [补偿激活报告](cart-context-compensation-activation-report.json)。金额变化最终保存、激活前已付款/授权结果恢复、新HTTP/Astro操作闭环仍待继续；新命令未接页面，主预览仍102且API未更新，未调用真实渠道，C01—C11未完成。


2026-11-02 接续：金额变化后的私有 CompleteContextPayment 已在所有旧会话删除成功后原子保存目标上下文、同步原支付集合并退出旧 checkout；旧 attempt 保留且不可再付款，空集合只允许已存在历史 checkout、无有效会话/实收且单购物车持有时复用。地址、配送、普通地区修改可继续同步该集合，新付款复用集合并生成新的 immutable attempt。真实 MySQL 验证待删除全表不变、最终收据故障全表回滚、六并发唯一保存、后续版本旧收据恢复及篡改拒绝、新付款 pending/paid 到唯一订单/扣款/库存预留/通知与同单重试。完整原23组103迁移 SQL、Go全包/vet和原付款前中英双宽度浏览器四组合+四关门禁通过；1848份SQL源码及935/144/645三端源码、二进制、日志和自有资源清理核验，详见 [金额变化最终保存报告](cart-context-changed-complete-report.json)。主预览仍102、API未更新，七入口200、299表/215外键及共享86事实保持，未调用真实支付。币种/精确小数专项、退休后商品/优惠/账户转移金额变化、替代已付款恢复及HTTP/Astro新流程继续开放，C01—C11未完成。


2026-11-02 接续：付款上下文新增两个显式启用的私有 POST 接口 context-payment 与 context-payment/restore，共用原 Context 解码、Origin/CSRF/持有凭证和频率限制。服务器从已认证 manifest 选择会话/attempt，浏览器不能指定渠道请求；响应只含 saved/restored/processing/requires_recovery 及保存收据。协调器限制单次外部变更，已持久化未知删除/创建仅观察同次结果；未知删除经 GET 确认取消后才保存，paid 观察保持冻结并拒绝补偿。恢复原付款保留原上下文且不写目标保存收据。CHECKOUT_CONTEXT_PAYMENT_ENABLED 独立默认关闭，复用原 Mollie 商户及 TLS transport，不重读第二份凭据。真实 Gin router/httptest+私有 MySQL/Redis 验证金额不变/变化、确认删除后的替代恢复、丢成功响应原命令重试、未知删除两次 GET 后取消恢复、paid 保持冻结及输入/权限/私有响应契约。完整原23组103迁移 SQL、Go全包/vet和原付款前四组+四关门禁 Astro 浏览器回归通过；1852份SQL源码、939/144/645三端源码、二进制、日志和自有资源清理核验。见 [付款上下文HTTP报告](cart-context-payment-http-report.json)。Astro bridge、浏览器持久意图和恢复界面尚未接入新接口，本轮浏览器仅证明原流程保留；主预览仍102及原API/关闭开关，七入口200、299表/215外键与共享86事实保持，未调用真实渠道。已付款替代恢复、币种/精确小数等专项及C01—C11完整交付继续开放。


2026-11-02 接续：Astro 私有 bridge 已仅放行 context-payment 与 context-payment/restore 两条 POST，保留 Origin/CSRF/持有Cookie/原操作编号边界；仅新路径使用50秒bridge与55秒客户端边界以覆盖45秒后端操作，旧请求时限不变。typed client 严格区分保存收据、恢复与进行中状态，拒绝混车/篡改版本/混合状态/私有附加字段；未自动重试。后端完成结果新增重新授权的当前cart及有效付款原始CREATE描述符，实测保留原付款(version4)、替代付款(version5)或退出付款(null)，不暴露渠道URL、请求或effect材料；零删除恢复也保留原付款标识。六项传输专项、完整245前端测试及Astro检查、Go全包/vet、完整原23组103迁移SQL通过；原付款前四组合+四关门禁浏览器回归通过。1852份SQL源码及939/144/646三端源码、二进制、日志和自有清理核验，见 [付款上下文传输报告](cart-context-payment-transport-report.json)。主102/API/开关不改，七入口200、299表/215外键及共享86事实保持。付款上下文持久意图、保留原CREATE的前端有效版本兼容、恢复界面和新流程真实浏览器联合仍待；本轮未把新接口挂到页面，不标C01—C11完成。


2026-11-02 接续：Astro 已接付款后地区/语言切换与显式恢复入口，复用 commerce 锁和 context journal。先解析原 CREATE，再在发送前持久化目标与原操作；保存/恢复结果与当前 cart、amounts、payment-options 一致后才更新付款绑定并清理意图。保留原 CREATE 输入版本兼容有效版本推进，未知结果、丢响应、读取或浏览器存储失败均沿原命令恢复。253 项前台测试、含新增绑定存储故障的25项专项及470文件 Astro 检查（0错误/0警告）通过；实际本地 Astro 中英×390/1440四组恢复面板验证通过，覆盖等待禁用、失败原编号重试、刷新恢复、URL/Cookie保持及无溢出。页面测试全部写请求被拦截，不能算 Gin/MySQL 连续联合；详见 [付款上下文页面报告](cart-context-payment-ui-report.json)，保留初次类型检查与测试 hydration 失败证据。主预览仍102/原API且新能力关闭，七服务入口200；新流程真实数据库浏览器联合、其他恢复边界和 C01—C11 完整交付继续未完成。
