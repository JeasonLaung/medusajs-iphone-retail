# 单台设备身份与运行时结构

这批承接设备入库的持久化与身份校验，属于 C05 的局部实现。尚未接入库 HTTP、Ant 页面、配货或发货，不能算设备业务闭环，也不改变整个现有前台的验收范围。

`0055_device_identity.sql` 提取关系 SQL 审核稿中的 devices、device_assignments、device_events、device_sources、device_supply_commitments、device_variant_tracking 六表及原索引，共34条原结构语句。保留软删除唯一性、活动配货唯一性、来源/仓库与追踪 SKU 唯一性、状态与 JSON 约束。增加默认关闭的运行时开关及跨字段身份占用表；没有启用设备命令或增加对外入口。

`devices.NormalizeIdentity` 保留原 SN 去空格及完整 Unicode 大写、100字符上限，IMEI 为15位数字、EID 为32位数字，均以字符串保留前导零。同一设备内重复号码拒绝。是否必须提供号码来自已验证商品：显式 requires_imei（含 false）优先，其次 device_type，最后商品标题/handle/type 的 iPhone 判断，不信任入库请求自报。

调用方必须在同一事务内完成当前用户、SKU、来源、分类、照片及仓库授权，然后先取得 `LockIdentityTx`，再创建设备、调用 `ClaimIdentityTx`、写事件，任一步失败整体回滚。校验器不提供业务授权。跨字段号码在 IMEI/IMEI2/EID 之间共享占用；同时检查尚无占用记录的有效历史设备。软删除后允许新设备重新占用，保留旧档案。后续导入、身份修改和删除必须遵守同一事务锁顺序；直接绕过组件写 SQL 不在该保证内。全局锁先保证正确性，后续若吞吐不足再以实测调整。

保持原规则：单台设备建档不改数量库存；本批没有库存写入。入库命令仍需接来源/仓库授权、有效追踪 SKU、文件真实性、分类初始状态、事件及幂等收据；之后再接设备分配、配货、发货与取消释放。

验证来自仅含本次暂存改动的独立源码快照，排除未提交的 Mollie 通知及其他业务修改。`go test ./...` 通过；`python retail/scripts/test-admin-auth.py --warehouses-only` 在独占、禁外网、未发布端口、单核的 MySQL/Redis 中运行 AdminAuthentication、InventoryStockEdits、ProductInventory 三套件，54项迁移通过。设备用例覆盖默认关闭、自动提交拒绝、跨字段/历史行冲突、6请求争用仅一个成功、同批冲突整批回滚、末尾占用写入故障回滚、软删除后重占及活动配货约束。已有仓库和全店库存回归通过。该轮没有入库 UI、真实前台履约或完整设备历史导入验证；`stock_changed:false` 表示组件未写数量库存，不是完整入库库存对账。证据见 [报告](device-identity-report.json)。
