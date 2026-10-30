# 真实历史回收资料隔离演练

2026-10-30。使用既有固定 v2 快照 `b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985`，先通过完整 159 表流校验，再提取回收申请和照片。源内只有 1 条 photo_review 申请、6 条照片、0 条设备关联；不能用这次实数演练证明历史设备关联有真实数据覆盖。原客户标识在源 customer 表有证据，但不创建旧登录账号。

可复现命令（本机私有路径，均在 Git 忽略范围）：

```powershell
python retail/scripts/test-buyback-joint.py --actual-buyback-snapshot logs/gin-migration/source-v2-20261027-payment-rows.ndjson --legacy-private-root medusa-store/apps/backend/.private-files
```

新增参数可选，默认仍只跑人工浏览器联合；提供时必须同时指定完整固定快照和私有目录。运行器核对快照 SHA256、严格私有文件名、目录约束/尺寸，将内容流入自有 MySQL 容器 tmpfs；没有源数据库连接、公开端口文件服务或外部支付/邮件调用。测试与驱动输出不发布，只读取固定计数摘要。测试完成销毁自有容器及网络。

实库使用生产 ImportBuybackHistory、BindHistoricalStoreTx、ReadLegacyPhoto、ImportLegacyPhotoTx 和 AdminRead/Photo/List：申请和六张照片原字段精确重放，私有字节逐一 SHA256 往返一致，复制后元数据仍可重放；后台状态和联系信息/照片数量与源一致，新客户不能读取，客户归属表没有绑定。所有真实资料最终由调用方事务回滚，确认申请未遗留；原六个源文件哈希保持不变。

真实详情通过现有前端 buybackRequestProjection 原样校验，不使用宽松替代投影。私有 DTO 仅停留在容器 tmpfs 并经 stdin 交给本机 Node，输出仅状态布尔值和数量。现有人工后台历史浏览器与原购物回收流程继续回归。

本次店铺绑定为明确指定的 fixture-store，验证的是迁移归属机制，**不证明真实历史店铺归属已验收**。原业务库和源图片未修改。生产导入协调/完整数据对账、历史未闭单后续运营、真实店铺归属和通知邮箱整体验收仍待完成。
