# Go导入值的严格转换组件

日期：2026-10-27。`retail/backend/internal/importvalue`提供进入MySQL前的值检查，现已被uniquekey的源行适配复用；旧`internal/importer/catalog.go`仍是未验收草稿，其忽略解析错误、补epoch、24/8金额字段与目录聚合结构尚未替换，不能运行它并宣称完整导入。完整字段执行器和快照演练仍待交付。

## 已实现规则

- Text：只接受JSON字符串，保留大小写、前后空格、Unicode形式和零字节；按码点检查显式长度，不将数值转字符串。无效UTF-8和未配对UTF-16转义直接失败，避免encoding/json替换为U+FFFD而合并身份；有效显式U+FFFD仍保留。
- Boolean：只接受true/false，false不等于未提供，0/1/字符串/null均拒绝。
- Integer：仅32/64位有符号JSON整数字面值，完整范围检查；不经过float64，拒绝指数、小数、字符串和溢出。
- Decimal38_18：JSON数值或显式数字字符串，通过字符串移动小数点展开科学计数法，完全不经浮点数。最多20位整数、18位小数，不舍入。超额小数即使全零也拒绝，负零及可表示的小数尾零保留；异常指数/超长数值失败。该组件不承担主要单位与cost_cents换算，单位由字段映射决定。
- Instant：只接受显式RFC3339时间字符串，转UTC后检查1000—9999年范围；拒绝非法日历、逗号小数、-00:00未知偏移、超微秒词法精度（包括多余零）及越界offset。不能补now/epoch，不能先让DATETIME舍入。
- JSONText：保留原始UTF-8字节、空白、键顺序、重复键和数值词法；拒绝无效字节/未配对转义，无interface{}→float64路径。JSON null是文档，SQL NULL/缺字段的区分由字段执行器负责，不能直接把RawMessage("null")用于所有源类型。

错误固定ErrValue，不带源值/秘密/ID。该组件不读环境、文件、数据库，不执行写入；字段可NULL、缺失、schema白名单、关联和完整业务验证由调用层负责。新HTTP命令可拒绝重复JSON键，历史原文保留不代表公开API允许歧义输入。

## 验证及实际接入

`go test ./internal/importvalue`的4组表驱动用例覆盖38位金额边界/负极小值/科学计数法、精度及溢出反例、UTC跨日、非法/超精度时间、191/192个emoji身份边界、UTF转义、精确原文JSON、布尔false与int32/int64边界。`uniquekey.fieldValue`已统一复用Text/Boolean/Instant，移除重复surrogate实现；其全部源计划、保护字段和事务测试继续通过。

人工MySQL子测试`strict_import_values_survive_actual_sql_without_coercion`实际使用该组件转换38位金额、带+08:00的微秒instant和含大整数/重复键/尾零的JSON，然后写入DECIMAL(38,18)/DATETIME(6)/LONGTEXT，读回与独立预期完全一致。该项记录于mysql-type-fixture-report.json的Go子测试清单；仅人工数据，无真实快照导入。复现按[登记测试流程](mysql-unique-key-registry.md)编译Linux测试二进制并运行隔离MySQLfixture。

## 接下来

[整行转换](mapped-row-import.md)已生成159表/1,691列计划，接入float32尺寸、文本数组与59组raw数值精确配对；[v2快照](cell-record-contract.md)严格区分缺字段/SQL NULL/JSON null并验证完整流及校验和。接下来补受保护源字段转换、新关系结构导入事务、登记和恢复。不能把单值或人工行测试当作全量数据验收；既有交易孤儿/库存差异和源约束hold仍须处理。T006/T012/T018继续进行。
