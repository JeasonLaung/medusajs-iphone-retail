# 历史主体、集成与编号；排除及重建字段

第六批映射 12 表、108 列，业务目标累计 159 表、1,691 列。另有 34 表、270 列逐字段注明排除或重建原因。193 表、1,961 列的设计去向覆盖齐全；**T006 仍未验收**：MySQL DDL/导入、密钥/文件、语义对账、索引约束及旧身份材料排除均须运行验证。

映射工具 `retail/scripts/map-history-integration-fields.py`、排除工具 `design-nonimport-fields.py` 均显式声明列名，新增/缺少字段阻止生成；`verify-field-mappings.py` 检查两类互斥、源类型/NULL、元数据/约束哈希及逐列覆盖。仅覆盖完成不能证明后续导入器遵守设计。

## 保留的业务目标

| 来源 | 处理 |
| --- | --- |
| customer / customer_group / customer_group_customer | 独立历史主体、分组与成员快照，保留原 ID 与必要业务资料；has_account 重命名 legacy_had_account，只作审计。无登录接口、密码、会话、邮箱唯一注册约束或新账号自动归属；旧分组不能给新账号价格权益 |
| mollie_config | 保留环境、回跳配置及 enabled 证据；API key 经旧 MOLLIE_ENCRYPTION_KEY/COOKIE_SECRET 的 AES-GCM 解密后转新信封。演练独立 hold，不因 enabled=true 发请求 |
| paypal_connection / settings | 商户凭据属于业务集成，不等于旧客户登录材料；连接当前列与 metadata.credentials 各环境都需保护。auth_code、secret、access_token 和嵌套设置不按普通 JSON 公开。不能假设使用 Mollie 的密文格式；禁止导入时交换授权码、刷新令牌或重连商户 |
| paypal_webhook_event | 保留 event/transmission 去重、原始证据、尝试与重试时点、处理状态；不把 status 当验签或结算证据。演练不处理事件；待处理/失败项必须对账后纳入新持久化 worker |
| paypal_metric | 单独历史运维统计，不能冒充销售账、再次向远端上报或用于生成付款/退款事实 |
| notification / notification_provider | 保留业务通知历史及去重，JSON 受保护；认证相关 payload 的 token/链接须排除。receiver_id 是历史接收主体，不自动 FK 新账号。channels 从 PostgreSQL text[] 转明确 JSON 数组，保留顺序、空数组和可能的 NULL 元素 |
| order_number_clock / sku_number_counter | 原布尔 true 单行及 int64 值原样保留；事务锁定原子分配。订单 tick 已为 10 微秒，不重复乘 10；SKU 计数含已软删记录和手工 ERP 编号。API 大整数使用字符串 |

源码证据：旧 backend/src 的 Mollie security、order-numbering/sku-numbering migrations；已安装 `@easypayment/medusa-payment-paypal@1.3.2` 实际 exports 指向 `.medusa/server/src`，本轮读取其中 4 个 models。此项仅证明实际模型声明，支付业务函数与编译产物的完整对应仍待验证。

历史 metadata 不能无审查变成新账号材料。原始快照可留受限来源证据；新历史库只保留经过审查的业务字段，剥离旧登录 grant、验证/重置能力。全量密封复制不等于已实现“不迁移旧登录账号”。新后台可授权查看历史资料，但不能按邮箱把历史订单、评论、回收或分组分给新登录客户。

## 不导入的逐字段处置

详见 `nonimport-field-disposition.json/.csv`。每列目标为 null，并明确理由及任务；不制造 Medusa 兼容影子表。

| 类别 | 表数 / 列数的用途边界 |
| --- | --- |
| exclude-login | 13 表：员工、auth/provider identity、MFA、验证/重置、邀请、API key、个人偏好和账号角色关联全部不导入。新的公开商店渠道配置从业务事实重建，不复用旧 publishable key |
| exclude-account-profile | 2 表：旧客户地址簿、account_holder 账号绑定不恢复；订单/购物车/履约地址及付款事实由业务映射保留 |
| rebuild-config | 5 表：旧 RBAC 定义只作动作覆盖参考，按新授权矩阵重建；无自动角色等价或员工赋权 |
| rebuild-runtime | 5 表：Medusa/MikroORM 迁移账本、锁、workflow execution 不启动/重放。未完成外部副作用另行核对，新 SQL/作业/幂等账本独立建立 |
| rebuild-derived | 6 表：搜索物理表、tsvector、索引版本和同步游标不导入；从已核验商品/选项/价格/渠道/库存重建，满足搜索契约再验收 |
| review-ui-config | 3 表：固定快照均零行。新 Ant Design 布局/表格按后台契约重建；正式切换前再检查，若新增实际定制则须显式映射，不沿用“空表”假设删除 |

排除列不需要伪造 NULL/默认值转换；它们需要导入器证明从未持久化其值。运行时“重建”亦不代表不用核对旧未完成任务。

## 本批证据与剩余项

固定快照预检 100 字段值无失败。编号/运行时审计 1,584 项无阻断发现，3 个旧格式订单号原样保留、1 份 PayPal 连接配置待凭据核对；5 个人工反例验证软删 SKU、上海时区/10 微秒、旧格式与无效日期区分、单行约束和 pending hold。没有调用 provider、解密真实秘密或改变源行。

累计完整性核对仍明确输出原来的 256 个交易缺失引用、119 个已软删库存/设备历史引用；另有库存预留合计差异在独立报告中。全部字段去向齐全不会消除这些问题。仍需全部默认值/索引/关系的 MySQL 落地、孤儿隔离与对账、密钥重加密、文件清单、身份材料排除验证，以及新的订单/支付/权限/UI 端到端验收。
