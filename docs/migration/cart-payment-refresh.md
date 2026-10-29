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
