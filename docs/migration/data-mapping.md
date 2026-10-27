# 源数据去向与字段转换基线

日期：2026-10-27。T006 进行中。清单覆盖 2026-10-26 只读快照的 193 张表、27,779 行，以及当前源库的 1,961 列结构元数据。快照与结构读取时间不同，正式导入时必须再次检查 schema 一致性。

机器清单见 [data-source-manifest.json](data-source-manifest.json)，逐表去向见 [data-table-disposition.csv](data-table-disposition.csv)。只包含表名、列类型、计数、数值位数统计与处置说明，不输出业务行、密码、密文、联系方式或令牌。原始快照仅保留在被 Git 忽略的本机目录。

## 处置类别

| 类别 | 表数 | 处理边界 |
| --- | --- | --- |
| migrate-business | 134 | 进入对应业务域字段映射；不表示已建表/导入，不直接机械复制源表 |
| exclude-login | 13 | 排除旧员工、认证身份、MFA、验证/重置令牌、邀请、API key 及个人授权；重新初始化 |
| exclude-account-profile | 2 | 不导入旧账号地址簿及 customer-account-holder 绑定；订单地址和付款事实另保留 |
| history-subject-only | 3 | customer/group/group-customer 仅保留必要历史主体/统计快照，不创建可登录旧客户 |
| review-open-transaction | 10 | 购物车及其关联：先分支付在途/已完成/普通未完成，不能因不保留旧会话就丢失在途支付 |
| protected-config-review | 6 | 邮箱、Mollie/PayPal、SEO provider、Webhook 配置须核对密文和访问控制；演练无外部副作用 |
| review-notification | 3 | 客户/员工/系统通知和 provider；区分历史、待发、去重与旧身份相关事件 |
| review-ui-config | 3 | 旧 UI 布局/视图/属性标签，按实际定制映射或解释不适用 |
| rebuild-runtime | 5 | 不复制旧锁、workflow 执行实例和迁移账本；未完成外部操作另作对账清单 |
| rebuild-derived | 6 | PostgreSQL 搜索索引和派生产物由已迁移商品重建 |
| rebuild-config | 5 | 重新建立权限规则/角色层级；个人授权不继承 |
| migrate-sequence | 2 | 保留订单逻辑时钟及 ERP SKU 计数位置 |
| review-provider-history | 1 | PayPal metrics 的用途和重报风险单独核对 |

所有表已有初步分类，无默认忽略；原始元数据清单的 status 保持 mapping-pending，单独的领域字段设计才记录设计进度。[目录及配置](catalog-field-design.md)36表292列、[库存/设备/售后](inventory-device-field-design.md)12表139列、[订单/付款/履约/退回](commerce-field-design.md)42表523列、[购物车/促销/税/配送](checkout-rules-field-design.md)33表321列、[回收/邮件/SEO/通知](operations-field-design.md)24表308列、[历史主体/集成](history-integration-field-design.md)12表108列，累计159表1,691列目标设计；另34表270列逐字段排除/重建，全部193表1,961列有设计去向。T006 验收还需要全域约束、关系、默认值、导入顺序和对账断言及运行证据，不能因设计覆盖齐全就标记完成。

## 关键字段转换

| 源字段族 | 目标约定与验证 |
| --- | --- |
| prod_/variant_/order_/dev_ 等 ID | 保留字符串；二进制/明确大小写比较；需要改 ID 则建立唯一 legacy 映射；不能用 uint 自增替换并丢旧链接 |
| order.display_id / custom_display_id | display_id 是旧整数序号，custom_display_id 为自定义展示号文本，两者不得合并丢失；19 位自定义号在 JSON/React 保持字符串 |
| price.amount、行成交价、税/优惠/运费/退款、回收价 | 主要货币单位，采用 Go decimal 与明确 MySQL DECIMAL；不整体乘除 100；API 精确字符串与展示适配一致 |
| device.cost_cents | 最小货币单位整数，单独约定，不与商品 amount 共用不明确的转换函数 |
| inventory_level / reservation_item 数量 | 源为 NUMERIC，并有 raw_*；先分析实际精度和领域约束，设备业务整数限制不等于所有源数量可无损转整数 |
| raw_amount / raw_quantity 等 JSON | 审核其精度信息与标量是否一致；导入不得经 JavaScript Number/Go float64；冲突报差异而不是静默择一 |
| timestamp with time zone | 统一 UTC DATETIME(6) / Go UTC，记录读取与展示时区；订单编号明确 Asia/Shanghai，不使用机器本地时区 |
| jsonb / metadata / items / history | 保留未知但有效业务字段和数值词法；明确列级 JSON schema；MySQL JSON 不保留原始键序，按业务规范化内容对账 |
| USER-DEFINED enum | 转受约束字段/参考表；列出合法状态和可转边；不能因旧 enum 不可直接用而放开任意字符串 |
| deleted_at | 保留所有有效及软删行；活动唯一性与唯一分配在 MySQL 用生成列/身份注册表等显式方案，不能使用不存在的部分索引语法 |
| SN / IMEI / IMEI2 / EID | 文本；规范化、同机重复、跨列冲突及软删除后重用规则单独测试；唯一约束不能只按每列独立 UNIQUE |
| order_item / order_line_item / changes / summary | 区分版本数量、成交行快照、变更/调整与汇总；旧商品改价/改名不回写历史成交内容 |
| payment/capture/refund/session/provider ID | 保留外部交易引用、状态和关联；旧认证账号不迁移不等于可丢付款账号持有人/在途 session 事实；敏感令牌字段单独保护 |
| file storage_key / public URL | 迁移公开资源及受保护对象，保留哈希/关联；私有文件不能由旧 URL 直接公开挂载 |
| customer_id / actor_id / created_by | 业务历史引用映射到历史主体快照，不能关联成新可登录账号；客户归属 API 根据新的明确授权处理，禁止邮箱自动认领 |

## 编号实际算法

证据：`backend/src/modules/order-numbering/migrations`、`sku-numbering/migrations`（根路径为 `medusa-store/apps`）。

- 订单时钟每 tick=10 微秒，单行 last_tick 原子推进为 max(last_tick+1, floor(epoch_seconds*100000))，输出上海时区 YYYYMMDDHHmmss + tick 后 5 位，共 19 位。时钟回拨仍保持逻辑递增。最早迁移曾用 100 微秒/4 位，后续迁移已经升级；新系统导入当前计数，不能再次乘 10。
- SKU 空/纯空白时分配 ERP + (last_id+10000)。清空已有有效 ERP 编号时保留旧号；手工输入较大 ERP 编号推进计数器，普通自定义 SKU 保留。必须同时导入历史软删编号及当前 last_id，不能只取有效 SKU 数量重新起号。
- MySQL 用事务内锁定计数行或等价原子更新，多个进程共用数据库事实；Redis INCR 不能成为唯一永久编号来源。并发、回拨、重启、手工保留与导入重试都有验证。

## 外部副作用与历史凭据

不迁移旧账号不等于删除所有邮箱或支付商户配置：后者是业务集成配置，仍属于保护性迁移审查。邮箱 AES-256-GCM 的旧封装为 base64(iv).base64(tag).base64(ciphertext)，12 字节 IV，key=SHA-256(MAIL_ENCRYPTION_KEY 或 COOKIE_SECRET)。若做兼容/重加密，密钥只在本地受控配置中使用，不写文档或日志，测试用人工密钥和样例密文。

注册验证邮件不得随历史待发队列重放。普通 outbox、营销投递、付款/退款超时和未完成 workflow 需要独立操作对账；读取旧 workflow_execution 仅用于识别未完成事实，不启动旧执行实例。新旧系统同时运行期间不能同时消费同一批真实副作用。

## 下一步字段映射交付

按依赖先建 catalog/pricing/region/channel 的精确字段表，再 inventory/device，再订单/付款/履约/售后及回收、邮件运营。每个目标字段附 source expression、转换函数、NULL/默认、精度/单位、索引/关系、导入断言及去向任务。旧表不迁移的字段必须有原因；source-manifest 与导入工具差异检查作为门槛。

第一批机器映射见 catalog-field-mapping.json / .csv；349条源约束、759个索引和17个枚举值见 source-schema-constraints.json。真实只读快照的标量/长度/精度/raw配对/引用预检见 catalog-preflight-report.json：151,907个字段值、1,779个raw配对、36,535个含软删对象的引用检查无失败。未验证MySQL DDL/活动唯一键/所有语义关联/JSON词法往返，不能当作已导入。

第二批预检见inventory-device-preflight-report.json：55,692字段值、2,880 raw配对、5,506引用检查；119条软删历史引用未解析单独保留。inventory-device-semantic-report.json有1条库存层预留合计差异及3条预留>现货观察项，审计退出1，不改旧源、不覆盖数量、不假造来源或订单行。隔离导入问题账本和受影响写入限制见该领域设计，未完成对账不标验收。

第三批commerce-field-mapping.json/.csv覆盖42表523列；commerce-preflight-report.json与commerce-semantic-report.json记录9类256个未解析引用值，影响162行且有未软删除记录，预检/审计退出1。当前103项付款实收/退款/版本检查无额外差异，9个人工审计用例通过，但全交易尚未对账。保留孤儿历史与外部引用，不假造订单或自动重放支付，详见交易字段设计。

第四批checkout-rules-field-mapping.json/.csv覆盖33表321列，预检4,729字段值、106 raw配对、543引用通过。购物车分类13已完成、1未完带付款历史、38无关联付款集合；不恢复旧身份、不自动清理在途引用。verify-field-mappings.py核对各批不重叠、全列覆盖、类型/NULL及报告哈希，明确输出既有失败；该完整性命令成功不等于业务对账通过。

第五批operations-field-mapping.json/.csv覆盖24表308列，累计147表1,583列，剩余46表378列。预检802字段值、61 raw配对、72引用通过，10项自定义语义检查无失败；2封旧注册验证邮件禁止重放且导入前清除旧认证capability，6张私有照片待清单/哈希验证。凭据只核对源类型，密文解密及重加密尚未演练；邮件、SEO与通知演练无外部副作用。详见[运营字段设计](operations-field-design.md)。

第六批history-integration-field-mapping.json/.csv覆盖12表108列，目标累计159表1,691列；nonimport-field-disposition.json/.csv另给34表270列明确排除/重建理由。193表1,961列均有处置设计，verify-field-mappings.py验证两类互斥且无漏列。100字段值、1,584项编号/运行时检查无失败；3个旧格式订单号保留、1份PayPal连接配置待核对。逐字段设计覆盖齐全不等于T006验收；数据库约束/默认值、密钥/文件与既有差异对账仍待完成。详见[历史/集成及排除设计](history-integration-field-design.md)。
