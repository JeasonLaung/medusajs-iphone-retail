# 设备照片与带照片入库

本批延续整个现有前台闭环的交付范围，只补设备入库依赖；不改旧前台页面，也不表示配货、回收或整个迁移已经完成。

React 入库表单使用 Ant Upload，支持选图、相机、预览和移除，每台最多8张。沿用原组件的等比例1800px、白底、JPEG质量0.85处理。上传期间或结果不明时阻止提交、移除行、重读和关闭；可用原操作ID及相同内容重试，也可明确放弃未确认的照片。达到8张后仍可移除照片。

`POST /api/admin/device-photos` 每次保存一张，要求 operation_id、filename、mime_type、content 四个显式字段。继承登录、Origin、CSRF、私有响应及 device.read/device.intake/product.read/inventory.read/warehouse.read 明确权限；数据库事务内重查当前权限和设备运行门控，撤权后旧收据也拒绝。仓库员工仍需后续专用入口。

JPEG/PNG/WebP 必须匹配实际文件头，解码后最多3MiB；拒绝非规范Base64、重复/未知字段、null、额外JSON和查询参数。核验与原来源一样属于文件签名核对，并非完整图像解码。仅该POST路由的请求上限放宽到4MiB+8KiB，其余仍为1MiB。客户端将多张图片逐张上传，避免把8张Base64同时放入一个请求。

0059迁移新增 device_photos。字节、媒体类型、SHA256、上传者和唯一操作ID同一事务保存于MySQL；同操作同内容恢复原ID，不同内容或不同主体返回冲突。当前以受限MEDIUMBLOB实现独立运行，备份会包含图片；文件对象存储迁移及未关联上传的定期清理尚未提供。

返回服务器生成的 `dphoto_UUIDv4` 和固定同源 `/api/store/device-photos/:photo_id`。公开设备照片符合原上传的public语义，不能用于回收私有照片或客户身份文件。GET核对文件头、长度、SHA256后返回固定媒体类型、nosniff、ETag及不可变缓存；不存在或损坏时不缓存为成功图片。

入库仅接受匹配的ID/URL，不接受任意远程URL；每台拒绝重复引用和超过8张。提交时重新核对真实保存字节，设备、入库事件、成功收据保留照片数组；近期50台的读取保留图片，刷新可见。照片预上传独立于入库，因此号码冲突导致整批回滚时文件仍保留，可修改批次重试。

隔离验证包含58项迁移、Go全包、SQL/HTTP、Ant类型检查与构建、19项原Admin浏览器回归。SQL覆盖4个并发请求仅一份文件、同请求重放、撤权、错误MIME/超限/非规范Base64、伪造引用、存储损坏、带照片入库/刷新与库存快照不变。合成API浏览器验证上传丢响应后原请求恢复及待确认照片阻止提交；不将其当作真实Gin证据。

真实联合命令：`python retail/scripts/test-warehouse-inventory-joint.py --device-intake`。真实浏览器登录→新建来源→开启SKU追踪→上传PNG并核对服务器返回JPEG→整批销售/退回入库→刷新照片仍可见→中英/390宽度。MySQL回查要求2台设备、2次intake事件、1份入库收据、4个身份占用、1份照片和1个设备照片引用，精确数量库存与原值一致。容器和临时文件清理后才生成报告。

证据：device-photos-report.json、device-intake-joint-report.json。历史文件引用迁移、跨SKU导入、移动入库和仓库员工设备操作、完整设备管理、配货发货及前台履约仍须继续。
