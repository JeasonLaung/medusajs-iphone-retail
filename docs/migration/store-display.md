# 商品展示扩展与深链字段

2026-10-28。Gin 商品列表/原 ID 详情新增商品与变体 `display`，Astro 同步接入明确类型。来源是已导入正式 catalog_products/catalog_variants 的当前 metadata 列，仍以完整原证据保留内部字段；公开接口不返回 metadata、payload 或任意键值。此批是读取投影，未新增通用 metadata 写入口，不代表完整商品编辑或 Astro 页面已验收。

## 实际消费与明确映射

核对旧 storefront 的 product-preview、product-detail、product-tabs、product-actions、product-variants 和 get-apple-launch-price，保留如下消费：

| 来源 | 新 DTO | 规则 |
| --- | --- | --- |
| product.card_colors | display.colors | 按来源顺序保留 name/hex；十六进制六位颜色，其他嵌套键不公开 |
| product.apple_model_specs 或根级规格回退 | display.model_specs | 只给旧组件实际读取的 15 个规格标量、单独校验的链接及 apple_model_numbers；标量统一精确字符串，空可选字段不输出 |
| specs.tech_specs_url | display.model_specs.tech_specs_url | HTTPS 且确切 apple.com/support.apple.com 主机，无账号/端口，保留官方规格链接 |
| product.apple_launch_prices | display.launch_prices | 存储容量原值、正金额及小写币种；明确数组，容量键稳定排序，不透传对象 |
| product.source | display.source | 仅 swappie-html-snapshot，承接原图片来源提示；其他来源文本和 URL 不公开 |
| variant.slug | display.slug | 原大小写/文本；沿用 iphone-*-[SA](85或100) 语法和有界长度，不生成不存在的组合 |
| variant.demo_price_basis | display.demo_price_basis | 仅 model-starting-price，承接原示例价格提示 |
| variant.apple_launch_price | display.apple_launch_price | 独立精确正金额及币种，不参与交易选价 |

标量规格具体为 release_year、display_size_inches、display_name、display_resolution_px、pixel_density_ppi、chip、rear_camera_system、rear_camera_count、front_camera、height_mm、width_mm、depth_mm、weight_g、connector、biometric；tech_specs_url 为单独校验的链接。nested specs 是对象时使用该对象，即使为空，也不与根级混合；不存在或不是对象时才使用根级已审查规格。

SQL NULL/JSON null 输出 colors=[]、launch_prices=[]、model_specs/source=null，变体三项为 null。继承原类型守卫：错误的可选颜色、标量、slug、价格及非允许枚举被忽略，不改写来源；嵌套未知字段不公开。数字金额仅接受来源 JSON 数字，输出主单位字符串，不转浮点或除以 100；字符串型原金额仍按旧守卫忽略。catalog 的 JSON 列为 LONGTEXT 加 JSON_VALID CHECK，避免 MySQL 原生 JSON 数字转 double；独占 SQL 验证大整数及 18 位小数直接从该列精确读取。

Astro 投影再次验证明确字段、白名单及类型，未知字段丢弃，已声明展示 DTO 损坏则整页失败。appleLaunchPrice 保留原容量选择、变体优先、币种一致及历史 EUR iPhone 13 容量参考价回退；使用精确字符串，不进行汇率换算。非法的原可选变体金额被后端省略后等同缺失，可回退到有效商品容量价；不保留旧非法对象遮蔽商品价的偶然行为。

## 读取与尺寸边界

沿用公开店铺/渠道、发布/删除条件及同一个只读 Repeatable Read。先基于已可见商品/变体的 ID 聚合 OCTET_LENGTH，只在每条不超过 64 KiB、两类总计不超过 4 MiB 后装载私有 JSON；隐藏或已删除商品的 JSON 不参与限额。再逐项投影，无额外写入、预留或会话。每商品最多 64 个颜色、64 个型号编号、128 个容量价；重复 JSON 键、深度超过 32、非对象根或超限整体 503，错误 no-store，不返回截断展示数据。当前不是统一的扩展编辑模型，后续编辑仍须显式命令、版本和审计。

## 验证与边界

[人工 SQL 报告](admin-auth-report.json) store_display 覆盖明确字段和私有排除、精确上市价、原 slug/示例提示、嵌套与根回退、异常可选值忽略、跨渠道隔离、每条/合并尺寸上限，以及调用前后来源指纹不变。纯函数补齐深度、重复键、非法 URL/金额/slug 和字段大小。

[固定来源报告](catalog-mysql-rehearsal-report.json) 用独立 SQL/JSON 读取，逐个核对实际 30 商品、1558 变体的颜色、消费规格、来源提示、slug 及示例价格提示，不调用生产投影函数作为期望值。真实来源无上市价扩展，这类行为通过人工 SQL/前端用例验证，不虚称真实样本已覆盖。

[Astro 报告](astro-runtime-report.json) 包含 11 项单元、类型/build、依赖审计、真实 Gin 源模块读取与生产 SSR health；新增展示用例覆盖规格/颜色/slug、明确字段裁剪和上市价优先级/回退。[真实后台报告](admin-live-report.json) 回归新增 DTO 的空展示与原有 Ant 操作；对应 UI 报告另核对中英三宽。实际 Astro 页面尚未引用商品模块，URL 解析、React 选择、原页面/CSS/资源、搜索和交易仍继续迁移。

本轮固定来源核对 134 个颜色、510 个消费规格字段（含链接及型号编号数组）、30 个来源提示及 1528 个原变体 slug；30 商品/1558 变体全部核对，完整业务读取前后指纹相同。Go test/vet、独占 SQL 和真实链路检查通过。旧 health、app、dk-en/store 均 200，后端近期日志无错误；storefront 保留既存 Fast Refresh 全量重载告警。
