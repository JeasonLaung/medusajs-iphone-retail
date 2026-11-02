# Gin 商店搜索：接口接入，页面和搜索保真尚待验收

2026-10-29。属于当前交付 C01，承接 [原搜索契约](search-contract.md)。本次只完成原生搜索 API 和数据库专项验证；[正式 Astro 商店页和原组件](astro-store-search.md)已接入，完整保真及英文分词/相关度尚未验收，不能据此宣称搜索闭环或全站完成。

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
- 型号缩写沿用旧前台 `i13pm → iPhone 13 Pro Max`，原输入仍在响应中。关键词采用 AND、末词前缀、Snowball 英文词干及 PostgreSQL 的127项英文停用词；缩写展开后只接受完整词干。来源为已安装 search-postgres 的 `keywordFragments`、`keywordTsQuerySql`。**词法解析器与 ts_rank_cd 相关度仍未达到完整等价验收**，不可用本次词干修复代替整体搜索保真。

2026-11-01：来源8000与当前9300商店同数据浏览器对照记录在 `logs/gin-migration/store-source-compare-1793517102498`。中英×390/768/1440的30款卡片文字、价格、图片地址、顺序及首卡几何一致；扩展搜索复现 `quantities`/`verifying` 来源30款而旧Gin为0、`the` 来源0而旧Gin为30。已以固定 `github.com/blevesearch/snowballstem v0.9.0` 修复这三项，并保留全词AND/末词前缀/缩写全词策略。

独立私有socket MySQL联合见 [搜索专项报告](search-english-native-report.json)：99迁移、30个人工商品，经过实际Gin搜索handler及完整SQL投影的11项HTTP查询全部通过；同轮原商品运营事务回归通过，私有数据库、进程与socket已清理。Go全包、搜索race和catalog vet通过。停用词来自 [PostgreSQL源文件](https://github.com/postgres/postgres/blob/REL_17_STABLE/src/backend/snowball/stopwords/english.stop)，词干依赖使用 [Snowball生成的Go实现](https://github.com/blevesearch/snowballstem)。主数据库和运行主二进制没有升级；修复后的Astro实际浏览器联合、PostgreSQL复杂token及覆盖密度排序继续待验收，整个迁移未完成。首轮夹具误用MySQL保留字rank导致失败，已修正引用后全新环境通过，失败环境也已清理。
- 下一步将原 URL 编解码、搜索/筛选/卡片/移动面板挂到 Astro 正式路由，保留 SSR、返回/前进恢复、空/错/加载状态与原样式，并补运行对照。正式路由及原组件已接入，参见 [Astro 搜索](astro-store-search.md) 的已验证交互和剩余项。

2026-11-01 后续实际浏览器证据：[搜索Astro联合](search-astro-native-report.json)已完成修复后的44组中英×1440/390 SSR/水合验证及桌面搜索输入/清空/后退；运行完全隔离的实际Gin、99迁移MySQL、Redis和构建Astro，无搜索API替身。源代码/二进制/日志SHA已核对，浏览器错误0，截图已人工查看中英桌面/手机。人工商品无SKU/价格，搜索专项不证明报价功能；原运营/合集/推荐联合另在同轮通过。主运行二进制未更新。原PostgreSQL16.15的只读函数记录保存在报告目录source-postgres-lexemes.txt：space-gray同时保留复合词与两个组成词，邮箱及1.5各保留单token；当前Unicode词边界仍不能与这些token等价，覆盖密度排序也继续未验收。

2026-11-01 复杂词元接续：[真实数据库与Astro联合](search-tokens-native-report.json) owner 37384c5731ba，新增29项本机原PostgreSQL16.15只读词元oracle，保留数字字母混合词、小数/科学计数/版本号、邮件地址，以及连字符复合词整体和组成词；英文词干/停用词继续生效。直接Gin handler的21类HTTP查询与中英×1440/390共84组实际构建Astro SSR/水合通过，原Ant运营/合集/推荐回归同轮通过，浏览器错误0，Go全包/搜索race/catalog vet通过。源码、二进制与日志SHA核对完成，独立MySQL/进程/socket/4个Redis键已清理，主库仍86迁移且价格写开关关闭。当前主二进制未更新。

词法oracle可复核：`python3 retail/scripts/capture-search-lexemes.py --output <新的证据文件>`。脚本只连接原商店本机PostgreSQL，在只读事务中调用词法函数，不访问业务表、不输出连接凭据、不覆盖已有证据。固定oracle位于 `retail/backend/internal/catalog/testdata/search-postgres-lexemes.json`。例：`space-gray` 同时要求复合词和组成词，因此不误匹配只有空格的 `space gray`；`6.1` 不拆成 `6` 与 `1`；`phones128` 不错误词干化为 `phone128`。规则参照[PostgreSQL16默认解析器类型](https://www.postgresql.org/docs/16/textsearch-parsers.html)，当前URL/主机/路径/文件及标记/entity类型与更多边界仍需实现和验证，`ts_rank_cd`也未验收，不宣称搜索完整等价或C01–C11完成。人工搜索商品无SKU/价格，本专项不替代报价验收。

2026-11-01 URL/标记接续：[实际Gin/MySQL/Astro联合](search-links-native-report.json) owner ad1de639a832通过34类实际HTTP搜索及中英×1440/390共136组SSR/水合，原Ant运营/合集/推荐回归通过，浏览器错误0。新增55组原PostgreSQL16.15只读oracle，加此前29组共84组，含23种来源词元类型的代表样例。已迁移URL整体/主机/路径重叠词元、文件路径、HTML标签与实体忽略、script/style正文忽略；保留来源URL末尾右括号等已观察行为，而不自行按通用URL库清洗。词法函数核对不访问业务表，固定oracle为 `search-postgres-links.json`，可用 `capture-search-lexemes.py --baseline retail/backend/internal/catalog/testdata/search-postgres-links.json --output <新证据文件>`复核。

此轮888后端/131后台/597前台源快照及二进制/日志SHA核对完成，前台源与上一轮完全相同；Go全包/搜索race/catalog vet通过。测试MySQL、Gin/Astro/Chrome进程/socket及4个Redis键清理完成，7个现有URL200，主库仍86迁移/写开关关闭。主运行二进制未更新，人工商品无SKU/价格。23类代表样例不等于所有解析边界已验收：畸形输入、locale等边界及ts_rank_cd覆盖密度排序继续待办，整个C01–C11未完成。

2026-11-01 相关度前置接续：[词位置验证](search-positions-report.json)新增95组原本机PostgreSQL16.15只读函数对照。Gin英文解析现保留词位置，停用词消耗位置、URL和复合词产生连续重叠词元、标签/实体及script/style忽略行为均与这95组来源一致；`searchTerms`继续由同一解析结果投影，不改变既有命中行为。Go全包、catalog race/vet通过，现有7个服务URL200，主库仍86迁移/价格写开关关闭，后端reload pause已恢复。未重跑数据库/浏览器联合、未更新9300或主Gin；上一轮136组浏览器证据不能宣称覆盖本轮新源码。字段拼接、默认权重与ts_rank_cd覆盖密度排序仍待接入，位置截断/单词出现次数上限及其他解析边界仍缺，不能据此标记搜索或C01–C11完成。

2026-11-01 覆盖密度相关度接续：[原生数据库/Astro联合](search-rank-native-report.json) owner cfc0908424b6通过35类实际HTTP与中英×1440/390共140组SSR/水合（浏览器错误0）。Gin已按已安装来源provider的title(A=1)→description(D=0.1)→option_values(B=0.4)字段顺序拼接词位置，采用默认normalization=0的覆盖密度最小覆盖计算；分数保留float32精度并按降序/ID升序处理平分。264组原本机PostgreSQL16.15只读oracle分数逐位相同，覆盖停用词间隔、重复词/前缀、跨字段权重/偏移、单字段255/拼接256次出现及位置16383饱和边界。固定对照见 `retail/backend/internal/catalog/testdata/search-postgres-rank.json`，复核命令 `python3 retail/scripts/capture-search-rank.py --inputs retail/backend/internal/catalog/testdata/search-postgres-rank.json --output <新证据文件>`；不访问来源业务表、不输出凭据。

原商店默认仍为最新上架；相关度通过原 `sortBy=product` URL显式选择。首轮owner 9b6fc1db3ac0的浏览器排名夹具漏设该参数，因此在比较最新顺序时失败；已按来源路由契约修正测试，未改变生产Astro UI或默认排序，失败环境也已清理。最终893后端/131后台/597前台冻结源与二进制/日志SHA已核对，前台生产源与上一轮相同；四个不同描述词距商品的SSR/水合排名正确，原商品运营/合集/推荐同轮通过。Go全包/catalog race/vet通过；实际查看英文桌面和中文手机截图。所有专用进程/MySQL数据/socket及4个Redis键已清理，7个主URL200，主库仍86迁移/写开关关闭，reload pause已恢复。

本轮只证明当前字段输入下的排名函数及上述联合，不代表全部搜索保真。继续核对发现来源 `search/helpers/option-values.ts` 使用trim后的“选项名:选项值”，当前Gin `searchProjection`仍只投影值，会影响选项搜索和筛选，下一批需修复并用真实关系验证。选项排列与来源同数据对照、畸形解析/locale等边界及C01–C11其余闭环仍待完成。主二进制/9300未更新；人工搜索商品没有SKU/价格，不据此替代报价验收。

2026-11-01 选项搜索投影接续：[原生数据库/Astro联合](search-options-native-report.json) owner ff34b3de0c9f通过49类实际HTTP（含7组限定选项精确筛选）及中英×1440/390共168组SSR/水合，浏览器错误0。Gin `searchOptionValues` 按来源输出去空白后的“选项名:选项值”，保持插入顺序、完整组合去重，同值不同名不合并；原存储/颜色等独立投影不变。8组已实际运行原 `toOptionValues` helper 的纯函数对照覆盖空/重复/同值异名、常规空白、中文、内含冒号及大小写身份；可执行 `node --experimental-strip-types retail/scripts/capture-search-option-values.mjs <新证据文件>` 复核，不访问数据库/凭据/支付。

独立MySQL中两类选项四个关联商品验证选项名搜索、值搜索、组合查询，7组API精确筛选验证组合身份/同组OR/大小写/裸值拒绝/删除值缺席；软删除和未关联的值不进入搜索与分面。Astro实际SSR/水合检查新增7类选项查询，保留原样式，没有新增选项筛选控件；7组精确选项筛选是API证据，不冒称前台按钮交互验收。原运营/合集/推荐同轮通过；Go全包/catalog race/vet通过；源/二进制/日志SHA已核对，专用数据库/进程/socket及4个Redis键清理完成，7个主URL200、主库仍86迁移/写门关闭。主二进制/9300未更新，测试商品无SKU/价格。

继续待办：原图查询给出选项的排列与当前SQL排序同数据核对；组合选项筛选仍沿用191字符输入上限，需按选项名和值的合法长度一并修复；ECMAScript trim与Go TrimSpace的罕见Unicode差异及其他解析边界。上述8组普通投影对照不证明所有Unicode边界或完整搜索等价，C01–C11整体仍未完成。

2026-11-01 选项边界接续：[原生联合](search-option-boundaries-native-report.json) owner 487e26f3ce82通过50类实际HTTP、168组中英双宽度Astro SSR/水合，以及实际Astro公开API桥接的601字符组合值筛选。`option_values`单值上限按现有编辑器title100+分隔符1+value500设为601字符，其余筛选仍191；超过602与其他属性192的请求测试拒绝。投影改用ECMAScript trim集合，FEFF移除、NEL0085和零宽空格保留，避免Go TrimSpace改变源身份；12组实际原helper对照通过。真实MySQL夹具存储带BOM的最长中文名/值，API返回精确去BOM组合，筛选唯一商品04；其余原运营/合集/推荐同轮通过，浏览器错误0，Go全包/catalog race/vet通过，所有专有资源清理完成。当前主URL7项200，主库仍86/门禁关闭，未更新主Gin。来源选项排列和其他解析边界仍待验收。

同一源码随后通过[完整原23组SQL](native-sql-99-search-boundaries-report.json)，1807文件、99迁移及37个专有数据库，二进制/日志/当前源码SHA核对；原生FK开启且网络关闭，专用服务器/数据/socket/Redis已清理。原23组不包含这次搜索浏览器，不能混淆覆盖。主库只读审计与统一更新演练顺序见[本地预览更新计划](local-preview-upgrade-plan.md)，备份恢复和主86→99实际演练尚未完成。
