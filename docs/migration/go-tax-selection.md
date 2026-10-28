# Go 税率选择与 system 税行

2026-10-28，T025/T032/T033 的计算子步骤。税区 SQL 读取、购物车重算事务和完整税务配置尚未验收。

`retail/backend/internal/tax/selection.go` 独立实现原税模块的候选筛选、稳定优先级及内置 system provider 投影。运行时只依赖 Go 与既有精确十进制工具，不加载、代理或启动 Medusa。现有开发服务、SQL migration、购物车 HTTP 能力和预览内容不变。

`Select` 只允许指定税区的默认税率或与本行 product/product_type/shipping_option 匹配的规则进入候选；默认税率带其他规则仍可入选。依原代码顺序，省商品/配送规则优先于省商品类型、省默认、国家商品/配送规则、国家商品类型、国家默认。同分保留输入次序，不取最大税率，不自行排序 ID。调用方必须保留已审查的目录顺序，后续 SQL 加载需要记录同级歧义，不能据稳定纯函数声称解决了来源查询无排序的问题。

选中项为 combinable 且存在 parent_id 时，仅附加候选中最优的一个父税区税率。不递归祖父区，不合计全部父候选，不为缺失父区补造税率。国家/省判定沿用 `province_code !== null`，空字符串与 null 的行为不同；未知 reference 不匹配商品。

`SystemLines` 保留商品税行在前、运费税行在后的原 provider 顺序，携带 rate_id/name/code/provider_id 和对应行 ID。null 税率成为精确字符串 `"0"`，非空值通过已有 `totals.Resolve` 规范化；不经过浮点，不提前按币种舍入，也不进行含税价换算。此函数只代表内置 system provider，不替代其他 provider，也不自动判断 provider 配置是否启用。

输入按有界内部快照处理；缺失标识、重复税率/行、税区关系不一致、非法十进制和过大集合整体拒绝，不返回部分税行。输出的税率、规则和可空字段与输入不共享可修改引用。shipping 显式判别保留“运费 option 为 null”与商品的区别，不依赖 option 值真假。

`capture-tax-selection.cjs` 直接调用安装的原 `TaxModuleService` 未装饰纯方法，及 `SystemTaxService.getTaxLines`，生成 24 组选择和 6 组 provider 人工对照。两份实际来源文件 SHA 固定在 `fixtures/legacy-tax-selection.json`，无需真实数据、账号、DB、容器或外部 provider。场景覆盖所有优先级、同分双向次序、组合父税区、不递归、缺失/不匹配父区、商品与运费隔离、默认与未知规则、空候选、null/零/高精度税率及 provider 输出顺序。Go 直接读取 fixture 对比，另验证非法边界、输入不变、输出独立及 32 个并发 worker 各 20 次选择/投影。

```text
python retail/scripts/test-tax-selection.py
cd retail/backend
go test ./...
```

本批上述命令通过，报告见 `tax-selection-report.json`，其中记录本批实现与 fixture 的 LF 规范化 SHA、实际原函数覆盖和验收范围。此处未运行 SQL 集成，也未调用外部税服务，不将普通 Go 全包测试的环境跳过视为数据库验收。

固定来源清单当前有 1 条 tax_provider、7 条 tax_region、0 条 tax_rate 和 tax_rate_rule。这只描述固定快照，不能硬编码免税或删除税率配置功能。后续需增加正式 SQL 结构及导入、国家/省规范化与父税区解析、自动/强制税计算和缺地址处理、giftcard 过滤、provider 校验、税率配置权限及 Ant 页面、购物车税行保存与金额装配。历史订单税行继续作为成交快照，不以当前税率覆盖。上述过程与地址、重新选价、促销和配送需要统一进入换区版本及持久幂等事务；本子步骤不开放 checkout，不表示 T025/T032/T033 完成。

后续SQL实现见[税区正式结构与读取](tax-schema-read.md)：新增0028和调用者事务内ReadSystem，已有人工SQL验证；本页纯函数报告仍只证明选择/投影范围，专用导入及购物车事务继续待办。
