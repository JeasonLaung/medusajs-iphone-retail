# 购物车地区与语言事务

2026-11-01。基础批完成预付款购物车上下文命令和前端持久重试参与者，随后已接原地区/语言弹窗并通过隔离浏览器联合；C01 与整个迁移仍未验收。主预览已受控升级到 schema 102，主购物开关保持关闭，详见本文末尾及 [主102报告](local-preview-102-report.json)。

## 来源与行为

来源为原 storefront 的 `locale-actions.ts`、`cart.ts:updateRegion`、地区语言弹窗，以及本地已安装 `@medusajs/core-flows@2.21.2` 的 `update-cart`、`refresh-cart-items`、`update-tax-lines`。已有 [原回调夹具](fixtures/legacy-cart-region.json) 和 `context_plan_test.go` 保留 12 组准备、16 组刷新决策对照。

跨地区时原流程**删除自定义价格商品行**，不能把旧币种金额转成正常报价。单国家目标产生仅含国家的独立配送地址；多国家目标清空旧配送地址；同地区切语言保留地址。账单地址不随该命令改动。

`force_refresh` 让税行进入替换流程，原调用没有传 `force_tax_calculation`。新命令因此使用常规自动算税判断：没有配送国家或关闭自动税时清掉旧税行，不能把强制计算缺地址错误当成原行为。测试曾揭示这一差异，失败证据保留，修正后原断言继续执行。

## 实现

`0102_cart_context.sql` 添加默认关闭的单例 gate 和以 cart/operation 为主键的收据。请求只含规范 version、region_id、locale，operation 来自 Idempotency-Key。币种、渠道、国家目录、报价及所有金额来自服务端。

`SaveContext` 在一个 Repeatable Read 事务中锁定当前归属、generation、版本和目录，检查收据，再处理地址及自定义行，重读新上下文并依次刷新商品快照、价格、配送、税和优惠，核算金额，最后递增版本并写收据。任一步失败整体回滚。重放返回原收据，不重复重算。已有支付集合或付款冻结拒绝，未假记支付撤销。

私有 `POST /api/store/carts/:id/context` 复用 Origin、Redis 会话、CSRF、当前 SQL 归属、32 次/15 分钟限流及 private/no-store 契约。JSON 拒绝重复键、未知字段、null、非字符串和多余尾部。收据只有 cart_id/version/replayed；不更新 cookie。

Astro bridge 和 CartClient 已添加固定 POST 传输。`cart-context.ts` 在同一 Web Lock 下先持久保存目录选择与命令身份，未知响应后复用原命令；保存成功收据后只重读当前购物车和金额，两者版本/地区/币种/语言一致后才清理 journal。无车访客不创建购物车。journal 不存 cookie、地址、支付资料或页面 URL。购物、联系人、配送、优惠、付款和客户购物车转移会拒绝未完成的上下文命令。

## 验证与边界

实际 MySQL/Redis 购物车专项已通过：8 个相同命令并发仅 1 次生效，精确 USD 报价和税额，单/多国家地址策略、自定义价格行删除、同地区语言修改、缺报价/货币精度回滚、末尾收据插入故障的全部表行指纹不变、旧版本/改请求/旧 generation 拒绝，以及真实私有 HTTP 的成功/重放、越权、CSRF/Origin、严格 JSON、限流 429。测试数据库使用专用私有 socket、原生外键，结束清理；未使用主预览数据或真实渠道。

最终完整 SQL 与前台测试证据见 [报告](cart-context-runtime-report.json)。基础报告记录的是弹窗接入前的状态。后续联合见下；尚须继续地区切换后的配送/优惠组合对照、商品翻译阶段核对，以及已有支付会话的持久协调。当前不能声称所有购物车上下文或国家语言交互完成；未升级主预览到 102。

## 原选择器实际接入与联合

[原生联合报告](cart-context-native-report.json)记录 owner `3a2f322ed6e3`。GinHeader 复用原选择器和样式，在购物车命令确认、当前车及金额重读一致后才写独立 `_retail_locale` 偏好 cookie 并导航；请求失败时保留原 URL/cookie，让原错误提示提供重试。这个写入顺序是本批有限行为调整，避免购物车失败却发布新地区偏好。URL 的 en/zh 展示语言仍为权威，da-DK 等显式选择保留；原 query/hash 不丢失。

实际私有 MySQL（102迁移、原生外键）、专用 Redis 命名空间、Gin 和独立构建 Astro 经 Chrome 完成中英×390/1440四组有车联合，以及全部购物命令开关关闭的四组无车切换。无 API 模拟；故障注入只丢弃实际成功响应，验证未知保存重用同一命令、已确认命令读失败只重读、刷新/后退及精确币种/税额。16次POST对应12笔唯一收据，四辆车均最终 version5/EUR/da-DK/空配送地址。六张库存、支付、订单保护表全部行指纹不变（并非空表断言）。浏览器异常0，手机截图人工检查；238项前台测试和467文件Astro检查0错误0警告。

全部临时进程、数据库目录/socket、专属Redis键已清理，三端源文件逐份指纹与当前代码一致。运行后只改测试runner超时清理和自动保存执行脚本的保障，未重跑该清理分支；已保存实际执行版本及两份SHA，报告不混用版本。主预览仍101及旧前台构建，未启用主购物门禁，不能把隔离验证称为9300已更新。

## 商品翻译来源核对

`capture-cart-translations.cjs --check` 已执行安装版本的原步骤回调和纯 helper，保留[五组对照](fixtures/legacy-cart-translations.json)：关闭 flag 时不解析任何服务，有/无变体及空列表均无写；启用时使用 locale 查询并覆盖符合类型的快照，补偿恢复原值，缺变体保留。原 helper 的 string/null 类型判断使 null description、普通 subtitle、string type/collection 保持原快照，不能按直觉全部替换。

来源 translation flag 定义默认 false，当前 medusa-config 只显式启用 rbac，受检本地 .env 未设置该 flag，9001 监听进程45329的初始环境也未含该键。以上支持当前配置使用关闭分支的判断，但不是对容器运行时 flag 的直接读取，也不证明 Gin 已支持启用后的翻译表和更新/补偿流程。未据此删除翻译验收缺口，未启用原站开关或加载旧服务。

## 已有配送/优惠与实际地址表单联合

[最终组合报告](cart-context-checkout-native-report.json)记录实际浏览器 `ef52c108ff7e` 和修复后的完整原23组 SQL `c55f563ef828`。同地区语言刷新保留配送方式 ID/地址并使用新报价；跨币种切换删除不适用的配送及其税行，撤去 EUR 手动优惠，应用 USD 自动20%；多国家目标清空配送地址和旧税/优惠，账单地址保持。原计算器新增第20组精度对照确认42.000000000000000000001的优惠额是8.4，最终税后36.9600000000000000000011；不以无限精度百分比乘积替代原分配规则。SQL末尾收据故障全表指纹回滚、幂等重放以及库存/支付/订单/优惠使用事实不变通过。

首轮实际浏览器揭示一个 SQL 单阶段测试未覆盖的缺陷：源规则生成的 country-only 地址被完整地址读取校验拒绝，GET contacts503，前台解码也拒绝。现仅对私有草稿读取允许有界空字段，仍要求合法国家并限制长度/控制字符；地址提交、配送及其后的付款继续要求完整地址。结账不再仅凭地址对象存在判断完成，草稿会回到地址步骤。四组中英×390/1440浏览器均实际进入带过期payment步骤的结账URL、看到原地址表单、保留国家/邮箱/账单并填写提交新配送信息至version8，再切回EUR至version9。没有改表单DOM/CSS。

同轮还有四组关闭购物开关的无车切换，16次上下文POST对应12唯一SQL收据。916后端/143后台/645前台文件及完整SQL冻结逐份SHA与当前一致；239前台测试、Astro467文件0错误0警告、Go全包及cart/identity vet通过。临时资源清理，原七服务入口200、近期日志无新panic/error。保留首次精度断言失败、浏览器503及修复前绿色SQL证据；不将修复前报告混称修复后验证。

主预览仍101及旧前台构建。下一步是102停写备份/恢复演练、受控主API及Astro更新，再处理已建支付会话的持久协调；不在主预览启用真实业务开关。

## 主102实际更新及运行事实更正

[主102报告](local-preview-102-report.json)记录完整SQL冻结 c55f563ef828、停写新鲜备份演练 15bf79e93807、唯一主Gin二进制 `7efcc5de4f089ce3e4418186e8edf5c6ea282a2dff6744f357bd2f3215b30a89`。101→102只新增关闭的cart_context_runtime和空operations；295原业务表不变。克隆精确恢复101的行/DDL/对象、再次102幂等以及激活门禁后拒绝逆向通过。主299表、297非journal表与演练一致，215外键零孤儿，保留共享86指纹不变；主逆向恢复没有执行。

更正此前多处“旧Astro构建”的运行判断：9300实际一直是PID29403的Astro dev，当前源码通过HMR生效。本批没有重启9300或在运行目录构建。旧报告及冻结SHA保留作为历史证据，不能继续据旧构建假设解释当前行为。

主浏览器首次验证发现Gin缺STOREFRONT_ORIGIN，匿名cart503导致无车偏好失败；已保留失败截图/记录，仅补本地9300地址、受控重启唯一Gin到42442，原配置0600备份保留。二进制/schema/数据库业务门禁/授权未变，再次全表验收通过；匿名cart恢复401。四组中英×390/1440真实Chrome无车选择、query/hash保留、偏好cookie、刷新/后退、选中态和Escape通过，业务写入0。42商店页/21组来源对照机型、顺序、卡片文字及图片一致，当前中英初始HTML各30机型。58页/16组公开SEO严格重跑通过，首轮来源Next中文help重复JSON-LD保留为未定因异常。原服务均200，近期日志无panic/fatal。已有付款会话协调、启用翻译分支、历史与完整C01–C11仍待；不据无车验收开放购物付款。

## 已有支付会话：目标金额预览

[原生准备报告](cart-context-payment-preview-report.json)新增内部PreviewContextPayment：在其自有Repeatable Read事务里重新核对owner/generation/version、原checkout冻结、激活的加密创建绑定与未付款会话，使用与SaveContext相同顺序的参与者实际重算地区/语言、商品、价格、配送、税及优惠。成功也完整回滚，不推进版本、不保存上下文/渠道收据、不解除冻结、不调用渠道；拒绝外层事务。事务内私有scope绑定精确连接、商店、cart与版本，不能与原付款完成只读scope混用。源集合即使金额/币种不变也检查独占归属、未付款状态和加密会话字段；输出只含私有计划与源字段SHA，不含解密资料。

真实已激活Mollie夹具证明158.75EUR不变保留会话、改价160EUR需删除会话、158.75CAD同金额换币种仍需删除，42.000000000000000000001目标精度保留。每次成功/拒绝均核对所有基础表行指纹不变，末尾税行故障回滚前置修改；旧版本、错误generation、无地区/密钥及authorized会话拒绝，公开SaveContext仍因原冻结拒绝。原参与者语句及顺序与上一已验收实现机械对照一致。

完整原23组102 SQL冻结220b1cdad9fb通过，1831份源码及二进制/逐组日志SHA一致；单独专项a07816cacd9c也通过。918后端/144后台/645前台冻结的实际原地区/配送/优惠/地址浏览器联合183c28206e4f重跑通过，中英双宽度4有车+4关门禁无车、12收据/精确金额及六保护表指纹保持，浏览器异常0。该浏览器验证已有预付款链，未通过UI调用新付款预览，不称付款上下文联合完成。专有MySQL/进程/socket/Redis清理，Go全包及相关vet通过；主仍42442/102二进制未变，299表/215FK/原共享86指纹再验、七服务200。

下一步仍需持久保存目标与源绑定、协调原checkout尝试、渠道删除/未知结果查询/补偿，最后一次性保存上下文和版本。此预览不会让已有付款会话的公开切换生效；有车转移、翻译及完整C01–C11仍未完成。


## 2026-11-01 已有付款上下文的持久准备

[真实数据库及原流程联合报告](cart-context-payment-stage-report.json)记录新增内部 `StageContextPayment`。复用 `cart_payment_refresh_operations` 的严格 Format2 manifest 和加密 `payment_effect_journal`，无需新增迁移。事务先锁当前归属，在保存点内实际应用并重算目标，再回滚临时修改，提交原来源指纹、目标计划、受保护的原会话删除请求和 payment-refresh 冻结。即使金额/币种不变、没有渠道动作，也只返回 staged，不能当作上下文已经保存。

重放必须匹配原请求、generation、版本、原 checkout、目标重算及来源指纹。真实八路并发验证一份 manifest、同一结果恢复；改请求/报价、篡改 manifest、错误 generation 和嵌套事务拒绝；末尾 INSERT 故障令所有基础表指纹恢复。金额不变保留会话；目标精确 `42.000000000000000000001 CAD` 仅保存原 `158.75 EUR` 的加密删除请求，未认领或调用渠道。除归属冻结、准备记录、动作账本三表外，所有基础表保持原样，原 checkout 及购物车版本均未改。

保留失败冻结 dafe513c2171、5ac6ea4db4a3、251805d9f46a。后两次诊断明确：首次准备提交后，其余重试先等待归属锁，再以当前锁定读看到记录大小，却用 RR 旧快照读取 manifest，导致 record-not-found。现 manifest 也使用 FOR UPDATE，专项 c363515351b4 和全新完整原23组 d4cc7037aa68 均通过、无跳过；1833份 SQL 源码、二进制、逐组日志 SHA 核对一致。原预付款地区/配送/优惠/地址 Astro 联合 cb1a462389b6 中英×390/1440四组有车及四组关门禁无车通过，12收据、浏览器错误0；它不覆盖新 stage 的 HTTP/UI。三端920/144/645文件 SHA 和自有资源清理已核对，Go全包及相关 vet 通过。

主唯一 Gin42442/schema102未更新，299表、215外键无孤儿及保留共享86指纹不变，七服务入口200，近期日志无 fatal/panic。尚无新 HTTP、渠道 dispatch/reconcile/compensate 或最终保存；C01–C11整体保持未完成。

下一阶段必须保留原 checkout 的不可变 input_version/cart_version，同时显式绑定保留会话后的新有效购物车版本，覆盖重试、付款状态、Mollie通知消费、库存/优惠成单与完成收据；不得直接覆盖旧尝试版本。改变金额或币种则必须先有真实删除/查询证据，再原子提交目标与单一新版本，失效旧 checkout，并能开启后续付款。0103已与“拉取最新代码并启动”任务协调预留给上下文修订绑定，尚未编写或应用。


## 2026-11-01 已有付款上下文的认领与执行

[实际隔离执行及完整回归](cart-context-payment-execution-report.json)新增私有 `ExecuteContextPaymentSession`，在同一个认领事务内重验目标、来源、原 checkout 及当前归属，解密原请求并解析显式删除适配器；仅成功提交的 Dispatch 允许调用。共享原 `ExecutePaymentRefreshSession` 的有界调用和结果持久化，原调用语句顺序保持，仅提取函数。渠道调用在所有 SQL 锁释放后进行；取消/撤权发生于已授权调用途中时，仍有界保存该 attempt 的事实，不授予后续调用或完成权限。

原生专项677761f57554及完整原23组102冻结b840d11908f8全部通过、无跳过。真实并发执行只调用一次；适配器内另开事务重读原准备及 inflight 记录证明提交与释放锁；请求保持原158.75EUR而非新CAD目标。确定未生效失败可用原请求重试；模糊错误仅返回不含私有细节的错误并保持 uncertain。认领写故障不调用、未配置适配器不消耗attempt、嵌套事务拒绝。结果写入丢失保留 inflight，重复及租约到期都不会重删。取消并撤销generation后的原结果保存成功，撤权后再执行拒绝。测试适配器完全人工，逐场景仅清理其协调记录后所有基础表指纹恢复，未真的取消原会话。原 standalone 执行回归也通过。

1835份SQL源码、二进制及逐组日志SHA与当前工作区一致；原预付款地区/配送/优惠/地址Astro联合23cef84d56f5四组有车及四组关门禁无车、12收据、浏览器错误0，三端922/144/645文件核对，自有资源清理。Go全包、相关vet通过。主42442/102、299表/215FK/共享86指纹保持，七入口200，未部署新二进制/开门禁/调用真实Mollie。

不确定结果的只读核对、上下文专用补偿、最终原子保存/有效版本绑定、HTTP与付款上下文浏览器联合仍待接通。当前执行是内部能力，不是用户可用的地区付款切换闭环；0103仍仅预留，整体C01–C11未完成。


## 2026-11-01 不确定删除的只读恢复

[真实SQL与原流程回归报告](cart-context-payment-reconciliation-report.json)新增私有 `ReconcileContextPaymentSession`。先复核当前目标/来源/原checkout/归属及精确attempt；活租约不查询，过期或uncertain只对原保存远端ID进行有界只读观察，不POST/DELETE。核对完整Mollie身份、session/cart、金额/币种及状态映射后，只有canceled/expired/failed可以记录原delete attempt成功；open/pending/paid/authorized保持uncertain与冻结。结果已成功的重放不再GET。查询期间无SQL锁，取消/撤权后仍可保存已授权查询的确切终态事实，但不授予新调用或完成权限。

首轮339faeb13e78保留失败：删除请求Data为渠道响应，缺原创建请求顶层cart/redirect元数据；直接交给原观察器会被身份校验拒绝。修复从原checkout的加密create账本恢复原请求，重新验证初始绑定，并核对两请求的session/collection/provider/amount/currency一致，远端ID仍取原delete数据；未放松观察器校验。专项c8274130f5cc与全新完整原23组2725668c2708全部通过、无跳过。错误attempt/目标/generation/嵌套事务拒绝，金额或远端身份不符及查询错误不改变基础表；结果持久化故障保留uncertain、可重GET；取消同时撤权仍保存已验证终态，撤权后拒绝查询。原执行及standalone路径继续通过，所有适配器均人工。

1837份SQL源码、binary与逐组日志SHA核对；原预付款Astro联合499f2e5d7815四有车+四关门禁无车、12收据/错误0，三端924/144/645文件核对，全部自有资源清理。Go全包/相关vet通过；主42442/102、299表/215FK和共享86指纹不变，七入口200，最近日志无fatal/panic。当前仅写精确attempt结果，未新增独立加密观察全文账本。补偿、最终单版本保存/原checkout有效版本绑定、后续付款及HTTP/Astro付款上下文联合继续开放，0103未编写或应用；不标C01–C11完成。


## 2026-11-01 原上下文付款的补偿准备

[补偿准备实际数据库及原流程报告](cart-context-compensation-stage-report.json)新增 `StageContextPaymentCompensation`，仍无新DDL。它不重算失败操作的新目标，而是锁定并核对原cart/checkout/generation/version、相同请求与manifest、原支付来源指纹及加密请求；目标报价变化不会阻断恢复原上下文。原checkout校验从预览提取，语句顺序机械比对保持一致。

所有inflight/uncertain删除先拒绝补偿；仅确认成功删除的原会话生成稳定新session ID，创建请求从原checkout加密create账本恢复，保留原158.75EUR及cart/回调metadata，不能把渠道response当作create输入。复用原补偿表的严格Format2，旧Format1不兼容。补偿准备提交后，正常context stage/删除入口拒绝继续，原版本、会话与checkout冻结均保留；还没有创建或激活替代会话。

首轮5766750ad195的五个并发重试失败揭示通用加密请求读取问题：锁effect看见新行后，request_envelope仍用旧RR快照。现StageSessionRequest/ReadSessionRequest两处统一FOR UPDATE，保留失败证据，并修正夹具清理的effect_kind字段名。专项9e25d2b2b1a6与全新完整原23组f1f8bea0eb71全部通过、无跳过；六并发一份manifest/一个稳定替代ID、目标CAD报价变77仍可准备原EUR付款、末尾故障全表回滚、未知删除/篡改拒绝、补偿后正常stage拒绝及原执行/恢复回归通过。替代create请求pending/attempts0，逐场景清理自有准备后所有基础表指纹恢复。

1839份SQL源码、binary与逐组日志SHA核对；原预付款Astro联合0642b3a0516f四有车+四关门禁无车、12收据/错误0，三端926/144/645文件核对，自有资源清理。Go全包/相关vet通过，主42442/102、299表/215FK/共享86指纹保持，七入口200。未调用真实Mollie、未更新主binary/开关。替代创建执行/未知恢复、并发succeeded结果读取、激活新的checkout绑定、最终单版本保存与HTTP/Astro付款上下文联合继续开放；0103仅预留，整体C01–C11未完成。


2026-11-01 接续：上下文付款补偿已接私有替代 CREATE 执行与同 attempt 的只读观察/有界发现恢复，复用已有事务外有界调用及结果持久化。实际 MySQL 验证提交 claim 后才调用、并发仅一次创建、明确失败同请求重试、未知创建不再 POST、精确身份/金额验证及丢持久化结果恢复；旧 RR 快照下的加密成功结果读取和幂等记录使用当前锁定读取。完整23组原SQL、Go全包/相关vet和原有地区/配送/优惠/地址四组浏览器回归通过，源码/二进制及隔离清理已核验。详见 [替代创建与恢复报告](cart-context-compensation-execution-report.json)。仍无替代激活、上下文最终保存或新HTTP/UI验收；主预览未更新、未调用真实渠道，0103仅预留，C01—C11继续未完成。


2026-11-01 接续：103新增独立effective_cart_version（NULL保留原绑定），原付款input_version/cart_version/会话/请求摘要不改。私有CompleteContextPayment已原子完成金额和币种不变时的上下文保存，保留原支付会话并释放本次操作冻结；连续两次保存后，原会话pending/paid查询、库存/促销/实收/通知成单及Mollie通知消费仍使用正确当前版本。真实SQL验证六并发唯一保存、最终收据故障全表回滚、旧收据重试、篡改/越权拒绝；102已有记录升级103原字段不变、物理过旧版本拒绝及重放通过。完整23组SQL、Go全包/相关vet和原有前付款四组浏览器组合回归已通过，见 [有效版本与保存报告](cart-context-effective-version-report.json)。新命令尚未接HTTP/Astro；金额改变、替代激活、完整页面联合继续待办。主预览仍102且未更新API，未调用真实渠道，C01—C11未完成。


2026-11-01 接续：私有CompleteContextPaymentCompensation已完成原上下文恢复激活。无成功删除时保留原支付会话并推进独立有效版本；确认删除后，仅从已加密认证成功的未付款CREATE结果创建新的immutable checkout_attempt，保留旧attempt和旧会话历史，原集合金额/币种与购物车上下文不变，不伪造目标地区保存收据。真实MySQL验证每类六并发唯一激活、未创建拒绝、最后一步故障全表回滚、改请求/异主重试拒绝、报价漂移恢复、此前已有有效版本后再替代激活、原会话查询/成单和新会话通知成单/同单重试。完整23组SQL、Go全包/相关vet和原有前付款四组浏览器组合回归通过，见 [补偿激活报告](cart-context-compensation-activation-report.json)。金额变化最终保存、激活前已付款/授权结果恢复、新HTTP/Astro操作闭环仍待继续；新命令未接页面，主预览仍102且API未更新，未调用真实渠道，C01—C11未完成。


2026-11-02 接续：金额变化后的私有 CompleteContextPayment 已在所有旧会话删除成功后原子保存目标上下文、同步原支付集合并退出旧 checkout；旧 attempt 保留且不可再付款，空集合只允许已存在历史 checkout、无有效会话/实收且单购物车持有时复用。地址、配送、普通地区修改可继续同步该集合，新付款复用集合并生成新的 immutable attempt。真实 MySQL 验证待删除全表不变、最终收据故障全表回滚、六并发唯一保存、后续版本旧收据恢复及篡改拒绝、新付款 pending/paid 到唯一订单/扣款/库存预留/通知与同单重试。完整原23组103迁移 SQL、Go全包/vet和原付款前中英双宽度浏览器四组合+四关门禁通过；1848份SQL源码及935/144/645三端源码、二进制、日志和自有资源清理核验，详见 [金额变化最终保存报告](cart-context-changed-complete-report.json)。主预览仍102、API未更新，七入口200、299表/215外键及共享86事实保持，未调用真实支付。币种/精确小数专项、退休后商品/优惠/账户转移金额变化、替代已付款恢复及HTTP/Astro新流程继续开放，C01—C11未完成。


2026-11-02 接续：付款上下文新增两个显式启用的私有 POST 接口 context-payment 与 context-payment/restore，共用原 Context 解码、Origin/CSRF/持有凭证和频率限制。服务器从已认证 manifest 选择会话/attempt，浏览器不能指定渠道请求；响应只含 saved/restored/processing/requires_recovery 及保存收据。协调器限制单次外部变更，已持久化未知删除/创建仅观察同次结果；未知删除经 GET 确认取消后才保存，paid 观察保持冻结并拒绝补偿。恢复原付款保留原上下文且不写目标保存收据。CHECKOUT_CONTEXT_PAYMENT_ENABLED 独立默认关闭，复用原 Mollie 商户及 TLS transport，不重读第二份凭据。真实 Gin router/httptest+私有 MySQL/Redis 验证金额不变/变化、确认删除后的替代恢复、丢成功响应原命令重试、未知删除两次 GET 后取消恢复、paid 保持冻结及输入/权限/私有响应契约。完整原23组103迁移 SQL、Go全包/vet和原付款前四组+四关门禁 Astro 浏览器回归通过；1852份SQL源码、939/144/645三端源码、二进制、日志和自有资源清理核验。见 [付款上下文HTTP报告](cart-context-payment-http-report.json)。Astro bridge、浏览器持久意图和恢复界面尚未接入新接口，本轮浏览器仅证明原流程保留；主预览仍102及原API/关闭开关，七入口200、299表/215外键与共享86事实保持，未调用真实渠道。已付款替代恢复、币种/精确小数等专项及C01—C11完整交付继续开放。


2026-11-02 接续：Astro 私有 bridge 已仅放行 context-payment 与 context-payment/restore 两条 POST，保留 Origin/CSRF/持有Cookie/原操作编号边界；仅新路径使用50秒bridge与55秒客户端边界以覆盖45秒后端操作，旧请求时限不变。typed client 严格区分保存收据、恢复与进行中状态，拒绝混车/篡改版本/混合状态/私有附加字段；未自动重试。后端完成结果新增重新授权的当前cart及有效付款原始CREATE描述符，实测保留原付款(version4)、替代付款(version5)或退出付款(null)，不暴露渠道URL、请求或effect材料；零删除恢复也保留原付款标识。六项传输专项、完整245前端测试及Astro检查、Go全包/vet、完整原23组103迁移SQL通过；原付款前四组合+四关门禁浏览器回归通过。1852份SQL源码及939/144/646三端源码、二进制、日志和自有清理核验，见 [付款上下文传输报告](cart-context-payment-transport-report.json)。主102/API/开关不改，七入口200、299表/215外键及共享86事实保持。付款上下文持久意图、保留原CREATE的前端有效版本兼容、恢复界面和新流程真实浏览器联合仍待；本轮未把新接口挂到页面，不标C01—C11完成。
