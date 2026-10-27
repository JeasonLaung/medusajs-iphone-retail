# 商品目录、页面消费与迁移契约

日期：2026-10-27。T002/T006 核对成果及 T007 的目录领域草案，尚未验收或完成实现。后端草稿缺口在下文明确登记，不能把旧页面直接接到草稿后宣称商品闭环完成。

## 源码证据

以 `medusa-store/apps/storefront/src/` 为根：`lib/data/{products,categories,collections,regions}.ts`、`lib/util/{product-variants,get-product-price,get-apple-launch-price,product-option-filters,sort-products}.ts`、`modules/products/components/product-actions/index.tsx`、`app/[countryCode]/(main)/products/[handle]/page.tsx`。商品公开渠道约束另见后端 `src/api/review-product.ts` 与 search middleware。静态清单及页面入口见 [源码追踪](source-traceability.md)、[前台路由](frontend-routes.md)。

## 前台必须继承的行为

| 能力 | 当前实际消费 | 迁移要求 |
| --- | --- | --- |
| 地区 | countryCode 或 regionId 至少一项；先解析 region 后请求产品 | 国家语言 URL 与 region 独立；币种从有效地区确定，不默认为 EUR 后展示所有国家 |
| 列表 | products、count、limit/offset；初始每页 12，可请求 100；分类、集合、handle 与选项值查询 | 过滤后计数与分页稳定，排序加 ID 稳定次序；未知条件拒绝，不能默默忽略 |
| 旧列表 helper 排序 | listProductsWithSort 先取最多 100 条再按最低变体价/created_at 排序，然后切页 | 这是仍需核对调用可达性的 helper，不能误当当前主商城搜索唯一实现；记录其上限缺陷 |
| 当前主商城搜索 | InstantSearch adapter 调 `/store/search`；SSR 先取 facets 和价格分布，客户端再更新结果 | 新 Gin 搜索契约必须承接过滤、facet、区间、统计、排序、query state 和 URL；不能用简单 title LIKE 列表代替 |
| 分类 | handle 可包含 `/`，children、parent、parent.parent 和产品关系 | 树结构、有效/公开限制、排序与父子导航保留；不是简单平面类别数组 |
| 集合 | ID 详情、handle 查询、列表及产品关系 | collection_id 过滤不可等同 category_id；保留原 ID/handle 和旧链接 |
| 选项 | 依 storage→condition→battery→color/colour 排序，其他选项保留 | 每个 product 的 option/value 归属必须正确；同名不能跨产品共享错误选项值 |
| 完整变体 | 缺少任一产品选项值的 variant 不参与购买，页面提示部分配置待补 | 保留不完整数据用于后台修复，不能为凑 SKU 自动虚构选项 |
| 可售 | !manage_inventory 或 allow_backorder 或 inventory_quantity>0 | 原数量库存与 required_quantity 联动；设备档案数不能代替可售数量；加购仍服务端校验 |
| 选项联动 | 前置选项约束后置；改变当前选项会选择有货且尽量保留其余选项的组合 | 不能只按全部当前选项过滤，导致其他电池/颜色永远不可达；匹配分数相同保留稳定顺序 |
| 变体 URL | variant.metadata.slug 可进入 `/products/iphone-…-S85` 等；旧 v_id 仍可定位 | 服务端从 slug 解析原 model handle 后查到真实 variant；不存在的组合 404，不能默默选默认 |
| 选择优先级 | 路径 slug → v_id → 初始 ID；无请求则首个有货完整变体，再首个完整变体 | 保留 URL 深链、刷新/后退、图片与初始选中项一致 |
| 切换配置 | history.replaceState 更新 slug 或 v_id；同步 title/canonical，不重新获取整页 | Astro React 岛保持同一可观察行为；保留其他 query 与 hash |
| 图片 | product images 与 variant images/thumbnail，颜色的本地缩略图后备 | 保留排序、关联、公共资源映射及无图回退，不把全局第一张图片用于所有配置 |
| 价格 | 当前 getPricesForVariant 仅接受 number；original/calculated/currency/sale 类型 | Gin 精确金额输出字符串，新 Astro adapter 显式处理；不能直接把字符串接旧组件导致全部“价格待定” |
| 加购 | 当前已购数量参与限购；加载、已加入、库存不足、通用错误分别反馈，成功跳购物车 | 页面按钮与 sticky/mobile bar 同步，服务端复核价格库存；不以截图为加购验证 |
| 产品扩展 | card_colors、apple_model_specs、apple_launch_prices、source；variant slug/demo_price_basis/apple_launch_price | 公开 metadata 使用逐键和嵌套结构白名单，保持实际文案/颜色/规格/上市价；其余源 metadata 留内部，不整包公开 |

## 字段与关联去向

以下为语义映射设计。正式表名和 DDL 在 T011/T020 实施前固定；当前 SQL 草稿只实现其中一部分。

| 源表/字段组 | 目标领域与转换 | 校验 |
| --- | --- | --- |
| product 基础、handle、status、thumbnail、collection/type、排序、尺寸、产地、时间/软删 | catalog 商品主记录，完整保留原 ID；商品属性不得因前台暂不展示而丢弃 | handle 大小写/历史软删除唯一性明确；删除产品不出公开 DTO |
| product_variant 基础、product_id、sku、barcode/ean/upc、title、rank、inventory/backorder、metadata、时间/软删 | catalog SKU 主记录 + 内部扩展；ERP 号保持原值 | 原 product 关联存在；内部物流/条码字段按接口授权选择 |
| product_option、product_product_option | 选项定义和商品选项成员关系分别保留 | 不以 option 表的名称推断 product 归属；软删 join 不生效 |
| product_option_value、product_product_option_value | 值定义与商品可选值关系分别保留 | 不能把同一个 option 下所有 value 自动赋给每个 product |
| product_variant_option | SKU 所选 option_value_id，与 option/product 成员共同校验 | 每 SKU/option 唯一；没有值、已删除值、跨产品值报差异，不能静默补齐 |
| image、product_variant_product_image | product 图片与 SKU 图片关联，rank、ID、软删和公共 URL 映射 | 变体图片属于相应商品；保留原排序，已删图片不出公开 DTO |
| product_category、product_category_product | 分类树、active/internal、rank/path 及商品关联 | 公开只给有效非内部分类；孤儿/循环在导入阶段报告 |
| product_collection、product_type、product_tag、product_tags | 集合、类型、标签及关联 | 关系完整且软删除语义明确；实际标签参与页面消费不能遗漏 |
| product_sales_channel、sales_channel、store/default channel | 独立渠道及商品成员 | published 但不属于当前店铺渠道也不得公开；不能只检查 status |
| product_variant_inventory_item、inventory_item/level、reservation_item | 库存域及每 SKU required_quantity | 价格导入/目录接口不能伪造库存；保留数量精度与本仓裁剪 |
| product_variant_price_set、price_set、price、price_rule | 价格集合、共享关联、金额及规则 | 共享价格不复制为各 SKU 无关记录后丢失后续联动；价格源 ID 保留 |
| price_list、price_list_rule.value、price_preference | 价目表生效区间/状态/类型、规则、含税偏好 | 当前源库无 price_list_rule_value 独立表；规则 value 按真实 JSON 结构解析；按区域、币种、数量、客户群匹配，未知规则不能套最低价 |
| region、region_country、store_currency | 地区、国家、币种/默认币种、automatic_taxes | country_code 映射按已配置地区；与前端语言分开 |
| metadata/raw JSON | 保留内部源证据，逐字段迁入可编辑模型 | 保留不等于公开；JSON 数字解析 UseNumber/精确十进制，19 位编号不能经 float64 |

原始 metadata 不能作为永久主数据模型替代关系表；修改业务字段后必须只有一个权威值，避免查询列和 payload 中出现两个不同的 title/status。

## Gin 目录 API 草案

新应用使用 `/api/store` 与 `/api/admin`，不依赖 Medusa SDK。公开店铺/渠道来自服务端配置或可信站点上下文，不能让任意客户端通过传入 channel_id 看见其他目录。URL/浏览器支付返回兼容入口保留旧路径，应用内部 API 无需保留框架私有 fields 语法。

| 新接口 | 入参 / 返回 | 权限 |
| --- | --- | --- |
| GET /api/store/regions | regions、countries、currency_code、automatic_taxes；分页或明确小集合边界 | 公开 DTO |
| GET /api/store/products | region_id、q、handle、collection_id、category_id、option_value_id[]、sort、limit/offset；products/count/limit/offset | published + 当前店铺渠道 |
| GET /api/store/products/{id} | 原商品 ID；地区上下文；product | 同列表可见性；URL handle/variant slug 由 Astro 先解析并验证 |
| GET /api/store/product-categories | handle、parent_category_id、树展开有界、limit/offset | active、非 internal、软删过滤 |
| GET /api/store/collections | handle、limit/offset；collections/count | 只返回本店可用关联；详情能力需补齐 |
| GET /api/store/collections/{id} | collection，所需公开摘要关系 | 与集合列表相同 |
| GET /api/admin/products[/{id}] | 内部商品、SKU、选项、图片、价格摘要；有界分页/关系选择 | product read + 数据范围/字段裁剪 |
| 后台目录写操作 | 商品、SKU、选项、图、分类、集合、价目表分命令 | 对应 write；审计、并发版本冲突；后续完整 schema 定义，不把 title/status 修改视作全量管理 |

金额以主单位十进制字符串传输，例如 `"919.00"`，币种为小写 ISO code；未知价格为 null，禁止以 0 混淆未报价。表示价税方式的字段必须明确。数组空态为 `[]`；计数是应用过滤后总数；稳定排序使用业务排序键及原 ID。所有时间为 UTC RFC3339，标识符始终字符串。

错误结构、HTTP 状态、CSRF、两类 cookie 和幂等规则由整体 T007 契约统一，不能每个领域自行发明。路径中 handle 与原 ID 不应共用模糊 OR 查询：保留独立解析，避免碰撞时读错商品。

## 已发现的后端草稿缺口

1. `internal/catalog/http.go` 公开列表/详情仍直接 Decode 原 payload，未实现公开字段白名单或渠道过滤；不可作为正式公开 API。2026-10-28后台列表及[详情/分页变体](admin-product-detail.md)已改为独立关系投影；公开接口/更新不据此完成。
2. 导入器未检查 product_product_option_value 关联、图片软删除/排序及 SKU 图片关联；部分分类/集合/地区关系缺失。
3. 当前 calculated_price 仅尝试无价目表/阶梯的基本价，不构成完整价格引擎；草稿字符串不能直接传给要求 number 的旧 helper。
4. UpdateProduct 在 GORM Updates 后读取 p 作为 before，可能已被 GORM 更新；需加行锁或版本控制，提前保存 before 后再审计。
5. 导入空库检查与 hash 查询缺少并发导入互斥；缺失/错误时间被默认 epoch，数字/JSON 解码过宽松，不能静默接受损坏输入。
6. schema 中历史软删 handle 的全表唯一性、共享 price_set 及 ID 语义仍待按源约束核对；不能借导入生成新 ID 后丢失源证据。

这些缺口纳入 T010/T012/T018/T020/T021/T026；本轮只固定契约，没有在未完成阶段门槛前继续扩写业务草稿。

## 固定用例规格

C01 商品发布但渠道不匹配不返回；C02 软删 product/variant/option/value/join/image 不出公开数据；C03 同 option 多 product 的 value 不串；C04 SKU 图片按原关联和排序显示；C05 不完整 SKU 保留后台、禁止购买；C06 前置选项变化可重选后置且 URL/价格/图片同步；C07 slug/v_id 深链及无效组合 404；C08 缺价 null 与真零价区分；C09 同区域/数量/客户条件选价及未知规则拒绝；C10 列表过滤、分页、排序稳定；C11 仓库嵌套库存不能跨范围；C12 并发目录更新审计 before/after 准确，重复导入不覆盖后台修改。均待新系统实现验证。

2026-10-28 区域接口已由[公共关系 DTO](store-regions.md)替换 payload 并补单条详情；固定 1 区域/7 国家核对与 Astro SSR 串联通过。公共商品、分类、集合、渠道、价格和页面仍按上述完整契约推进。

2026-10-28 [公共分类/集合及当前店铺渠道](store-taxonomy.md)已替换对应草稿，公开 DTO 使用 product_count 承接数量消费；商品页/卡片仍待公共商品及选价实现。

2026-10-28 [公共商品关系读取](store-products.md)已移除上文第 1/3 项涉及的公开 payload/旧选价草稿，按正式渠道、商品、变体、选项和图片关系投影；区域/阶梯/条件选价尚未实现，明确不返回 calculated_price，capabilities 中 pricing/availability/checkout 均为 false。此阶段不支持 region_id/option_value_id[]/sort，不能把上述目标契约视为当前全部可用。逐键 metadata、可售和前台页面仍继续迁移。

2026-10-28 公共选价补充：[Gin 精确区域/数量选价](store-pricing.md)增加显式地区、店铺币种、数量阶梯、规则具体度、sale/override 原价、含税偏好及有界集合读取；未知条件/不完整变体不报价。固定来源两个上下文与旧服务核对，人工 SQL 覆盖价目表及失败边界。availability/checkout 仍关闭；客户身份、完整税/优惠/配送交易、可售、metadata 和 Astro 页面继续推进，T020/T025/T026 未整体验收。

2026-10-28 商品展示补充：[Gin/Astro 明确展示扩展](store-display.md)承接当前实际颜色、消费规格、来源提示、上市价与变体 slug/示例提示；私有 metadata 不透传，同一只读快照按可见 ID 先检查 64 KiB 单条/4 MiB 合并预算。人工 SQL、独立固定来源和 Astro 11 项用例跟踪；完整编辑、深链/React 选择、实际页面和交易仍待完成，相关任务未整体验收。

2026-10-28 Astro 选择与深链补充：[原配置联动及页面数据组合](astro-product-selection.md)保留选项顺序/前置约束/稳定评分、slug/v_id 优先级、图库回退及 query/hash；可售按 Gin 冻结/单仓观察处理。请求内国家归属、精确模型 handle 和组合匹配，未知组合不默认、依赖失败不伪装 404。320 组旧纯函数比较及 16 项 Astro 单元、真实源模块集成报告跟踪；实际 DOM/CSS/React 岛/History/加购仍待完成，相关任务未整体验收。
