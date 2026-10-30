# 私有回收照片上传与读取

2026-10-30。新增客户 POST `/api/store/customer/buyback/requests/:id/photos`、GET `/api/store/customer/buyback/requests/:id/photos/:photo_id`。沿用账号锁后 Redis 会话复核及显式申请/店铺归属，上传需 CSRF，所有响应 private/no-store；不向客户端暴露存储键。

保留原校验：四字段严格 JSON，item_id 按 JS trim/UTF16 长度归一化，六种照片类型，JPEG/PNG/WebP 原字节签名和声明 MIME 匹配，解码最大 3 MiB。兼容原合法字母表下 Buffer.from 的无 padding、不完整末尾 sextet 行为；重复键、字段大小写别名、未知字段和超限仍拒绝。只允许 awaiting_photos 与 ready_to_ship，后者只能上传 packaging/sealed；每个 item 最多 30 条有效照片，过期记录也占原配额。申请行锁防止并发突破。

0076 增加私有图片字节表。复用现有设备照片的数据库私有存储方向，记录与字节在同一事务写入，后续失败一起回滚；读取重验 SHA256、长度、字节签名和双侧 MIME。已过期、删除或不属于本申请的照片不返回；写开关关闭不阻碍合法读取。旧 provider storage_key 保留，其文件导入/解析尚未实现，不能声称历史照片已迁移。

全局请求边界和 Astro bridge 只为固定 POST 照片路径放宽到现有图片大小限制；其他客户写接口继续 16 KiB 桥接上限。GET 图片响应允许完整 base64 3 MiB，要求正确成功状态，无 cookie 写入权限。仅传递四个既有 BUYBACK 照片错误码及相应状态，隐藏服务端说明，客户组件可使用原中英文文案。新增客户客户端和严格公开照片/图片内容投影；未挂原照片控件。

## 验证

- 75 正式迁移、Go 全量通过，隔离源码快照未包含另一组未提交的 Mollie 工作。
- 真实 MySQL/Redis/Gin：3 MiB HTTP 上传及 SQL 字节往返、损坏哈希拒绝、过期拒绝、跨账号读写拒绝、10 种禁止状态、待寄出仅两种照片、外来 item 拒绝、后续失败整体回滚、配额填至 29 后 4 并发仅 1 成功/3 拒绝、HTTP session/CSRF/查询/重复 JSON、写禁用仍可读及申请软删保护。
- Go 单元覆盖三种原格式、非 canonical base64、trim、严格字段及精确 3 MiB 边界；全局边界单元覆盖固定 route/method 放宽。
- 169 项前端测试通过，含 3 MiB bridge 双向传输、原字节签名、普通申请路径不放宽、严格 201/200、内容 MIME/身份/额外字段和本地化错误投影。Astro 359 文件 0 错误/警告，5 hints。
- 旧 health/app/sell 均 200，近期 stderr 未新增；既有 locales 404 保留。

后台鉴权、库存编辑和商品库存实库回归另记结构化报告。原详情页面/手机压缩和上传控件、客户命令、平台审核、最终报价及付款退回、180 天物理清理和邮件继续待办。本轮不能视为回收闭环或真实 Astro 浏览器联合通过。详见 [报告](buyback-photos-report.json)。
