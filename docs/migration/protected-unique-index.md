# 受保护业务令牌的完整唯一比较

日期：2026-10-27。Go `uniquekey.SecretRegistry`现已实现受保护退订令牌的专用登记路径，人工MySQL验证通过；28项源长文本唯一键的hold仍保留，真实领域命令/导入器/授权尚未接入。本稿不代表整个邮件模块或生产密钥配置已经完成。

## 索引与密文

索引材料是显式注入、独立生成的32字节HMAC密钥和固定key ID，不读取环境或回退COOKIE_SECRET。HMAC-SHA256输入采用长度前缀：固定域、key ID、源约束scope、原完整组合编码。HMAC只定位候选桶；同桶读取owner及full_value，使用[目标加密信封](protected-envelope-contract.md)解密，再逐字节比较完整组合值。强制碰撞测试验证不同值能并存、相同值冲突；没有普通SHA令牌指纹或明文登记副本。普通scope继续使用既有完整值算法。

登记密文AAD绑定`claim.<scope>`、unique_key_claims/full_value和owner，业务字段密文独立绑定business-capability/mail_contacts/unsubscribe_token和业务ID。两种信封不能互换，换行亦不能解密。业务字段的密文先通过专用接口验证再构建内存登记变化；公开请求不得直接提供scope或登记内容。返回错误固定，不含令牌、owner、email、密文或底层SQL。

`0003_unique_key_secret_scopes.sql`保存scope的固定key ID及加密配置校验值。该值是HMAC(独立配置检查输入)，由同一业务加密环再密封，避免明文暴露索引密钥或认证指纹。每次受保护事务先锁scope配置行、认证并比较预期校验值；同ID但不同材料、不同ID或缺少旧解密key均返回ErrProtected，不能悄悄换桶漏检旧占用。配置记录首次由显式调用创建；其初始化信任调用者提供的密钥，备份恢复必须连同该记录、所有claims与原密钥处理。

scope配置锁持有至事务结束，因此当前同一受保护scope的写入串行；这是明确的吞吐限制。其后复用owner/旧新桶锁、完整比较和同事务写入；不使用Redis锁提供持久唯一性。任何失败都须由调用者回滚整个业务事务。配置行不能在线删除后“重新初始化”；HMAC换key/ID需要独占重建全部登记、对账及版本化切换，目前API明确不支持该操作。业务加密key可保留旧解密材料并切换活动key，但批量重密封/备份恢复操作仍待实现。

## 联系人适配与执行证据

普通SourceChanges/ApplySourceRow继续整组拒绝mail_contacts，普通Apply拒绝受保护scope，防止误写令牌明文。SecretRegistry.ApplySourceRow仅支持已固定的mail_contacts退订令牌方案：普通目标行投影不含token，另一个参数必须仅有该列的非空业务信封。解密认证后同时构建email与token两项登记，不能只验email后漏掉token。业务行完整验证、权限、版本及事务边界仍属调用者。

`TestRegistryMySQL/protected_source_claims_hmac_envelopes_and_fail_closed`在禁网、无端口、1核/768MiB的临时MySQL中运行真实0002/0003 SQL、专用Go适配及生成的mail_contacts表。人工测试验证：

- 恒定测试HMAC下两个不同令牌成功，相同令牌拒绝；相同email不同令牌亦拒绝。
- 数据库登记值无人工令牌明文，能按正确owner解密；业务字段有独立有效信封。
- 错HMAC材料/ID、缺旧信封key拒绝；损坏或换行候选密文不能被忽略，返回存储失败并要求回滚。
- 释放后可由新联系人使用，旧联系人恢复冲突；显式回滚后联系人业务行不存在，重用原值成功。
- 8路不同联系人争抢同令牌仅一事务成功；登记与业务行同事务写入。
- 普通Apply向受保护scope写明文被拒绝。

Go单元测试另验证索引确定性、scope/key ID/材料域隔离及无效配置拒绝。实际测试清单与二进制hash见mysql-type-fixture-report.json，重现按[登记组件流程](mysql-unique-key-registry.md)编译Linux测试二进制后运行隔离fixture。仅人工值，无旧商户密钥、账号、令牌或服务调用；未验证真实数据导入、HTTP权限及密钥丢失恢复。
