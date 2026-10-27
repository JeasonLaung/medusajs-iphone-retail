# 公共分类、集合与店铺渠道

2026-10-28。Gin `/api/store/product-categories`、`/api/store/product-categories/{id}`、`/api/store/collections` 和 `/api/store/collections/{id}` 改用正式关系模型及明确公开 DTO。旧 payload 读取移除。对应 T020/T025/T026 的部分能力，公开商品与完整前台仍未完成。

## 店铺及可见性

服务端环境 `STOREFRONT_STORE_ID` 明确绑定已有店铺，读取其当前有效 default_sales_channel_id；店铺/渠道删除、渠道禁用、缺失配置或模型异常均 503。请求不能传店铺、渠道或 Medusa fields 覆盖范围。没有默认选择第一条店铺或第一条渠道。

分类仅返回未删除、active、非 internal 的记录。公开 parent_category_id、递归 parent_category 和直接 category_children 构成导航，父级/子级同样受可见性过滤；隐藏或已删除父级不通过子级泄露 ID/内容，子级保留独立公开信息。原持久关系不变。祖先循环、超过 16 层祖先、完整有效目录超过 1000 个节点，整体 503；不能按当前分页只装载半棵树。主列表及子级以 rank/id 排序。

集合要求未删除且至少关联一个本店渠道的已发布有效商品；空集合、仅草稿/已删除/其他渠道商品的集合不公开，详情 404。分类保留有效空导航，但 product_count 仅计算本店渠道已发布商品。两个统计均排除已删除商品/渠道关联；同商品属于其他渠道不会额外计数。

Category 明确公开 id/name/description/handle/rank/parent_category_id/product_count，以及受控祖先/直接子级；Collection 仅公开 id/title/handle/product_count。内部 metadata、external_id、mpath、状态标记、渠道 ID、时间和原 payload 不返回。前台 adapter 应以 product_count 承接旧 `.products.length` 消费，不生成虚假商品数组；商品卡片仍由后续正式公共商品接口提供。

## 查询及一致性

列表接受 limit（1—100，默认 100）、offset（0—100000）与精确 handle，分类另接受 parent_category_id，其中字符串 null 选择公开根节点。斜杠 handle 作为 query 处理。计数在筛选后、分页前计算，空页数组为 []；父级和子级附加关系不重新分页。详情只按原 ID 读取，无 ID/handle 模糊 OR。

未知/重复/空/非法 UTF-8/控制符/损坏 query、非规范分页及详情附加筛选均 400。只读 Repeatable Read 覆盖店铺绑定、目录和计数，正式模型不就绪时关闭读取；成功公共重验证，错误 no-store。不读取用户 cookie 或生成会话。

## 验证和剩余

[SQL 联合报告](admin-auth-report.json) 的 store_taxonomy 验证实际 Gin 路由下的当前渠道计数、两渠道共享商品、草稿/删除过滤、隐藏父级、两级祖先、直接子级、斜杠 handle、稳定分页、空页、详情、query 拒绝、配置缺失、禁用渠道/删除店铺、循环/尺寸/深度边界和读取前后指纹。使用独占可销毁人工 MySQL fixture。

[固定数据整合报告](catalog-mysql-rehearsal-report.json) 比较全部实际公开分类字段、当前渠道商品计数和集合数量；测试 owner 显式使用固定快照内已验证的单个店铺，不根据请求推断店铺。[真实 Gin/browser 报告](admin-live-report.json) 在登录前校验完整公开 DTO，并回归既有后台及 Astro SSR。旧开发服务及真实数据未改。

公共商品/变体/图片、逐键 metadata、区域/数量选价、数量可售、搜索 facets、Astro 页面/CSS/字体/交互及分类/集合后台编辑仍待实现。当前公共商品草稿不因本次渠道组件完成而变成合格实现，不能据此切流。
