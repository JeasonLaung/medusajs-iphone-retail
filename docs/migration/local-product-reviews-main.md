# Windows 已付款订单评价、审核与公开展示

2026-11-02，当前恢复库 product_review_runtime 的 reads/writes 均关闭，本人订单评价入口因此503。新增 `configure-local-preview --reviews`：仅既定 development/127.0.0.1:13307/eurorefurb_local_trial 可用，事务锁定runtime并检查版本1，再启用读写；不创建、导入或批准评价，不修改授权。主本机重放两次，主库新账号评价仍0。忽略.env中 `LOCAL_PREVIEW_REVIEWS_ENABLED=true` 由start.bat显式读取，示例默认false；SQL门禁即时生效，无需重启服务。

`python retail/scripts/test-local-customer-account.py --reviews-from-checkpoint=790ce0fc9e71` 从上轮真实Mollie已付未发货订单的检查点建立新副本。新账号原有验证、付款和自动归属来自真实流程；本轮不新建付款或伪造已付。运行器关闭副本结账和付款上下文入口，沿正式客户/审核API验证评价。源十二表计数不变。

owner `3da5020f730b`：390宽客户登录本人账号，从订单详情写五星关联购买评价，首次真实提交201响应丢弃，再同内容/UUID重试；只有一条提交回执。待审评价不公开，新UUID重复提交409。独立1440宽管理员登录实际后台，在同一商品评价区批准；丢成功响应后同命令重试，无重复审核回执。继续拒绝→待审→批准，每步核对公开API：只有批准后可见。最终中英文手机商品页SSR及浏览器均可读标题，正文script被转义且不执行，公开结果不包含私有订单ID/邮箱/account_id，浏览器异常0。

SQL核对一个approved且verified_purchase的新评价、submit回执1、moderate回执4。订单、订单项、付款、capture、交易、库存、预留、设备分配、价格九张保护表的数据导出SHA在整个评价过程保持一致。首次浏览器已通过，但原指纹导出含全实例GTID导致误报；逐表业务INSERT核对一致，改为显式 `--set-gtid-purged=OFF` 后复核通过，没有调整业务约束来通过检查。

副本及授权、专有19100/19200/19300进程、8个Redis键清理完成，忽略目录保留SQL/截图；提交[报告](local-product-reviews-main-report.json)不包含凭据、cookie或付款内容。Go productreviews/customer/配置命令检查和vet、Python/JS/PowerShell语法检查通过，主三端及实际公开评价接口正常。

这是C09评价主流程的当前Windows数据联合，不能替代普通未关联评价、多客户并发、全语言宽度、历史评价导入或完整C01—C11验收。此前独立套件的相关证据仍有效；本次不修改评价UI或原业务规则。
