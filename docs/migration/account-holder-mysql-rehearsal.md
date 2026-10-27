# 真实历史付款主体 MySQL 演练

2026-10-27，结果见 [account-holder-mysql-report.json](account-holder-mysql-report.json)。固定 v2 快照 SHA256 `b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985` 中的一条 system provider 历史主体，已走过专用 Go 原子导入及实际 SQL。此演练不代表完整数据迁移、付款能力或新客户归属已验收。

Go 先完整检查159表/27,140行的记录、列计划、表/文件摘要及结束记录，再独立验证全部引用。375项历史缺失与零项物理外键缺失必须符合固定证据；关系仍未验收。随后只选择唯一 account_holder 及其实际引用的 payment_provider 原行，普通转换 provider、专用转换 holder；不改写 provider 原 is_enabled，也不因此执行它。

目标为唯一所有权临时容器，使用本机 MySQL 8.4 镜像（实际8.4.11），单核、768MiB、禁网、无发布端口。完整159张审查表及0002/0003/0007显式迁移在独立库创建。数据位于512MiB tmpfs，固定输入流向128MiB tmpfs；非私有Go测试程序位于 `/opt`，避开本机tmpfs noexec。没有源数据库、应用DSN或部署服务器连接。

实际执行及检查：

- 同一事务暂存1 provider、1 holder、1操作hold和1完整provider/external_id唯一声明，回滚后业务、账本、owner/bucket/claim全部为空。
- 再次整批提交，验证各行计数；读取hold三次（回滚阶段、提交阶段及重放阶段）。
- 精确重放不重插provider、不重写holder；全部目标行、账本、唯一登记的带表/长度/SQL NULL标志内存指纹不变，原nonce及key ID保持。
- 持久化data解密后与源JSON字节一致，metadata保持SQL NULL，错误行AAD被拒绝；独立检查原holder对应完整唯一scope的登记存在。
- 没有导入旧客户账号、认证或付款绑定表。external_id和email只保留历史标量事实，不作为新客户授权依据，不调用provider方法。

报告只保存固定表名、计数、源快照hash和代码hash。源ID、原文、密文和内存指纹不输出。新随机32字节Ring key只在Go内存，未写出恢复密钥；演练结束核对所有权标签删除容器和唯一临时测试程序，tmpfs副本与临时新密文随之消失。字符串/进程复制无法保证物理清零，不宣称密码材料被彻底擦除。

```powershell
python retail/scripts/rehearse-account-holder.py --snapshot logs/gin-migration/source-v2-20261027-payment-rows.ndjson
```

完整全域导入/恢复、375项引用缺口、持久密钥/读取授权、其他provider资料、付款主体操作hold的执行器集成及Gin业务仍缺。专用组件没有一般账号恢复或解除hold入口，T006/T018仍不验收。
