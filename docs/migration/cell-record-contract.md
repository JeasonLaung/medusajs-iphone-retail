# 保留SQL NULL与JSON文档的逐列记录

日期：2026-10-27。源码核对确认现有export-medusa.cjs以to_jsonb(整行)构造v1快照：SQL NULL与JSON null在该表示中都成为JSON null，不能从固定快照可靠反推。新纯SQL构造器cell-record-sql.py与Go rowimport.DecodeRecord提供v2逐列格式，已在隔离人工PostgreSQL验证，未重新采集现有业务库或替换固定快照。

## 固定记录格式

每条记录只有format_version=2、table和cells三个字段。table必须属于159表显式字段计划；旧登录表等34项排除/重建表不能通过Go解码器。cells必须精确覆盖该表全部字段，不可缺失或多出字段。

- SQL NULL：cell仅有`{"sql_null":true}`，字段必须允许NULL。
- 非SQL NULL：cell仅有`{"sql_null":false,"value_json":"该列PostgreSQL JSON表达的原始文本"}`。value_json必须是字符串，其中内容必须是有效UTF-8 JSON。

例如JSON文档null使用sql_null=false和value_json字符串`null`，与SQL NULL完全分开；文本空串则value_json包含JSON引号。数值以PostgreSQL的to_json(列)::text先变为字符串，再被外层JSON转义，避免外部JS/Go数值解码改变大整数或小数尾零。JSON/JSONB列保留数据库当前表达，不承诺恢复PostgreSQL存储前已被jsonb规范化的键顺序、空白或重复键。

SQL对每列先做IS NULL判断，然后构造对应cell；通过VALUES及json_object_agg避免宽表函数参数数量上限。标识符仅接受显式计划的受限ASCII名称，业务行值不拼接进SQL。所有表的真实导出必须处在同一只读REPEATABLE READ事务、固定UTC和一致schema中；纯构造器本身不连接数据库。完整流式导出、快照manifest和恢复机制尚未实现，不将多次查询称为同一快照。

## Go边界

DecodeRecord接收最多16MiB的单条记录，验证完整JSON、有效UTF-8/UTF-16转义、精确字段集合和逐层类型。root、cells及每个cell都检查重复键，不允许JSON解码覆盖前值。v1、未知字段、缺字段、未知表、sql_null非布尔、NULL附带value_json、非NULL缺value_json、非法JSON原文或尾部第二文档均失败；不返回部分行。拒绝后错误固定，不回显记录、行ID或秘密。

成功结果是[整行转换层](mapped-row-import.md)的SourceRow，仍需Convert做逐字段类型/长度/精度及受保护转换判断。解码不是SQL写入、快照整体校验或域授权；同样不是保留所有真实历史字段/账号排除已验证。

## 实际证据与复现

`test-cell-record-postgres.py`使用已有本机postgres:16-alpine，禁网、无发布端口、1核/384MiB、临时数据盘。创建实际text/jsonb/timestamptz人工product_tag三行，执行真实构造器查询，再调用Go TestActualPostgresCellRecords→DecodeRecord→Convert；验证SQL NULL、JSON null、大整数9007199254740993、小数1.2300、UTC微秒及空串。另158张人工空表执行对应查询，覆盖宽表语法；空表字段使用text替身，只证明SQL结构，不能算逐类型真实行验证。报告cell-record-postgres-report.json仅计数/哈希/镜像ID，容器结束核对所有权删除；记录内容只保存在Git忽略的人工fixture文件。

```powershell
python retail/scripts/test-cell-record-postgres.py
```

默认go test对实际PostgreSQLfixture子项skip，独立单元测试验证重复root/cell键、非法/多余结构、v1拒绝及NULL区分。必须运行上面隔离脚本才有真实PostgreSQL路径证据。

[单事务全表导出及Go流式校验](snapshot-stream-contract.md)已通过人工159表/3行路径验证，包含逐表COUNT、footer及整文件hash，未重新采集业务库。仍须可信真实运行manifest、类型/约束指纹、旧固定快照null歧义处置、受保护字段转换、关联/金额raw对账及正式MySQL导入执行器；不能为了推进而把歧义null默认为SQL NULL。T006/T018未验收。
