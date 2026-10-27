# 订单、付款、履约与退回逐字段设计

日期：2026-10-27。T006 第三批42表523列，与目录和设备批合计90表954列，剩余103表未设计。机器设计：[JSON](commerce-field-mapping.json)、[CSV](commerce-field-mapping.csv)。这里没有已执行的MySQL DDL、沙箱付款或新业务验收。

## 保留结构

| 源结构 | 目标设计及意义 |
| --- | --- |
| order、order_address、order_cart | orders、order_addresses、order_carts；原ID/展示号、客户历史主体、渠道/区域、地址成交快照、购物车唯一完成关联 |
| order_line_item、order_item | 分别为成交商品快照与按order/version关联的数量/单价；不能把两表合并丢掉变更版本 |
| order_line_item_adjustment/tax_line | 版本折扣和成交税行；保留原promotion/rate/provider引用及已成交数值，不依赖当前配置仍存在 |
| order_shipping、order_shipping_method及adjustment/tax_line | 运费方法快照及版本归属，保留return/claim/exchange相关运费 |
| order_summary、order_transaction、order_credit_line | 各版本汇总、交易账本和抵扣，分别保留不互相覆盖 |
| order_payment_collection/order_fulfillment/order_promotion | 模块之间真实关联、复合键、原id及软删；没有外键不能据此判定没有业务含义 |
| order_change/action | 请求/确认/拒绝/取消记录、顺序、原操作者、amount和details；用于理解订单变更，不重放历史工作流 |
| order_claim/item/image | 原生索赔、额外行/原因/数量和照片；当前0行仍保留能力，不能冒充已被设备售后完全替代 |
| order_exchange/item、return/item/reason/fulfillment | 原生换机/退回、订单版本、数量、收到/损坏数量、原因树和物流关联 |
| payment_collection/payment_session/payment | 付款集合汇总、provider会话、实际付款记录分别保留，不用一列paid替代 |
| capture/refund/refund_reason | 实收与退款独立账本；退款note保留售后查重证据，但新命令还要持久幂等 |
| payment_provider、payment_collection_payment_providers、account_holder | provider启用配置/集合可用provider、付款商户主体；account_holder不是登录账号 |
| fulfillment/address/item/label/provider | 包裹时间/取消/配送方法、地址、商品数量与库存引用、物流号/标签；分批履约不能压成订单一个tracking_number |

目标表名和全部字段见机器清单。库存、来源、渠道、商品等引用使用前两批目标名；carts、历史客户主体、shipping_option等尚待对应字段设计。order_number_clock不混入订单流水，本轮未覆盖，后续单独编号域处理。

## 金额、数量与版本

numeric字段设计DECIMAL(38,18)，Go内部仍用高精度运算。amount/unit_price/compare_at_unit_price/refund_amount/difference_due是主要货币单位，rate是百分比，quantity类是数量；不能统一按分处理，也不能把difference_due/credit等合法有符号历史值改为非负。raw_*保留并比对标量。action.ordering源int8使用BIGINT和API字符串，禁止JS Number吞精度。

order.display_id旧整数序号与custom_display_id自定义文本均保留，后者不是对象关系ID，不生成新号覆盖。order.version、order_item.version、order_summary.version、change.version及return/exchange.order_version具有不同作用；查询当前视图按明确版本取，保留全部历史行。原记录created_at/updated_at/deleted_at及专用requested/confirmed/canceled/shipped时间保持UTC微秒。

行项目的商品名、型号、SKU、选择值、成交单价、含税标记属于快照；商品/变体/税率/促销后来删除不影响读取历史订单。保存其原引用并允许历史解析失败，不在查询时套当前目录名称/价格/税。order_line_item.totals_id和reference/reference_id等为内部/多态引用，待调用证据确认后解析，不按后缀随意建实体外键。

status/type来自源check或enum约束；迁移不将authorized改为captured，也不把return.received直接视为refunded。运行状态转换和合法动作仍须独立命令及权限，参见[支付契约](payment-contract.md)、[结账契约](checkout-contract.md)和[设备命令](device-commands.md)。

## 身份与provider保护

order.customer_id/order_address.customer_id保留历史客户主体，匹配的是旧业务证据，不是新客户登录表。created_by/requested_by/confirmed_by/declined_by/canceled_by/marked_shipped_by只保留原操作者或system标识；新账号不按邮箱/旧ID自动取得交易。订单地址/邮箱含必要历史联系信息，服务端裁剪并限制访问。

payment_session.data/context/metadata、payment.data/metadata、account_holder.data/metadata可能同时含恢复所需外部引用和秘密。目标使用加密迁移封装保留精确JSON；稳定provider/payment/order/capture/refund引用另按白名单提取并保留来源。未核对字段不公开、不记录日志、不自动删除。与普通JSON_VALID文本不同，加密封装不能直接SQL JSON查询；明文校验在加密前进行，封装版本/密钥管理和MySQL演练仍待T016/T018/T035实现。

实现补充：[payment_session.context审查](payment-context-review.md)现显式移除嵌入旧客户地址簿/账号绑定及默认地址标记，保留经核对的历史联系、交易账单地址和provider付款主体；这是“不迁移旧账号”排除规则对JSON嵌入资料的延伸，不能整段原文复制。真实21个context临时信封往返通过（9个含排除资料），人工LONGTEXT持久化验证通过。原始来源证据保留在受限旧快照；目标是投影后的值。其他保护JSON仍hold，未接入完整支付行转换或实际运行。

后续实现：[payment_session/payment.data专用审查](payment-data-review.md)覆盖42个真实对象，手动仅空对象、Mollie逐层允许字段并精确比对行金额/币种/session和metadata引用；临时新信封往返及42次错行拒绝通过。其他保护JSON和完整行入口继续hold；不调用provider，不恢复扣款/退款或生产手动付款。

不迁移旧登录账号不等于删除付款商户account_holder或provider session事实；这些事实仍支撑退款与对账。在途支付切换另有决策与验收，当前既不调用provider，也不恢复旧session向外扣款。导入不能触发通知、退款、发货或旧事件重放。

## 真实快照差异

```powershell
python retail/scripts/map-commerce-fields.py --check
# 两条命令当前应退出1；真实历史引用未解析，不能按全绿处理
python retail/scripts/preflight-catalog-mapping.py --domain commerce --bundle logs/gin-migration/source-20261026.json
python retail/scripts/audit-commerce-snapshot.py --bundle logs/gin-migration/source-20261026.json
# 人工审计拒绝/金额用例，无数据库或快照访问
python retail/scripts/test-commerce-audit.py
```

字段预检检查7,005个字段值、428个raw配对、840个引用；标量类型/长度/金额精度无失败，但有以下256个未解析引用值，影响162条不同记录。这里不是256笔订单。

| 源引用 | 未解析值数 | 其中未软删除 / 已软删除 |
| --- | --- | --- |
| fulfillment_item.line_item_id | 1 | 1 / 0 |
| order_exchange.order_id | 10 | 10 / 0 |
| order_exchange_item.item_id | 10 | 10 / 0 |
| order_fulfillment.fulfillment_id | 85 | 85 / 0 |
| order_fulfillment.order_id | 85 | 85 / 0 |
| order_payment_collection.order_id | 18 | 12 / 6 |
| order_payment_collection.payment_collection_id | 9 | 3 / 6 |
| return.order_id | 19 | 19 / 0 |
| return_item.item_id | 19 | 19 / 0 |

不能仅因未soft-delete判定其为完整有效交易。也不能推测这些必然是演示/测试记录就删掉。报告只保留字段名和计数，原始ID与内容留在Git忽略快照。对应缺失对象可能在快照前已硬删除或其他历史操作中移除，原因仍需隔离追踪；现有证据不支持自动修补。

独立语义审计运行103项检查：capture/refund非负及不超付款/实收、payment/session/collection币种及归属、collection实收/退款与明细合计、order当前版本summary与行关联唯一。**这些已检查项目无额外失败，但引用审计仍失败**；并不证明完整订单合计、授权金额、旧provider远端状态或历史版本全部一致。9个人工测试检验超额扣款/退款、已删capture、币种/session错配、精确0.1+0.2、负退款、版本重复及孤儿引用能被识别，审计不改输入。

## 隔离导入与对账策略

1. 所有原始行、关联ID和软删状态进入受保护迁移历史，逐行来源/hash可追溯；未解析关联也完整保留，不生成假订单、假付款或假包裹，不因FK失败而跳过记录。
2. 目标业务图只接入可验证的关系；原始reference与nullable resolved reference/问题账本分开。需要隔离的历史聚合只读展示缺失上下文，不提供退款、履约、订单编辑或库存释放命令。
3. 检查问题传播范围：如link的order不存在但payment_collection仍存在，付款事实也不能因link不可解析被清除；应保留外部引用并单独对账。不能只按本表孤儿数决定哪些资金记录丢弃。
4. 外部账本沙箱/受控对账、订单版本重建及文件关联按任务完成后，才给每个问题明确处置；不要把当前字段设计宣称为已修复全域历史。生产源不在本轮修复，MySQL演练也尚未开始。

迁移报告必须展示总行/软删/隔离历史/已解析业务图数量、精确金额、原ID集合、版本和外部引用差异。基线阶段保留阻碍项，随后实现migration_issues及安全只读行为；不通过降低断言、丢失历史或重算成交值来得到成功标记。
