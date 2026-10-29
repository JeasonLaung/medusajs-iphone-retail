# 促销资格规则匹配

2026-10-29：独立 Go `promotion.MatchRules` 承接订单、items、shipping_methods 三种服务端上下文中的规则谓词。所有规则 AND；eq 对取出的所有值匹配，in 对任一个值匹配，ne 对所有值不匹配。空数组只有 ne 成立。支持嵌套对象、数组一层展开、原 scope 前缀移除及标量路径提前结束；字符串化保留 null、undefined、布尔、数组与对象的原比较语义。它与配送规则的比较器独立，不能互换。

范围比较支持 gt/gte/lt/lte，使用精确有理数，不先转换浮点或按两位小数舍入。数值型 JSON 在等值规则中按原 JavaScript Number 字符串语义比较，范围规则则保留输入十进制词法；超出 JavaScript 安全整数的金额必须由服务端以十进制字符串提供。范围输入拒绝非数字和过大指数。上下文、规则数、路径深度和规则值有明确大小限制。

实际安装的 promotion `utils/validations/promotion-rule.js` 是行为来源。`node retail/scripts/capture-promotion-rules.cjs --check` 运行其纯函数，验证 34 组独立预期、输入不变并核对固定源码 SHA 与完整 fixture，见 `fixtures/promotion-rules.json`；不初始化应用、数据库或 provider。Go 测试逐项复用该 fixture，另验证精确小数、超安全整数、科学计数、负零、非法上下文、超限和不支持操作符。已通过 `go test ./...` 与 `go vet ./internal/promotion`。

非法配置返回错误，调用者必须停止该次促销计算；不能把错误当作无规则而放行。不同于旧函数的短路返回 false，本实现先验证所有规则结构与范围阈值，避免前面的不匹配掩盖后面的错误配置。规则 values 中混合非字符串时仍按原实现只比较字符串；完全没有字符串则返回错误。

这只是资格谓词，不读取浏览器拼装的可信业务事实，不查 SQL，不生成折扣、不占用预算、不落库。活动有效期、自动促销与 code 合并、商品/运费分摊、buy-get、共享折扣余额、预算并发和完整购物车重算仍须实现；T032/T033 未验收。旧 46 组 commerce fixture 仍是其他规则与分摊子集的参考，不能用这 34 组资格验证替代完整流程验证。
