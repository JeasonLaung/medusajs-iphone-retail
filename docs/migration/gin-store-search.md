# Gin 商店搜索：接口接入，页面和搜索保真尚待验收

2026-10-29。属于当前交付 C01，承接 [原搜索契约](search-contract.md)。本次只完成原生搜索 API 和数据库专项验证；正式 Astro 商店页、原搜索组件及英文分词/相关度尚未验收，不能据此宣称搜索闭环或全站完成。

## 请求与响应

`POST /api/store/search` 为匿名只读请求，不接收渠道、店铺、币种或任意排序字段。服务端绑定 `STOREFRONT_STORE_ID`，根据有效地区和该店铺允许的币种选择价格。请求 Cookie 不用于授权或报价，响应不设置 Cookie，`Cache-Control: no-store`。

```json
{
  "region_id": "region-id",
  "queries": [{
    "key": "main",
    "query": "i13pm",
    "filters": {"storage_options": ["128 GB", "256 GB"]},
    "sort": "created_at:desc",
    "page": 1,
    "limit": 50,
    "min_price": "100.01",
    "max_price": "900"
  }]
}
```

`results` 数组保持请求顺序和 key，包含原 query、`currency_code`、hits、过滤后总数、分页、`facets`、`catalog_facets`、`price_distribution`。所有金额均为精确十进制字符串，缺价为 null，零价为 `"0"`。hit 的 price 含 min/max、与最低报价同一变体的 original，以及明确的 on_sale；折扣参考价不跨变体拼接。

筛选属性白名单：category、model、storage_options、battery_options、display_size、color_families、rear_cameras、condition_options、release_year、labels、option_values。同组 OR、跨组 AND；商店的固定 `category:iPhone` 条件始终生效。分面计数排除自身属性的选中条件，保留其他条件和价格范围；目录分面不受当前 query/refinements 限制；价格分布排除价格范围自身且在分页前计算，保留 query 和其他筛选条件。分布提供完整原始金额序列供原图表分箱，重复价格按商品数量保留。

sort 仅允许 `created_at:desc`、`relevance`、`price:asc`、`price:desc`、`title:asc`、`title:desc`；缺省按创建时间降序，价格排序始终将缺价排在末尾，同值按产品 ID 确保稳定顺序。请求上限 64 KiB、1—8 个不同 key 子请求、query 256 字符、key 64 字符、每属性 1—32 个不重复值、值 191 字符、每页 1—100（缺省 50）、页码 1—2000（缺省 1）。未知字段、重复 JSON 键、非法金额/倒置范围、URL query 参数均拒绝。

## 数据与一致性

批量请求在同一个 repeatable-read 只读事务中读取当前渠道的未删除、已发布商品。商品关系、选项完整性、图片归属、公开 metadata 投影沿用详情 API；地区报价和库存读取提取为共享函数，避免搜索维护另一套价格/库存业务。

卡片储存容量和颜色的现货标签仅来自完整配置且实时可售数量大于零的变体。型号 metadata、允许 backorder 或不管理库存不能制造现货。真实 SKU 提供可见的型号容量参考；筛选容量来自真实变体关系。颜色家族继承原项目颜色别名；公开字段采用明确 DTO，不暴露原始 metadata、价格规则、仓库或私有渠道关系。

当前选择实时 SQL 快照读取，无独立索引、Redis 缓存或重建任务，因此本路径不受旧索引事件缺失影响。这替代原契约草案中的 outbox 索引方案；若后续引入异步索引，须重新满足可靠重建和实时价格/库存校验。当前目录上限 1000 商品；超限及关系、metadata、价格或库存读取达到既有安全边界时整体 503，不返回伪装完整的第一页分面。此为实现资源限制，不代表已完成大目录性能验收。

## 已验证与剩余项

- Go 专项覆盖筛选 OR/AND、排除自身条件的分面、分页前价格分布、范围边界、超过 JavaScript 安全整数精度的价格排序、零价和缺价、固定类别隔离及输入/失败边界。
- 隔离 MySQL + Redis 经实际 Gin router 验证批量结果顺序、当前渠道/发布/软删除限制、地区币种、搜索与详情价格一致、实时库存减少后现货标签消失、backorder/unmanaged 不制造现货，以及禁用渠道拒绝访问。测试不操作旧业务库。
- 型号缩写沿用旧前台 `i13pm → iPhone 13 Pro Max`，原输入仍在响应中。当前关键词为词边界 AND、末词前缀和字段权重原型；**不等同于 PostgreSQL English stemming / ts_rank_cd**。来源为已安装 search-postgres 的 `keywordFragments`、`keywordTsQuerySql`；其默认 English 配置、词干、停用词和覆盖密度排序仍需对照。中文行为、相关度排序及拼写边界也未通过保真验收。
- 下一步将原 URL 编解码、搜索/筛选/卡片/移动面板挂到 Astro 正式路由，保留 SSR、返回/前进恢复、空/错/加载状态与原样式，并补运行对照。当前 `/[countryCode]/store` 仍未交付；没有用一个新简版列表页替代原页面。
