# 历史客户主体：业务证据与旧账号绑定排除

2026-10-28。CustomerHistory 与 customerimport 为原 customer 的 12 字段提供专用
转换和调用方事务内的导入。目标是 historical_customer_subjects，不是新客户登录表。
旧 has_account 转为 legacy_had_account，仅记录历史观察。不得据同邮箱、手机号、
原主体 ID 或此布尔值创建会话、接管旧订单、恢复付款方法或取得客户组/员工权限。

## 保护转换

普通 rowimport.Convert 及其他通用保护转换不再接受 customer，即使 metadata 为
SQL NULL 也必须走专用入口，避免普通导入绕过身份处置。完整字段/类型/时间和 SQL NULL
证据仍先受严格转换约束；未知字段、缺失字段、带 Raw 的 SQL NULL 或伪造布尔均拒绝。

固定 v2 快照中 5 行客户 metadata 全为 SQL NULL，没有证据支持任意非空业务内容。
当前接受 SQL NULL、JSON null 和空对象；非空 JSON（包含未知业务备注）拒绝并留待
专门审查，不能通过“已加密”保留旧 token、密码、嵌入角色或新账号绑定。
源快照仍是受限历史证据，不公开原行或联系资料。

SQL NULL 留为 NULL；JSON null 和空对象保持原始 JSON 字节及空白，使用既有
AES-256-GCM 密钥环密封为二进制 LONGBLOB。AAD 固定 customer-subject-history、
historical_customer_subjects、metadata、原主体 ID。不同主体、错误字段/用途、
篡改、未知 key 和明文/旧格式 fallback 均不能读取。读取后再次核对允许的 JSON 基线。
姓名、公司、联系方式、原操作者、软删与 UTC 微秒时间逐字段保留，不按邮件自动归属。

## SQL 原子边界

0018 的 migration_customer_dispositions 与主体行在同一事务保存：来源 ID、快照 SHA、
完整来源行 SHA、完整目标行 SHA、首次随机尝试与策略版本。disposition 固定 excluded、
reason 固定 historical-subject-not-login，两者都有 CHECK；不提供激活或自动重绑入口。
BindingDisposition 仅证明旧绑定被排除，不能作为私有记录访问许可。

相同来源重放先锁回执并读原密文，认证并比较明文后沿用已存密文核对完整目标 SHA，
不会因随机 nonce 重封装而改写证据。轮换期间保留旧解密 key；缺旧 key 时拒绝，不能
用新的加密结果覆盖。目标字段、来源、快照或排除处置改变均冲突，不 upsert 主体。

调用方负责完整快照/来源 schema/数量和关系核对、显式 schema 计划、独立目标与密钥
管理，任何错误或后续领域失败都回滚整个事务。组件不 commit、不访问源库、不开 HTTP、
不发送通知或调用 provider，不操作部署服务器。
历史客户 email 完整值唯一策略仍为 hold，没有建立前缀唯一索引，也没有把旧账号唯一
规则当成新客户注册规则。新客户身份、归属转移和历史数据认领均需独立流程。

## 验证

[离线报告](customer-history-conversion-report.json) 通过完整 159 表/27140 行 v2 framing
与固定 SHA 后，验证 5 行/60 字段、5 个 metadata SQL NULL、5 次普通入口拒绝和 5 项
账号绑定排除；未写 SQL。人工用例另覆盖 JSON null/空对象、末位时间、中文空白、
密钥轮换/缺 key、跨主体/篡改拒绝、嵌入凭据及未知内容拒绝，没有打印实际联系资料。

[联合 MySQL 报告](admin-auth-report.json) 的 customer_history 覆盖 18 文件 schema、
二进制持久化/读取、8 路并发重放、旧 RR 快照重放、主体失败回滚回执、轮换保留原密文、
缺 key/源/目标/快照更改拒绝和 SQL 禁止激活旧绑定；无登录账号创建。

[固定快照 SQL 演练](catalog-mysql-rehearsal-report.json) 通过专用入口导入 5 个主体，
逐行排除旧绑定，随后接入原 336 普通订单行；回滚、提交与精确重放都保持完整指纹。
演练未恢复登录账号，目录/库存/客户/订单仍是分阶段提交，不是正式全域协调器。
本次主体先导入后，13 项订单客户引用可解析，阶段性持久引用差异为 293 项（上一批未导入主体时为 306 项）；这不是 293 笔订单。原 375 项全域缺引用和库存 4 项差异继续保留，不因本批被修复。

尚缺正式全域协调器、长期密钥存储/恢复、其他非空客户 metadata 审查、完整权限和新
客户认证/注册、订单查询与 Ant 页面、checkout/付款/履约/取消及 Astro。不能宣称客户
领域、订单领域或整体迁移完成。
