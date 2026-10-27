# 商品关系结构的显式迁移入口

2026-10-28，新增 `0010_catalog_relational.sql`，接入既有 `cmd/migrate` 和语句账本。
生成器从严格目录计划递归取得36目录表+9父依赖，使用审查DDL生成器选择完整
45表/374源列及相应索引、外键。没有把159表全部宣称可投产，不恢复旧账号；
所有导入值必须显式提供，未复制旧触发器。

```powershell
python retail/scripts/generate-catalog-schema.py --check
cd retail/backend
go run ./cmd/migrate
```

仅为显式配置的独立新MySQL。现有0001—0009文件保持原字节，执行器按完整计划、
校验和、专用连接锁和逐语句inflight保护执行。失败后禁止直接重试，须按既有
不确定DDL规则对账；本批没有自动清库、强制标成功或关闭外键。

0010先创建guard：七张旧草稿领域表必须为空，另外38目标表和七个备份表名须
不存在。条件不满足时CHECK拒绝，尚未改名。通过后一次RENAME保留旧表为
`legacy_0001_*`，不删除它们，再创建45表和关系。新admin_users、auth_version、
迁移账本和审计表保持原样。旧草稿业务数据不自动转换，退役v1入口不会恢复。

末尾写入catalog_runtime_schema模型版本、规范DDL摘要和read/write能力；目前
只开放读，writes_enabled受CHECK限制为false。旧UpdateProduct草稿有错误before
审计且无并发/完整权限，已删除实现，鉴权/CSRF后返回503 catalog.write_unavailable。
后续真实命令显式扩展，不把表建成当成可编辑完成。

Gin启动改用schema.CheckApplied核对完整编译计划：所有版本/文件字节checksum、
语句数/进度、applied和inflight一致，没有额外步骤；任意一条migration不再足够。
目录模型marker也须与代码匹配，health复核MySQL、Redis及marker。这些是声明和
账本核对，不等于自动information_schema全结构差异审计；手工DDL检测仍待实现。

十个源索引仍在internal/catalog/schema_plan.json逐项登记：九个长文本完整唯一
键须接入唯一声明组件及相应领域写入/父依赖导入，一个JSON GIN检索索引待转换。
不使用前缀唯一掩盖语义；当前未开放这些写命令。运行时默认值/并发版本/幂等/
outbox、其余114表与全域差异仍缺，T011/T012/T018/T020未整体验收。

[真实MySQL报告](admin-auth-report.json)验证空库10文件和重复跳过，实际核对45表/
374非生成源列；旧9文件空表升级后新建员工账号与auth_version不变。旧商品/地区
有人工行、目标图片表冲突三种拒绝均保留原数据结构、无改名，重跑仍ErrUncertain。
旧计划、模型摘要损坏、inflight和checksum漂移拒绝readiness；直接开启writes
被CHECK拒绝。新账号认证回归继续通过。

[真实浏览器](admin-live-report.json)现由这10文件建库，脚本不再注入审查DDL或
临时改名，继续验证Ant登录/列表/详情/变体/退出。[真实目录演练](catalog-mysql-rehearsal-report.json)
同样改由嵌入计划建库后执行目录子图回滚/提交/重放和HTTP。九父领域正式账本仍缺，
375项全域缺引用保持失败，不能据此开放全量导入或部署。
