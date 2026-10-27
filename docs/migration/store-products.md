# 公共商品关系读取

2026-10-28。Gin `/api/store/products` 和 `/api/store/products/{id}` 移除旧 payload、旧 catalog_prices 选价及 ID/handle OR 查询，改用正式关系模型。此批完成商品展示资源的部分能力，T020/T025/T026 和整个迁移尚未验收。

沿用服务端 `STOREFRONT_STORE_ID` 明确绑定及默认销售渠道。仅公开本渠道未删除关联下的 published、未删除商品；店铺/渠道配置缺失或失效返回 503。单个商品跨渠道、草稿、删除、缺失均 404。只按商品原 ID 获取详情；handle 通过列表精确查询。

公开字段仅有 id/title/handle/subtitle/description/thumbnail、仍有效的 collection_id、options、images、variants。商品及变体 metadata、导入 payload、内部状态、修订、成本、渠道及管理字段不返回。metadata 的前台展示键尚待逐键类型投影，不能恢复整包透传。

选项复用后台已验证的商品选项及值归属查询；变体按 variant_rank（null 最后）和 ID 排序，公开 SKU 保持字符串，图片按 rank/ID 排序。变体选择只能来自当前商品有效的选项和值成员关系；缺失、重复维度、外来成员导致 configuration_complete=false，外来值 ID 不公开。变体图片必须同时属于当前商品、有效图片和有效关联。空关系统一 []，不通过库存标志推算可售数量。

列表默认 12 条，limit 1—100、offset 0—100000，按商品 ID 稳定分页，count 在分页前计算。q 对 title/handle 进行文字包含匹配，转义 SQL LIKE 的 %、_、!；handle 精确匹配；collection_id 和 category_id 联合筛选，分类要求 active/非 internal/未删除，集合要求未删除。未知、重复、空、非法编码及非规范分页返回 400；详情仅允许后续[区域选价](store-pricing.md)增加的 region_id/quantity，不接受列表筛选。币种、渠道和客户群不能由客户端覆盖。

同一个只读 Repeatable Read 事务覆盖渠道、计数及全部关系。每商品最多 1000 变体/图片，复用选项原有上限，整个响应的选项、值、图片、变体、选择及变体图片共同限制为 10000 条；超限整体 503，不返回看似完整的截断关系。成功公共重验证；错误 no-store；不读取身份 cookie 或建立会话。

后续已增加[区域与数量选价](store-pricing.md)：显式有效 region_id 时 pricing=true，缺省仍为 false，变体 calculated_price 明确为报价对象或 null；quantity 必须伴随 region_id。availability/checkout 仍为 false。可售数量、购物车、税务结算及 Astro 业务页面尚未完成，此响应不能作为可购买商品实现验收。

[人工 SQL 报告](admin-auth-report.json) 的 store_products 验证双渠道、共享商品、草稿/删除、文字搜索、筛选、ID 详情、选项及图片归属、不完整配置、空数组、关系超限、未实现能力及读取指纹。[固定来源整合报告](catalog-mysql-rehearsal-report.json) 核对所有实际公开商品与变体字段、选项归属、图片、分页和详情；[真实 Gin/browser 报告](admin-live-report.json) 在登录前验证公开接口及非公开商品拒绝，再回归原有 Ant 后台功能。报告中的哈希约束对应实现版本，不代表全前台或完整交易验收。

固定来源本轮实际核对：30 个公开商品、1558 个有效变体、6142 个有效选项选择、424 张商品图片；全部只读接口调用前后完整数据指纹一致。人工 SQL、真实进程、19 项后台 UI 测试/9 张截图、Go test/vet 均通过。旧 /health、/app 和 /dk-en/store 返回 200；现有 storefront 日志中的 Fast Refresh 全量重载告警与本轮前相同，无新增后端错误。
