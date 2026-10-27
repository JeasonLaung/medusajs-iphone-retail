# 商品与配置逐字段目标设计

日期：2026-10-27。T006 第一批：36 张源表、292 列，显式列名覆盖商品/选项/图片/分类/标签、共享价格集合/规则/偏好、区域/国家/币种、渠道和店铺。其余 157 表仍待逐字段设计。机器清单：[JSON](catalog-field-mapping.json)、[CSV](catalog-field-mapping.csv)；这不是已执行的 MySQL DDL，也不是导入成功证明。

## 生成与核对

```powershell
python retail/scripts/capture-schema-constraints.py --check
python retail/scripts/map-catalog-fields.py --check
```

第一个命令仅以 BEGIN READ ONLY 从本机既有 PostgreSQL 提取约束/索引/枚举定义，不读业务行；结果 [source-schema-constraints.json](source-schema-constraints.json) 覆盖 349 条约束、759 个索引及 17 个枚举值。第二个仅读已提交元数据和显式字段声明，不访问数据库。源码列增加/删除、类型变化、目标设计变化会使检查失败，需要审查后重新生成。

字段清单逐项包含 source_expression、目标表/列、类型设计、nullable、缺失处理、转换名、关联、断言和 designed-not-implemented 状态。只对明确列名生成，不用“未知字段全部 payload”兜底；目标列名原则上保留原名，仅 category_product/tag join 两处名称明确转换。source constraints/index names 链接回原始定义，不能把标记关联等同于已转成 MySQL 约束。

## 类型和精确性

- ID 及关系采用最长191字符的 UTF-8 字符串设计，保留大小写、不 trim；handle/SKU/barcode/ean/upc 最大512字符。该长度是目标约束，不是声称源 text 已有长度上限；导入预检超限必须停下报告，不能截断。编号、国家 num_code 保留字符串前导零。
- 使用 `utf8mb4_0900_bin` 保留尾空格区分，不能误用有 PAD SPACE 行为的 `utf8mb4_bin`；目标实例须支持该 collation 并在 T012 实测。依据 [MySQL Unicode collations](https://dev.mysql.com/doc/refman/8.0/en/charset-unicode-sets.html)。这也意味着旧 SQL 草稿的 handle/SKU collation 需要修订。
- 本批 price.amount、currency.rounding 与数量设计为 DECIMAL(38,18)，保留20位整数和18位小数；进入数据库前精确检验超限/舍入损失。源快照金额最大4位整数、分数0位并不等于未来只能整数价。该设计用于持久化字段；促销中间运算不能据此截为18位，更不能统一先两位舍入。
- `raw_*`、metadata、price_list_rule.value 采用经 JSON_VALID 约束的 LONGTEXT 精确文本存储，Go UseNumber/RawMessage 处理；保留数值词法与 JSON null/SQL NULL 区别。公共 DTO 仍逐键白名单。索引查询所需字段另建类型明确的关系/标量，不能靠 MySQL 原生 JSON 数字重序列化保证任意精度。MySQL 原生 JSON 的数值与类型转换行为需另行实测，参考 [JSON 数据类型](https://dev.mysql.com/doc/refman/8.4/en/json.html)。原 payload 草稿需调整，不能宣布已无损。
- 时间保持源 UTC instant，DATETIME(6)；缺少必填时间/无效值/越界拒绝，绝不默认 epoch 或 now。未提供的字段与显式 null 区分；nullable 才可存 null。boolean false、空字符串、0 均按其真实类型保存。
- 商品物理尺寸源是 float4，目标 DOUBLE 承接有限 float32，单独保存为物流维度；不把其转换逻辑用于价格或库存。NaN/Infinity 必须拒绝并登记差异。

## 键、唯一性与软删除

从实际源定义确认：product/category handle、variant sku/barcode/ean/upc 是 `WHERE deleted_at IS NULL` 唯一。目标设计用活动生成键或等价唯一注册关系，仅活动行参加；删除行保留原值，恢复前检查冲突。不能把 `(value,deleted_at)` 当 MySQL 活动唯一约束：NULL 比较无法阻止两个活动行重复。

product_option.title 只有“未删且 is_exclusive=false”时全局唯一；exclusive 的同名选项可并存。option_value 的 `(option_id,value)` 仅活动唯一，option_id 为 NULL 时保留源 NULL 唯一性语义，不用空字符串替代。price_rule 的 `(price_id,attribute,operator)` 活动唯一；price_preference 的 `(attribute,value)` 活动唯一且 value 可 NULL。

这些源字段是无界 text；目标最长键及复合键总长度要在 T012 的实际 MySQL/InnoDB 上核对。LONGTEXT 上不能直接照搬全值 UNIQUE；不能用前缀唯一索引替代精确唯一性。需要先预检实际最大长度，超目标限制则调整设计；长文本必要时使用锁定唯一注册命令并做完整值碰撞核验，不能直接信哈希相等。

以下关联的主键是业务对，而非名为 id 的字段：product_sales_channel、product_shipping_profile、product_variant_inventory_item、product_variant_price_set、region_payment_provider、sales_channel_stock_location；源 id 另保留，不能假设其独立唯一并用它覆盖业务对。category_product、product_tags、variant_option 同样保留复合主键。其余带 id 的成员关系各按源主键审查，不因字段类似就统一去重。

## 关联、默认值和删除策略

- option→value→product_option membership→product_value membership→variant selection 逐层保存。全局 option 可共享，但 value 是否属于当前商品由成员关系决定，不能把所有 option.values 发给每个商品。
- variant 与 price_set 的关系、price 的主键及 price_set_id 原样保留。不能为每 SKU 复制一份 price 并丢掉共享集合语义。shipping option 的 price_set 属于同一 pricing 域，不能漏导。
- 图片主体与 variant_image 关系保留 id/rank/deleted_at；public URL 改写通过文件映射单独记账。未删变体指向已删图片或跨商品图片作为差异报告，不静默挂第一张图。
- 分类 parent 与 mpath 同时保存并校验环/孤儿/路径一致；mpath 是派生路径，变更时领域命令重建，导入发现矛盾需记录，不静默覆盖原始证据。
- 省略 SQL DEFAULT 依赖：导入每个源字段都显式提供。新写入默认值属于领域 schema/命令设计，不用旧 DB 默认掩盖导入缺项；源默认定义尚需下一批元数据扩充。
- 保留源关系包括软删除父行。目标外键不随意 ON DELETE CASCADE 到历史业务；运行命令优先软删除。硬删除/隐私处置另有明确保留策略才执行。源 FK CASCADE 是证据，不是机械复制到所有新业务表的指令。
- 跨 inventory location/item、payment provider、shipping option 的字段已有目标参考，但对应表模型仍待领域映射；全域导入完成前不能用空记录或删除关系来凑外键成功。
- store 默认 region/channel/location 允许源 NULL，不能凭表第一行自动填补；country.region_id=NULL 代表未配置国家，不自动绑定默认地区。销售渠道停用、分类 internal、价格表状态都保留并影响读取。

## 导入顺序与验收断言

先 currency/channel/region/store 和独立 catalog definitions，再分类父树/商品/选项成员/变体/图片，随后 price_set/list/rules/preferences 和关联。store defaults 与跨库存、付款、配送域关系可以在相关主表齐备后第二阶段验证；循环依赖先显式 nullable 暂置，再事务修复及验证，不关闭 FOREIGN_KEY_CHECKS 掩盖孤儿。

每表按总行、活动行、软删行与主键集合对账；每列按转换后规范值比较；所有 numeric/raw 对校验，未知规则保持内部且选价拒绝；字符串长度/排序规则/大小写/尾空格与历史唯一键分别测试；空表仍建目标模型，不能因 price_list 当前为0行就删除能力。目录公开 DTO 另外检查发布/渠道/软删/内部字段，不把“导入有行”当作可公开。

T012/T018 后续必须运行 MySQL DDL 与真实只读快照预检及导入演练；当前未接 MySQL，尚无列长度、JSON词法、活动唯一键或全关联的实际导入证明。当前 `0001_foundation.sql` 和 catalog importer 属未验收草稿，不能用此文档倒推它们已满足设计。
