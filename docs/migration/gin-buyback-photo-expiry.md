# 回收照片 180 天清理

2026-10-30。依据原 workflows/expire-buyback-photos.ts 与 jobs/expire-buyback-photos.ts 迁移。原任务每天 03:00 执行，仅 closed_at 严格早于 now − 180×24 小时的未删除申请，清理未删除且尚未 expired 的照片。

## 实现

ExpirePhotoPage 固定本轮 cutoff，按申请 ID 游标分页（每页最多 100）；每个申请独立事务。应用时锁申请后重新核对 closed_at，再锁照片，以固定顺序每批读取最多 100 条元数据，不把图片字节加载到进程。清理所有历史版本，不只当前照片。删除 buyback_photo_bytes 与清空 storage_key、标记 expired 同事务，任一步失败均回滚。保留照片元数据、审核说明、业务申请、历史与通知。未闭单、恰好 180 天、最近闭单、软删、已过期均不新增删除。

原系统的外部私有文件路径尚未经过 Gin 导入验证；本任务只处理明确 buyback-db:<photo_id> 或空存储键，其他路径保持原样并在 legacy_files 计数，绝不使用任意路径执行文件操作。历史文件真正删除仍待导入/存储适配；这部分不能计完成。

## 独立命令

在 retail/backend 使用已迁移的专用 MySQL：

```powershell
go run ./cmd/expire-buyback-photos
# 默认仅预览，photos 是候选总数，legacy_files 是待导入外部对象数。
go run ./cmd/expire-buyback-photos -apply
# 每天进程所在时区 03:00 执行，首次等待下个 03:00。
go run ./cmd/expire-buyback-photos -apply -daily
```

命令启动先 CheckApplied，不建表、不启动 API。整轮 10 分钟上限、退出信号取消；失败已提交申请保留，未提交申请下次重试。日志仅数量，不记录私有图片内容、键或客户信息。日调度用日历日期保持当地 03:00，覆盖夏令时切换。没有启动任何实际清理进程或操作旧开发数据库。

## 验证

- 在实际 MySQL/Redis 浏览器联合夹具中验证 180 天前后 1 微秒、恰好边界、未闭单、预览、过期、旧外部路径、重复运行与调用方回滚。三条实际照片（含被替换版本）全部按规则处理。
- 元数据写入故障注入验证：删除私有字节后发生错误，事务恢复原字节和元数据。
- 命令调度单测覆盖 03:00 前/恰好/后、跨日和欧洲夏令时日期。Go 全量测试通过。
- 原回收客户及后台审核、检测报价、登记打款与拒绝退回两条联合路径保持通过。报告见 [打款联合](buyback-joint-report.json)、[退回联合](buyback-return-joint-report.json) 的 photo_expiry 字段。

## 待完成

旧回收申请/外部私有照片导入与清理、通知邮箱整体验收、真实设备入库到回收浏览器联合以及 HTTPS Secure cookie 验证仍待办；本批不宣称全部迁移完成。
