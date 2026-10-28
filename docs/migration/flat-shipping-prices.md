# 固定运价的共享价格集合读取

2026-10-28，T032/T033中间实现，配送可用性及完整重算未验收。

`pricing.ReadSets` 从可信服务器解析的价格集合ID读取当前有效集合、币种价格、完整规则、价目表和含税偏好，复用原精确选择引擎。原商品变体 `pricing.Read` 先解析变体关联，再调用同一集合读取；没有复制另一套价格规则。集合及关联查询有数量预算，金额使用原raw精度，缺失/删除集合及缺价不伪造零报价。

`fulfillment.ReadFlatPrices` 在调用者SQL事务内核对履约模型，并按 fulfillment_option_price_sets 读取未删除 flat 配送选项的有效价格集合。同集合可以被多个选项引用；单选项存在多个有效集合关联则整批拒绝。calculated、缺失、已删除选项没有报价，也不调用动态provider。返回值保留服务端选价证据，不是公开HTTP DTO。

这只是运价阶段，不授予配送资格。原 list-shipping-options-for-cart-with-pricing 分开处理flat与calculated，flat使用cart上下文的价格集合，calculated交由provider。后续编排仍须完成店铺渠道→仓库→履约集合、地址地理区、商品shipping profile及规则、provider可用性和上下文hook。当前 pricing.Context 只承接已经实现的地区/币种/渠道/数量/客户组；cart总额等额外价格条件、原运价上下文缺省数量语义与自定义条件仍须核对承接，不能把当前报价直接作为公开可选运费，也不能擅自填quantity=1代替原缺省语义。

人工实库在 identity/flat_shipping_prices_test.go，接入原价格专项人工库。验证共享集合、超18位精确金额、显式可信地区/数量/币种上下文、含税偏好、缺价非零、动态选项排除、删除选项/关联/集合、重复输入、多集合歧义、读取前后指纹不变、拒绝自动提交和停用模型。实际运行状态以 admin-auth-report.json.flat_shipping_prices 及源码摘要为准。原商品选价、购物车和其他领域由同一隔离SQL整组回归覆盖。

```text
python retail/scripts/test-admin-auth.py
cd retail/backend
go test ./...
go vet ./internal/pricing ./internal/fulfillment
```

本批没有新SQL迁移、启用能力、HTTP、外部调用、持久写或前台样式变化。运费保存与失效配送方法移除、税行/促销及支付重算、正式页面和checkout仍需后续完成。

本批结果：完整隔离SQL及Go全包/静态检查通过；固定快照159表/27140行复核通过，30个公开商品、1558变体、两个报价上下文与原服务对照，查询指纹不变。报告源码SHA逐项核对；测试资源清理完成。进一步审查原pricing/repositories/pricing.js确认：quantity为undefined时仅要求min_quantity<=1或为null，不检查max_quantity，直接填1会改变边界。当前入口仍要求显式正整数quantity，这项原缺省语义继续待实现，不以此批测试宣称覆盖。
