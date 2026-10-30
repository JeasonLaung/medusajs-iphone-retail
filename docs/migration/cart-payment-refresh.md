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
