# 102 → 103 字段升级：Windows 换机验证

2026-11-02，接续用户提供的 `mysql-schema102.sql.gz`。本项仅验证新增字段和约束，不恢复私钥、运行应用或启用支付/邮件渠道。

## 结果

- 在全新、无网络和端口映射的 MySQL 9.6.0 容器中恢复已校验的102备份，开启 native FK / foreign_key_checks，事件调度关闭。
- 当前显式迁移器首次输出 `Applied 1, skipped 102 migrations`，再次输出 `Applied 0, skipped 103 migrations`。
- `cart_checkout_attempts.effective_cart_version` 为可空 `BIGINT UNSIGNED`，已有记录保持 NULL；`ck_checkout_effective_version` 已启用。
- 299张原基表中，排除两个迁移账本表，对297张业务表的45,278行逐原始字段核对：NULL、二进制、精确金额、时间和内容指纹一致。原102迁移校验和保持一致。
- 完整结构导出对比仅有预期的新字段/CHECK变化；原表、视图、触发器、例程和事件定义保持一致。源备份文件未改变，临时容器与其数据卷已删除。

去敏结果见 [升级报告](schema103-windows-rehearsal-report.json)。此结果证明提供的102备份可以升级到103；没有升级任何已有业务实例，也不替代付款上下文的实际浏览器连续联合验收。

## 修复换机阻碍

Windows `core.autocrlf=true` 将部分迁移文件检出为CRLF，导致Go内嵌文件SHA256与原MySQL账本不符。迁移器拒绝执行符合预期。新增根目录 `.gitattributes`，仅对 `retail/backend/migrations/*.sql` 固定LF，恢复仓库原始SQL字节。未修改历史SQL逻辑、账本或校验规则。

跨平台验证命令（需要Docker、Go和Node，预先拉取 `mysql:9.6`）：

```sh
node retail/scripts/rehearse-schema103.mjs /path/to/mysql-schema102.sql.gz
```

脚本只使用自建容器、随机所有权标签和新匿名数据卷，不挂载宿主目录、不连接已有MySQL、不开应用服务。退出时核对所有权并清理自有资源；原始结构和失败诊断留在Git忽略的 `logs/gin-migration/`。

验证：真实备份升级/重复执行/全量业务行与结构对比通过，`go test ./internal/schema` 通过。额外 `go test ./...` 发现既有 promotion 测试仍要求19个用例，而交接fixture已为20个；该失败另项处理，不表述为全包通过。9001/8000与新预览端口在本次开始前即未监听，本项没有启动或停止应用服务。
