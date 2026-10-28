# 配送地理区域结构

2026-10-28，迁移0030补齐原`geo_zone`到`fulfillment_geo_zones`：1表/11来源列。此前0021承接的是履约、位置关联及选项规则，不包含此表；地址资格不能因缺失表而跳过检查。

`generate-geo-schema.py`从已审查DDL及逐字段计划生成独立SQL和编译模型，检查与既有领域表不重叠。保留country/province/city/zip枚举、国家/省/城市文本、postal_expression与metadata原JSON文本、时间和软删除；service_zone_id保留物理外键及原级联行为。普通文本索引是已审查的MySQL前缀索引，不能把索引前缀当成完整字段相等条件。

0030先检查目标表及运行时标记不存在，已有同名表时明确失败，不覆盖、改名或删除原表。`GeoReady`核对模型SHA、版本和reads_enabled；writes_enabled始终关闭。Gin启动与health加入该检查，启动不会自行执行DDL，已有独立Gin库需要显式应用0030。

人工独占MySQL专项验证：新库30项完整迁移及重复执行、原JSON空白/重复键/超JS精度数值和前导零保留、SQL NULL、中文/非ASCII、非法枚举/JSON/父外键拒绝、父级删除级联、模型漂移/读取关闭拒绝、已有同名表及行保护。全组回归报告见[admin-auth-report.json](admin-auth-report.json)的`geo_schema`和实现SHA；普通Go测试跳过实库用例，不可代替该报告。测试资源沿用禁网、单核、有界内存的自有MySQL/Redis，完成后删除。

固定来源快照含7条geo_zone行（本批仅核对数量，没有导入）。专用导入回执/依赖hold、邮编JSON条件到MySQL的实际匹配、完整渠道/位置/集合/区域/地址/规则资格、provider及购物车保存仍待完成；不开放HTTP、普通配置写或checkout，不代表完整配送业务验收。
