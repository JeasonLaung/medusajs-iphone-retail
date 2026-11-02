# 本机目录快照与显式导入

2026-10-31：支持本机 PostgreSQL → 私有 v2 快照 → 独立本机 MySQL 的商品目录依赖子图，不启动 Docker。仅完成本期数据导入的目录部分，整个迁移仍未验收。

## 读取与校验

`retail/scripts/export-cell-snapshot.py` 保留原 `--container` 接口，新增互斥 `--local --host 127.0.0.1 --port PORT`。本机连接必须显式指定回环地址、端口、用户、数据库；认证从 libpq 环境或 `PGPASSFILE` 读取，密码不放命令参数。`--psql` 可指定本机程序路径。

两种连接方式执行同一份 SQL：159 表在一个 `REPEATABLE READ READ ONLY` 事务中导出，结构守卫在任何记录前执行，保留 SQL NULL / JSON null、精确数值、UTC 微秒、软删除行、逐表 COUNT、完整 footer 和 SHA256。快照、临时查询及 checksum 使用独占创建、0600 权限；失败/超时保留私有部分文件，不写完成 checksum，不能覆盖已有文件。Go 必须再次完整校验，checksum 文件本身不是来源认证或业务验收。

本机恢复后的 8 张表仅存在删除列物理编号被压缩的差异。结构校验按可见列顺序比较，接受该恢复差异，仍拒绝增删列、换序、类型、可空性和精度变化。`test-native-source-shape.py` 在本机 PostgreSQL 对人工元数据执行 10 种只读 SQL 场景，不创建表、不读取业务行。`test-snapshot-export.py` 验证连接边界、完整 hash、权限、失败/超时和不可覆盖。

## 目录提交边界

新命令 `retail/backend/cmd/import-catalog` 的默认模式只规划，不加载环境或连接数据库：

```sh
cd retail/backend
go run ./cmd/import-catalog --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256
```

只有显式 `--rollback` 或 `--apply` 才连接 `.env` 中的 `MYSQL_DSN`，并要求 `--target-database eurorefurb_NAME` 与其一致、目标为回环 TCP。数据库和账号必须专用于新应用。该名称限制不替代操作者检查连接归属；导入器不创建数据库、不执行迁移，完整显式迁移计划须已应用。

```sh
go run ./cmd/import-catalog --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256 \
  --target-database eurorefurb_NAME --rollback
# 回滚验证及目标检查通过后：
go run ./cmd/import-catalog --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256 \
  --target-database eurorefurb_NAME --apply
```

协调器先验证完整 159 表流，保留固定 37 张目录表及明确审查的 9 张父依赖。普通字段须完整转换，目录闭包内任何缺失引用都拒绝；表依赖和分类自引用按父节点排序，循环拒绝。默认限制全部源行 100,000、保留字段载荷和关系键 64MiB（不是 Go 堆内存上限）。闭包以外的缺失引用明确保留在输出，不能作为全域导入授权。

同一专用连接取得导入互斥锁、UTC/严格 SQL 会话，并在一个事务中写入全部 46 表，保留物理外键检查。目录和九个父表均使用逐行来源、目标、快照及首次尝试账本，并精确读取所有字段验证；DECIMAL 比较显式使用精确类型。业务表不做覆盖更新、不删旧行。已有额外行、不同快照、同 ID 的业务修改或证据冲突拒绝整批提交。同快照重放核对现存字段，保留已有随机证据。崩溃后的事务由 MySQL 回滚；提交结果不确定时只能核实/重放同一快照，不盲换新快照。

## 本机实证及剩余工作

当前只读源快照为 159 表、21,913 行、190,286 字段，普通值检查错误 0；全域仍有 349 个逻辑引用缺口、物理外键缺口 0。历史客户行现在正确标为必须走专用历史转换，不再使普通预检错误中断；不会恢复旧登录或按邮箱授予历史订单。

46 表目录闭包共 17,821 行，闭包内缺口 0。独立本机 MySQL 已验证：全量写入/逐字段校验后回滚，目录/行账本/唯一声明均为 0；再次提交；完整重放前后 46 业务表及 5 个辅助表全部字段指纹相等；在未提交事务中修改商品后重放拒绝，回滚后全部字段仍相等。新 API 绑定导入店铺后，Astro 中英文首页、正式商品列表和商品详情可读；原 Medusa 健康、后台和 `/dk-en/store` 仍可用。

仅目录导入不代表库存、历史订单/支付、设备/私有照片、商户秘密、评价或全域迁移完成。当前新页面仍可能显示无货/评价暂不可用；购物车、真实渠道支付/邮件等开关没有被导入器开启。后续继续库存及历史引用处置、其余领域导入协调、文件对账和完整业务联合验收，不允许以本报告替代 C01–C11 验收。

同日后续进展：[本机历史订单与库存导入](native-history-import.md) 已扩展到历史客户、订单、库存、可选历史支付及履约/税务，共 97 张源表、20,089 行。本段保留上面的目录阶段边界；最终本机缺口和验证结论以该后续报告为准。全域迁移仍未验收，支付按当前用户决定以 Mollie 为本期渠道。
