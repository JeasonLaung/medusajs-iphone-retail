# MySQL回收价格与公开估价接口

2026-10-30。0073新增buyback_prices，已提交迁移数量为72（编号0037是此前未完成且未提交的独立支付工作，不包含在本次验证中）。原id、product_id、capacity、amount/raw_amount、pricing_rules、币种、软删除及时间保留。容量使用LONGTEXT原值，活动行以SHA256摘要实现原product/capacity部分唯一索引，精确查询同时比对容量文本；摘要冲突将拒绝插入，不会合并业务记录。已删除价格允许重建。产品外键有效，没有客户认证关联。

Gin的 GET /api/store/buyback/prices?product_id=... 返回四字段公共价格列表，最多100项；POST /api/store/buyback/estimate 接收原问卷字段并调用已对照的Go规则。未找到价格保持unconfigured/null，SQL NULL及JSON null规则回退原参考金额，禁用及资格失败分别返回disabled/ineligible。沿用原公开价格按商品ID读取的范围，不擅自给其增加销售渠道过滤；商店必须由服务器配置并真实存在。读操作使用只读RR事务及有界请求超时，不创建任何账户、报价配置或业务数据。

原raw金额优先，示例10.1234567890123456789由Gin精确保留为主货币单位JSON数字。浏览器JSON数字转换沿用原JS显示精度，不声称前台浮点能保留任意精度。公开列表不返回raw/rules/来源或内部ID。非法存储配置、原金额无效及过大内容返回503，不以0兜底。输入拒绝重复/未知query、重复JSON和非法问卷。

Astro新增两个同名固定路由及buybackBridge/buybackClient。匿名只读，不转发Cookie、Authorization或上游Set-Cookie/错误正文，不跟随跳转；POST保留原始字节，不能通过JSON重编码绕过Gin重复键检查。响应严格投影，金额仍按元显示。原sell问卷页面尚未挂接此客户端，历史价格也未导入；此处不宣称回收已可提交。

真实MySQL/Redis运行ProductPrices（包含新BUYBACK_PRICE_REHEARSAL）、AdminAuthentication、InventoryStockEdits、ProductInventory四组，清理独占容器；规则估价152.02、缺值/旧价/禁用、容量转换、重复及软删除、错误/权限/只读指纹通过。Go全量、3项前端测试和Astro332文件检查通过（0错误/0警告/5条既有提示）；旧9001 health/app及8000 sell为200。完整Astro→Gin浏览器联合、历史导入、原问卷及申请/照片/设备/付款退回仍待完成。源码SHA及证据范围见 [报告](buyback-prices-api-report.json)。
