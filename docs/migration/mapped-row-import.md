# 显式字段计划的Go整行转换

日期：2026-10-27。`retail/backend/internal/rowimport`执行六批映射的159表/1,691列转换计划；已经人工MySQL验证目录标签整行、唯一登记及参数化SQL同事务路径。它尚不是完整导入执行器，旧目录导入草稿没有被替换，也没有导入固定真实快照。T006/T012/T018继续进行。

## 计划与范围

`build-import-plans.py`严格使用六个固定字段映射文件生成Go嵌入的field_plans.json，含各输入的规范换行SHA256、源表/字段、目标表/字段、类型、nullable、转换名、字符上限和显式枚举值。校验表/字段集合及目标列不重复；未知转换或缺少枚举定义拒绝生成。`--check`检测漂移，不读业务行、环境或数据库。程序在进程中只解码一次为私有不可变查询表，避免每行解析全部映射。

Convert只接受有计划的源表，34项排除/重建表不能通过此入口恢复。必须提交该表全部源列，缺字段、额外字段、非法值任一失败都返回空BoundRow，不输出半行。返回固定目标表及有序Columns/Values，Values用于SQL绑定，不能拼接业务值进SQL；不会运行源默认值、改ID、补epoch/now或按邮箱建立新账号关系。

Cell显式包含SQLNull和Raw：SQLNull=true时Raw必须为空且列可NULL；SQLNull=false时必须有合法字段值。JSON字段的Raw=null是非SQL-NULL文档，空字符串也不是NULL。任何把SQL NULL与JSON null合并成同一JSON null的旧导出均不能直接喂给本接口；快照适配必须提供来源证明或显式报告歧义，不能猜测。当前没有用推断补足真实快照中的null类别。

普通文本/身份/历史主体引用保留原文，遵从映射字符上限；枚举按逐字段允许列表；布尔、整数、金额及时间使用[严格值组件](strict-import-values.md)。时间作为显式UTC微秒SQL字符串绑定。minor_unit_integer按源int32范围保留整数，不换算单位。有限float32仅用于尺寸字段，先解析为源float32值再承接DOUBLE；legacy_float_rate_decimal按源十进制呈现转换，不混入尺寸浮点路径。文本数组保留数组顺序、空字符串、NULL元素及原JSON文本，不接受数值元素或逗号拼接。

### raw数值配对

生成计划显式列出59组raw字段及对应标量字段；缺对应字段、非数值转换或覆盖数漂移拒绝生成。Convert只在完整字段转换及全部配对通过后返回BoundRow：两侧SQL NULL必须一致；非NULL raw必须是精确含value和precision的对象，缺键、重复键、额外键和JSON null均拒绝。precision须为1—1,000,000,000的JSON整数，保留其原值。value按十进制语法、1024字节及指数±1024上限解析，用math/big.Rat与已经过DECIMAL(38,18)检查的标量精确比较；无float64、容差、舍入或乘除100。

本机安装的@medusajs/utils 2.21.2 `dist/totals/big-number.js`使用bignumber.js的toPrecision(precision)，默认20；bignumber.js 9.3.1的precision有效范围上限为1E9。precision是有效数字设置，不是MySQL小数位数。raw的`1.2300000000000000000`应与标量1.23相等，因此raw.value不套用标量18位小数的词法限制；原始raw JSON仍原文绑定，绝不按precision再次舍入。未知历史对象形态明确拒绝，不能当作可自动修复。

人工测试覆盖59组计划、超JS安全整数的一位差异、38位数、负零、指数、小数尾零、微小不相等、NULL组合及非法对象。含必填受保护字段的行仍返回ErrProtection，该类表的配对测试只验证独立配对组件，不计作已解密或已导入。非受保护行验证不一致时返回空BoundRow。真实历史数值与业务单位对账仍须完成；此处不是业务计算引擎。

受保护非NULL字符串/JSON返回ErrProtection，必须后续逐域解密/密封再绑定，不可写原文或绕过；可NULL字段的SQL NULL可以保留而无需解密。JSON字段合法性与原文保留不意味着其provider token已获准公开。错误固定且不包含业务值、行ID、密文或源payload。

### 源CHECK约束

字段计划现钉住source-schema-constraints.json的SHA256，并核对六批映射的约束证据hash。生成器将159张映射表涉及的全部47条源CHECK编译成结构化规则：39条文本枚举、3条金额非负、1条整数非负、2条布尔单例、2条税区NULL组合。剩余3条源CHECK属于排除/重建表，不恢复旧表。运行时不执行源SQL；未知表达式、未映射字段、字段转换类型不匹配、enum_text允许值与源CHECK不一致均拒绝生成。

其中商品状态、价格表状态/类型、价格规则操作符、通知状态、Mollie环境原本使用utf8_text转换；现在仍保留其原始文本与目标设计，额外由CHECK验证允许值。枚举逐字节匹配，不做大小写或空格归一化。CHECK接受UNKNOWN（SQL NULL），与字段NOT NULL独立；JSON文档null不是SQL NULL。非负金额用精确有理数检查，负零通过，负的最小可表示小数失败。税区分别执行parent_id IS NULL OR province_code IS NOT NULL、parent_id IS NULL OR provider_id IS NULL，不合并为另一条业务规则，也不补造关联。

Convert在全部字段及raw配对之后执行CHECK，只在全部通过时返回BoundRow；必填受保护字段仍先hold。Go测试覆盖47条计划的合法/非法输入、完整NULL真值表、精确数值边界，以及商品/价格/编号/购物车整行拒绝；购物车反例的raw和负标量保持一致，确保失败来自非负CHECK。Python生成器6组边界测试拒绝未知SQL、不同cast、缺字段、错类型及枚举漂移。这些测试不是47条真实业务数据对账或新业务写路径接入的证据；税区外键与MySQL CHECK级联冲突仍须另行解决。

离线审计另以固定SHA256的原始忽略快照执行Go的同一CHECK组件：35张有约束的映射表、47条规则，其中30条有真实行、17条为空样本；397次行/约束检查、21次NULL列输入、无失败。见[source-check-snapshot-report.json](source-check-snapshot-report.json)，逐约束保留次数和实现hash，不保存业务值。这一检查只允许已证明为文本/布尔/数值的CHECK列利用v1的null，不尝试区分JSON列的SQL NULL/JSON null，也不执行Convert或MySQL导入。空样本继续由人工边界覆盖，不作真实历史覆盖承诺。

```powershell
python retail/scripts/test-import-plan-checks.py
python retail/scripts/audit-source-checks.py --bundle logs/gin-migration/source-20261026.json
```

## 已运行验证

`go test ./internal/rowimport`逐项执行159张表/1,691字段人工类型输入；受保护项明确断言ErrProtection，不能把这些计作已导入。逐表验证缺列/多列拒绝，标签表显式SQL NULL/非法NULL双重表示，独立手写期望覆盖int64边界、38位金额、UTC微秒、布尔false、数组/NULL/含逗号文本、尺寸浮点、原文身份与非法类型/溢出。默认go test无数据库写入。

`TestRegistryMySQL/mapped_source_row_bound_sql_and_unique_claim_same_transaction`调用真实Convert(product_tag)、源唯一键适配及实际生成的catalog_tags表，在同一事务绑定写入，读回验证标题空格、JSON大整数/重复键/小数尾零、UTC微秒与可NULL列。另行插入JSON null文档并证实metadata IS NULL为false。该次人工表写入不构成完整导入器或全表事务策略；测试记录见mysql-type-fixture-report.json。

```powershell
python retail/scripts/build-import-plans.py --check
```

MySQL集成复现沿用[登记组件隔离流程](mysql-unique-key-registry.md)，编译Linux测试二进制后使用本地镜像禁网、无端口、1核/768MiB执行并清理。

## 剩余实现

[逐列v2记录](cell-record-contract.md)已在人工PostgreSQL证明SQL NULL/JSON null来源区分及Go重复字段拒绝，完整159表流封装与校验和验证已运行；旧v1固定快照不能据此自动消除歧义。仍需真实一致快照导出与正式导入执行器；受保护域转换；真实raw/CHECK对账及关联检查；有顺序的正式SQL迁移与完整事务/幂等/运行账本；唯一登记/业务写入/恢复覆盖；已发现库存与交易差异对账。空库字段可转换、少数人工行可写入不代表全量数据已迁移或可以切换运行时。
