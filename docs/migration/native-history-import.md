# 本机历史订单与库存导入

`retail/backend/cmd/import-history` 复用已经审查的目录、历史客户、28 张订单表和两张库存表转换器。默认只做完整 v2 快照规划，不加载环境或连接数据库；报告中的零条目标异常不代表已经检查目标，必须看 `target_verified`。

```sh
cd retail/backend
go run ./cmd/import-history --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256
go run ./cmd/import-history --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256 \
  --target-database eurorefurb_NAME --rollback
# 查看回滚结果与依赖缺口，再显式提交：
go run ./cmd/import-history --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256 \
  --target-database eurorefurb_NAME --apply
```

目标必须是独立回环 TCP MySQL，数据库名与 `MYSQL_DSN` 和显式参数一致，完整 SQL 迁移已应用。导入器不会创建或修改表。客户、结账和退款运行开关必须关闭；期间不要运行任何会修改目标的业务 worker。与目录导入共用数据库互斥锁，严格 SQL/UTC 会话、外键检查始终开启，所有领域在一个事务中验证及提交，失败整批回滚。

客户历史使用私有 `.env` 中持久化的 `CUSTOMER_KEY_ACTIVE` / `CUSTOMER_KEYRING_JSON`；密钥不可提交、丢失或每次随机重建。导入不会创建登录账户、会话或客户订单归属，也不会按邮箱连接新旧用户。非空旧客户 metadata 仅接受已有专用策略批准的空对象/JSON null，未知字段拒绝。

目录及父表先逐字段核对，再导入客户历史、订单、库存。旧订单中尚不存在的支付、履约等父项使用既有不可变 `order_migration_reference_holds` 保留，绝不创建替代订单或补造父项。两张库存表的有效引用缺口拒绝；仅允许编译策略已经批准的、**已软删除预留**缺少订单行这一逻辑历史引用，原 ID、删除时间、数量和逐行来源账本全部保留，报告显式计数。这不是全域引用验收豁免。任何物理外键缺口仍拒绝。

库存先完整保留原值，再通过既有 `inventoryimport.PreserveHolds` 记录预留不符、负可用量或无库存行预留等不可变异常，不修改数量或释放预留。目标额外行、来源快照变化及同 ID 业务修改拒绝；同快照重放验证所有字段，包含 DECIMAL 的第 18 位小数、加密历史及其首次证据。

## 可选历史支付

显式增加 `--include-payment-history` 才把已经审查的八张历史支付表加入同一事务；还须配置独立持久化 `PAYMENT_KEY_ACTIVE` / `PAYMENT_KEYRING_JSON`。默认不会导入这些表。敏感渠道上下文使用现有受保护历史策略，未知字段或不符合策略的载荷拒绝；历史付款和会话保留执行排除记录，历史账户持有人不恢复账户链接。此选项不调用 Mollie、不恢复旧付款任务，也不授权扣款、退款或通知分发。

```sh
go run ./cmd/import-history --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256 \
  --target-database eurorefurb_NAME --include-payment-history --rollback
```

`payment_history_source_rows` 表示快照内八表来源总数；`payment_history_staged` 表示事务中确实验证过这些表，`applied` 才表示命令提交成功。

## 可选历史履约和税务

`--include-logistics-history` 把既有转换器审查的八张履约/配送范围表和四张税务表加入同一事务，使用相同来源账本、目标逐字段验证和不可变引用留置。只保留历史事实，不创建运单、不发货、不调用物流或税务提供商。顺序是先验证订单，再写入这些依赖；`order_reference_holds` 是订单校验时的观察数，依赖导入后可能减少。初次观察账本保持不可变，后续只读对账区分已存在父项与仍缺失项，不删除或释放旧观察。

```sh
go run ./cmd/import-history --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256 \
  --target-database eurorefurb_NAME --include-payment-history --include-logistics-history --rollback
```

## 可选历史购物车

`--include-cart-history` 在同一事务中保留十张历史购物车表。先导入地址，再按依赖顺序导入购物车、行项目、支付关联及配送记录；支付父项须已存在。所有来源引用必须存在，不能用旧客户邮箱恢复归属。关联表使用编译映射中的完整复合主键，不假设每张表都有单列 ID。

导入时锁定购物车运行时能力配置，要求写入和 `cart.create` / `cart.access` 命令关闭。发现来源购物车已有任何新运行时归属即拒绝。导入只写历史事实及不可变来源证据，不写访问权、创建作用域、客户绑定、结账尝试或支付执行表。`runtime_authority_restored=false` 不代表已经完成新的购物流程验收。

```sh
go run ./cmd/import-history --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256 \
  --target-database eurorefurb_NAME --include-payment-history --include-logistics-history \
  --include-cart-history --rollback
```

## 可选设备前置数据

`--include-device-prerequisites` 加入 `device_source`、`device_variant`、`device_supply` 三张表，依次保留来源、SKU/库存项追踪关系和供应承诺。它不导入设备 SN/IMEI、设备分配、事件或照片，也不把供应承诺转换成数量库存。全部父项必须已存在；同一事务锁定设备运行时配置，身份和命令开关均须关闭。

本机已用现有目录父项完成独立事务演练：2 条来源、1,558 条追踪、0 条供应承诺；精确重放、时间末位漂移、缺失父项、两个运行开关分别开启后的拒绝均通过。事务内部核对设备、分配、事件、身份声明、开关和库存/预留指纹保持不变，全部测试写入回滚。这 1,560 行随后已正式提交，并完成包含订单/支付/库存/购物车在内的提交后整批重放。 新增 CLI 选项仍要求完整迁移已应用；使用固定提交 `8d7aed6` 的完整 86 迁移基线加本批导入源码构建，不混入正在编辑的订单迁移，也不绕过版本检查；可复现源码及二进制摘要见 `native-import-release-report.json`。

原生设备前置验证见 `native-device-prerequisites-report.json`。当前两张回收私有 WebP 的来源键、MIME 和字节已只读核对，生产受限读取器确认源文件不变；尚未复制目标私有字节，也未因此验收回收申请/设备关联。

## 设备本体只读审计

```sh
go run ./cmd/audit-device-history --snapshot /absolute/private/snapshot.ndjson --sha256 SHA256
```

此命令不加载环境、不连接数据库，先验证完整 159 表流及全部引用，再使用生产设备规则核对有效 SN/IMEI、商品 metadata 的 radio 要求、跨字段身份唯一性与照片引用；按原语义从 `order_item` 的历史版本核对分配/售后行归属，不猜测最新订单版本。只输出聚合数量，不输出设备身份、客户或订单标识，不规范化来源数据。

本机 52 台设备中 7 台有效、45 台已删除；有效身份、1 条分配和1条售后申请的订单行语义检查均无异常，50 条事件保持原事实。设备照片引用为0。44处缺失来源和11处缺失仓库都在删除设备上；编译策略仅批准前者的历史保留，后者仍要求父项存在。因此 `identity_import_ready=false`，设备本体未因本次审计导入，也没有放宽引用规则。完整证据见 `native-device-history-audit-report.json`。

## 本机验证边界

2026-10-31 的本机快照中，目录依赖闭包 17,821 行，订单相关 324 行、历史客户 4 行、库存/预留 1,710 行。后面三类已完成整批回滚、正式提交，以及提交前后精确重放和未提交数量冲突回滚测试；登录账户及订单归属字段指纹保持不变。库存包含 1,562 条有效库存行、6 条有效预留；72 条删除预留保留缺失订单行引用。原始库存数量异常留置为 0，订单导入时记录 254 处目标依赖缺口，不能把它们当作已修复。

随后八表历史支付 74 行、履约/配送范围 148 行及税务 8 行均完成回滚、正式提交和提交后精确重放/数量冲突回滚。十张历史购物车表的 135 行也完成相同验证，另验证金额末位冲突、已有新归属冲突均拒绝，人工测试改动全部回滚，归属及运行时表指纹保持一致。设备前置三表另保留 1,560 行，目前本机验证范围是 110 张源表、21,784 行。历史支付、履约及税务当前自身引用留置为 0；订单初次留置观察仍保留 254 条，其中 32 条已有父项、222 条仍缺失。原始快照的 349 处逻辑引用缺口没有因此被更改或豁免。源税率表本来为 0 行，导入没有补造税率。

实际商品详情已从无库存变为可读取来源库存的状态；购物车/付款开关依然独立关闭，当前商品页的加购按钮禁用并可能显示暂不可加购提示。真实 Mollie 渠道、邮件、设备/文件、评价，以及其他领域仍待验收，`complete_import_accepted` 始终为 false。本机回滚/提交证据见 `native-history-import-report.json`；命令支持本身不代表真实渠道或全域迁移完成。
