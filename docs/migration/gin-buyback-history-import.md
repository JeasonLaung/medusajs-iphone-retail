# 历史回收记录导入事务

2026-10-30。`catalogimport.ImportBuybackHistory` 补齐原 buyback_request、buyback_photo、buyback_device_link 三类记录的事务暂存入口。价格仍使用原 Import；通用目录导入和 schema 生成器范围保持不变。

复用已存在的严格字段转换、金额/raw 配对检查及 migration_catalog_rows 证据账本。保留原 ID、客户标识、申请状态、联系方式、地址、设备项、历史、付款凭证、退回单号、关闭时间、照片审核说明和软删除时间。相同快照和源行重复导入时核对已存目标；源、快照或目标漂移拒绝覆盖。金额使用 DECIMAL(38,18) 显式比较，不能用浮点比较掩盖末位差异。

照片和设备关联先锁所属申请并检查 JSON 设备项 ID，拒绝外来项、空项或重复项；设备外键及活跃 item/device 双唯一约束由正式 0078 表保证。软删除关联保留。申请原 customer_id 仅为历史标识；不建立 customer_accounts 或 customer_buyback_submissions，不按邮箱自动绑定，也不发提醒或改动库存。

调用者必须传入专用目标库事务；出错必须整体回滚。父记录需先暂存，完整快照哈希/尾部、所有源关系、域对账及文件验收通过后方可提交。本入口不会提交事务，也未启用已禁用的旧 import-medusa 命令。复用既有迁移账本，不新增 SQL 迁移。

照片只迁移元数据，原 storage_key 保留；不读取任意磁盘路径，不伪造字节。外部私有图片迁移、历史记录的后台授权入口、真实历史资料完整演练及全量导入协调器仍未完成。

验证在自有 MySQL/Redis 浏览器夹具中进行，使用人工申请/照片及检测设备。覆盖精确重放、源/快照/目标冲突、18 位金额末位冲突、外来项、活跃项/设备重复拒绝、软删历史保留、无新账号归属及调用方回滚。运行器为 `python retail/scripts/test-buyback-joint.py`；专用日志 `logs/gin-migration/buyback-history-import.log`。测试不会向旧业务数据库导入数据。
