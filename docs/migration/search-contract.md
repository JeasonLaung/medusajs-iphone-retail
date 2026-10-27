# 搜索、筛选和商品卡片契约

日期：2026-10-27。T002/T007/T022 的源码基线与新接口草案；尚未实现 Gin 搜索。

## 当前实际路径

主目录 `storefront/src/modules/store/templates/index.tsx` 使用 InstantSearchNext，默认每页 **50** 个 hit，固定 `category:iPhone`，共享 navbar search client。不是旧 listProductsWithSort 的每页 12 / 最多 100 条路径。当前 `getStoreSearchData` 在 SSR 取全目录 facet 选项和符合 query/refinements 的价格分布，再由客户端继续查询。相关代码均在 `medusa-store/apps/`：

- 后端 `backend/src/search/product.ts`、`search/helpers/{pricing,stocked-options,card-sku-options,color-families}.ts`。
- 前台 `storefront/src/lib/{search-client,data/store-search}.ts`、`lib/util/{store-search-routing,product-search-query,product-card-stock}.ts`。
- UI `modules/store/components/store-refinements/` 和 `store-hits/`；查询适配器的批量/不同 facet 语义仍需运行采集核对。

## URL 与查询保真

| 浏览器 query | 原属性 | 保留规则 |
| --- | --- | --- |
| q | query | 用户原输入留在 UI/URL；仅检索时标准化 i13pm→iPhone 13 Pro Max 等缩写 |
| series | category | 固定 iPhone 类别条件仍有效，多选系列 |
| model | model | 模型过滤保留兼容，即使当前可见控件用 series |
| storage / battery / display | storage_options / battery_options / display_size | 数组值按原顺序编码、可恢复 |
| color / camera / condition / year | color_families / rear_cameras / condition_options / release_year | 与原显示值匹配，不以内部 ID 替换共享链接的文案值 |
| labels / options | labels / option_values | 保留已有链接兼容 |
| sortBy | index/排序表达式 | 默认 product/sort/created_at:desc；product 表示相关度，其他排序依现有配置白名单 |
| page | 页码 | 正整数；默认 1，<=1/无效不保留无效 UI state |
| range / toggle | 嵌套状态 | 价格区间及已有状态链接继续可读 |
| product[...] | 旧 InstantSearch UI state | 旧链接兼容解析，不能忽略或把嵌套对象当普通字符串 |

当前数组通过逗号分隔，值内 `%`、`,` 先分别转 `%25`、`%2C` 再交给 query serializer；解析按相反语义恢复。Astro 不能用简单 split/decode 顺序导致颜色名等值被拆坏。清空筛选同时重置搜索和排序到默认；移动筛选/排序面板与桌面共用 query state，回退恢复。

标准化识别 i/iphone + 1—2 位型号，后缀 p/pro、pm/pro max/promax、plus、mini、e；发生扩展时 match_strategy 改 all，保持原输入；其余请求默认 last。不能只用前缀 LIKE 充当相同搜索体验。实际分词、中文、相关度权重和拼写边界纳入对照用例，尚未验收。

## 筛选、价格与库存语义

- 搜索文档包括 category、labels、option_values、model、真实 SKU 的 storage_options、color families/hexes、battery/condition、release year、display inches、rear camera count 及币种价格字段。status/sales_channel_ids 用于过滤但不向 hit 暴露。
- title 权重 3、option_values 权重 2、description 权重 1。筛选同组 OR、跨组 AND 的具体运行查询及 disjunctive facet 计数需采集确认；新 API 明确表达，不向客户端开放任意 SQL/搜索字段。
- 库存卡片必须是**完整配置且可售数量为正**。后端索引按渠道关联仓库计算，前台对每批最多 100 个产品再读实时库存覆盖 stocked_storage_options 和 stocked_color_options，避免索引延迟。没有 SKU、只有型号 metadata 或 allow_backorder 不等于有实物库存。
- 这与商品详情允许 backorder/unmanaged 的购买规则有区别；不能把“可下单”和“有现货标签”合并。
- 当前索引只有 EUR/USD；未知币种降级读取 EUR 是既有风险。新接口返回明确 currency，不能把 EUR 数字标成别的币种；真实地区价格与索引币种差异须对账。
- 每币种 min/original 来自同一最低价 variant，max 为各 variant 最大 calculated；on_sale 明确 false，与缺少价格不同。不能把一个 variant 的 original 拼到另一个 calculated 制造虚假折扣。
- 索引价格上下文当前只有 currency，不含 region/customer group；列表和详情可能因此不同。新价格域须统一上下文并列出对照差异，不能继续把索引数字当最终成交价。
- SSR 价格分布查询会每批 100 条继续到末页，不是只取第一页。分布 key 不含价格区间本身，需维持拖动区间时的直方图上下文；实际 widget 算法待细核。
- 产品/库存/预留事件触发重索引；价目表定时边界、部分链接及直接 upsert workflow 无事件是源码 TODO。Gin 使用持久 outbox + 定时有效期检查；Redis 缓存失效不能丢掉重建任务。

## 新接口边界草案

`POST /api/store/search` 是只读批量搜索。输入仅允许已定义 query、region、过滤属性/排序、分页和 facet 请求；每个子请求有稳定 key，响应结果顺序与请求一致。认证不是公开产品查询必要条件，但店铺/渠道可见性始终由服务端限制。客户私有条件不混入公共缓存。

结果需有公开 hits、过滤后 count、分页、facet 值及计数、价格 min/max 与 histogram 数据、query/currency。Decimal 金额传字符串；原 InstantSearch UI 如保留，使用独立 adapter 显式转换、限制展示精度，不把 Medusa adapter 留在最终运行依赖中。

服务端限制批量子请求数、每页上限、query/数组长度、facet 值数和展开深度；所有过滤参数化。具体上限在 OpenAPI 中固定后实现，不能假装当前草稿 GET products 的简单分页已经覆盖。

## 对照用例

S01 空 query 首屏 50 条/分页；S02 i13pm 等缩写且 URL 保留原词；S03 中英、多选跨组与单组；S04 逗号/% 字符和旧嵌套链接往返；S05 价格排序/区间/histogram 全数据；S06 无价与零价/on_sale=false；S07 无货 SKU、backorder、非完整配置的卡片标签；S08 预留后实时库存、索引滞后/更新恢复；S09 下架/跨渠道数据不从 hit/facet/count 泄漏；S10 首屏 SSR 与客户端水合一致；S11 手机面板清空/切换/后退；S12 目录价和详情价在同上下文一致。均待运行验收。
