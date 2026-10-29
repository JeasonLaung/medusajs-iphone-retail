# 促销上下文的当前商品关系

2026-10-29：`catalog.ReadPromotionProducts`按内部购物车刷新图所选字段读取当前商品关系：id、is_giftcard、collection_id、type_id、weight以及categories.id/tags.id。该投影用于后续items.product规则，不是公开商品DTO，不包含metadata、客户或provider数据，也不改变已保存购物车商品展示字段。

调用方必须提供事务，读取沿用同一快照并检查catalog schema；不创建/提交事务，不做商品发布或销售渠道过滤。原内部QueryGraph可读取draft商品和非公开分类关系，故本阶段也不使用storefront公共目录筛选，避免业务规则被公开可见性过滤器悄悄改写。商品可购买、渠道/库存和配置完整性仍由购物车刷新/购买流程校验，存在内部关系不授予购买资格。

重复输入ID合并；缺失/软删商品不放入map，供完整组装器保留cart line并表示null product关系，不凭空重建来源对象。有效商品的null collection/type/weight保持null，无关联返回明确空数组。分类不要求active或非internal；分类/标签本身软删则不参与有效关系。按源ID稳定排序。weight源列是DOUBLE，本函数用SQL文本读取后以JSON number提供规则比较，不能宣称恢复源DOUBLE已丢失的任意十进制精度。

限制1000输入商品，每类关联10000条、每商品1000关联，查询取上限加1并整体报错，不截断后当完整规则事实使用。表名和字段均固定；标识符沿用catalog现有校验规则，不另行禁止已被来源模型允许的空格。未支持字段由完整上下文组装阶段继续补齐；不能把本投影直接当所有商品/variant规则都已支持。

## 验证

隔离CartRuntime实际MySQL人工配置验证draft商品、inactive/internal分类关系、重复输入、缺失/删除商品、软删标签、空type/collection与数值weight、metadata不载入；把实际投影编码成product对象，用已有原行为对照过的MatchRules验证items.product.categories.id/tags.id/is_giftcard/weight路径。独立事务中更新商品weight，RR同一快照保持原结果；autocommit、控制字符ID与关闭schema拒绝。测试结束清理专属promo-facts-*人工配置，不修改实际商品。

完整及cart-runtime-only报告含promotion_products与相关源码SHA，必须同时匹配才算证据。该阶段尚未组装完整购物车上下文，也不计算/保存新折扣，不代表checkout完成。

## 规格与配送档案

`catalog.ReadPromotionVariants`补齐原`cart/utils/fields.js`选择的items.variant.id/product.id/product.shipping_profile.id及weight/length/height/width/material。规格属性读取当前variant列，不回退到product重量或材质；0、null、原样材质字符串分别保留。数值列仍是DOUBLE，以SQL文本转JSON number，不声称恢复任意精度。只选择这些字段，不载入metadata或库存/provider内容。

同样要求调用方事务及catalog schema，最多1000输入ID，重复合并。缺失/软删variant不进入map；缺失/软删product为null；draft商品不被公开目录过滤。配送档案要求链接和目标均未软删，没有关系为null。每商品为单一关系，多个有效目标整体拒绝，不能随意取第一条。有效配送链接查询上限1000并取上限加1检测，不截断。此投影不验证配送方法可用性，也不授予商品购买资格。

隔离MySQL验证规格数值、空字段、材质首尾空格、无product的variant、缺失/软删规格与商品、软删配送链接与目标、重复配送档案拒绝；JSON投影通过现有MatchRules验证原variant规则路径。外部事务同时更改尺寸和配送链接，RR读取保留同一快照。autocommit、坏ID、超限输入和关闭schema拒绝。报告promotion_products新增variant_dimensions_material_shipping_profile及ambiguous_shipping_profile_rejected，仍明确cart_context_or_checkout_integrated=false。

下一步补shipping option、订单/地址/客户新登录态等上下文并连接选择、计算、保存、版本/幂等和支付刷新。T032/T033未验收。
