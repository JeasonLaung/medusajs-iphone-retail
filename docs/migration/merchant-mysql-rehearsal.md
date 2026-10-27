# 真实商户配置隔离 MySQL 演练

2026-10-27，结果见 [merchant-mysql-rehearsal-report.json](merchant-mysql-rehearsal-report.json)。它证明固定真实快照中的商户配置走过 Go 原子导入及实际 SQL，不代表完整迁移、生产配置可用或 worker 已实现。

输入为已忽略的 v2 快照，SHA256 `b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985`。Go 完整校验 159 表、27,140 行、表摘要/结束记录/文件 hash 后才开始商户暂存，另重新执行完整关系校验，要求保留 375 项已知缺失、物理外键缺失为零。关系仍未验收，不能把子域演练当全域接受。

七表中实际有 mail_account、webhook_endpoint、mollie_config、paypal_connection、paypal_settings 各一条；seo_provider 和 seo_setting 为空，不算真实样本。旧域解密由受限 VM 中的既有 TypeScript 实现独立计算，Go 经显式域 reader 验证；旧域密钥、源 ID 和预期原文由 Node 子进程 stdout 进入 Python 内存，再通过匿名 stdin 送入容器内 Go。脚本不向用户转发该私有 stdout/stderr，不写私有 payload 文件，不把源 key 放参数或环境。固定快照中的 PayPal 字符串都是 SQL NULL，JSON 仅为已审查 baseline；非空真实分环境凭据仍没有证据。

目标为唯一随机所有权容器：本机已有 MySQL 8.4 镜像（实际 8.4.11），单核、768 MiB、禁网、无发布端口、512 MiB 数据 tmpfs 和 128 MiB 输入 tmpfs。创建完整审查版 159 表及 0002/0003/0006 账本，不修改旧数据库。使用实时 stdin 流向 tmpfs 写入快照，非私有测试可执行程序放容器 `/opt`；不通过 archive copy 绕过 tmpfs。新 32 字节密钥仅存在 Go 内存，不备份或持久化。

实际验证：

- 首次整批暂存 5 业务行、5 hold、1 邮箱完整唯一键登记；回滚后业务表及全部账本/登记表为空。
- 整批提交得到相同计数，三个阶段逐条检查 hold，共 15 次。
- 重放 5 行后，全部配置列、账本摘要/首次尝试值及唯一登记的私有内存指纹完全一致，保留原 nonce 和 key ID。
- 8 个保护字段 SQL 读回并解密，与 Node 独立预期逐字节一致，3 个为空字符串；8 次错误行上下文被拒绝。
- 3 个保护字段 SQL NULL 保持 SQL NULL；原邮箱 ID 确有对应 scope 的唯一登记。ID、内容和指纹不进入报告。

报告只含固定 schema 名、计数、快照 hash、代码 hash、限制及清理状态。Python/JavaScript/Go 字符串和进程复制无法保证物理清零；可清零的字节尽量清零，结束核对所有权删除容器及临时二进制。数据 tmpfs、私有输入副本与临时新密文随容器消失，无真实新密钥持久化。提供的旧 `.env` 能认证本快照，不能证明线上运行时 key 或生产恢复流程。

```powershell
python retail/scripts/rehearse-merchant-config.py --snapshot logs/gin-migration/source-v2-20261027-payment-rows.ndjson --env medusa-store/apps/backend/.env
```

仍缺全域完整值转换/正式导入组合及恢复、375 项引用缺口对账、其他保护 JSON、生产密钥管理/授权和 worker/Gin 执行器强制执行 hold。源配置的 enabled/connected 不代表可以发信、注册回调或刷新 token；本演练未调用 provider 或部署服务器，T006/T018/T035 不验收。
