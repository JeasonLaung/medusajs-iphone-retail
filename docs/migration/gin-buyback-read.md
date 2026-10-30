# 新账号回收列表与详情读取

2026-10-30。新增受保护 GET `/api/store/customer/buyback/requests` 与 GET `/api/store/customer/buyback/requests/:id`。沿用客户会话锁后复核 Redis、已验证/启用及 auth generation。只认 `customer_buyback_submissions` 的账号/店铺显式绑定，原申请 customer_id 也必须一致；历史邮箱或 ID 相同不授予权限。未拥有、软删除详情返回相同 404。

详情读取当前申请，不能使用提交时的不可变回执替代。原联系/原路退回地址、设备项、当前报价、物流、收付款备注、审核说明、原历史 note/photo_id、创建/更新/关闭/删除时间及 180 天保留期均承接。金额沿用已有原始精度读取器，损坏 raw 不降级成标量。

0075 新增原 BuybackPhoto 记录字段和软删/状态约束。读取只投影公开照片元数据，不查询 storage_key；按原 oldest-first Map replacement 返回每个 item/type 最新版本，时间相同按生成 ID 取最新，替换保留原分组顺序。该表尚无上传/图片读取生命周期。

列表每页 20、创建时间倒序并以 ID 稳定排序；同一事务读取总数、详情和全账户汇总。汇总流式遍历全部有效归属申请，paid 累加手机数量及分币种到账，其余除 returned/canceled 累加处理中，金额保留原逐笔 Number.EPSILON + roundPrice 的语义，不按当前页汇总。仅允许 canonical offset 0..100000。

Astro customer bridge 加入固定 GET 列表/详情及规范分页，沿用签名和客户 cookie 白名单、私有 no-store，拒绝额外查询/嵌套路径/不正确成功状态/cookie 变更；公开详情/列表响应上限与已有订单读取一致为 4 MiB。前台组件尚未改成调用这些读取接口。

## 验证

在隔离源码快照执行 74 个正式迁移，不包含另行未提交的 0037 Mollie 工作；不触碰旧开发数据库。

- ProductPrices 真实 MySQL/Redis/Gin 套件：创建基础回归、当前详情、跨账号/店铺/历史 ID 拒绝、损坏 raw、精确 final amount、照片版本/时间并列/软删、公共投影、会话退出/账号禁用、GET 来源/查询/匿名拒绝、只读不依赖写开关。
- 增加 23 个实际创建的人工申请，验证 20 条分页/不重复、全局汇总、paid 两个币种、returned/canceled 排除、超过页尾仍保留汇总，以及全部严格分页 HTTP。
- Go `go test ./...` 通过；后台鉴权、InventoryStockEdits、ProductInventory 三组真实 SQL 回归通过。
- 前端 166 项 Node 测试、Astro 357 文件 0 错误 0 警告（5 hints）通过。
- 旧 health/app/sell 均为 200，近期 stderr 未新增；已存在 `/store/locales` 404 保留。

状态和照片记录为隔离夹具，不能视为审核/付款/上传已实现。本轮没有 Astro→真实 Gin 浏览器联合，也没有原列表/详情页面验收；后续继续接原页面、照片、客户命令、审核检测、最终报价、付款退回及邮件。详见 [结构化报告](buyback-read-report.json)。
