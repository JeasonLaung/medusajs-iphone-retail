# 迁移基线索引

日期：2026-10-27。总任务见 [任务清单](../gin-react-astro-tasks.md)，实现状态见 [retail/MIGRATION.md](../../retail/MIGRATION.md)。本目录是业务核对证据及契约草案，不是业务迁移完成报告。

| 范围 | 交付物 |
| --- | --- |
| 全量入口与承接任务 | [业务基线](business-baseline.md)、[源码追踪](source-traceability.md)、source-manifest.json |
| 页面与状态 | [32 种 URL + 7 个 handler](frontend-routes.md)、[84 张本机截图索引](frontend-baseline.md) |
| 身份与数据范围 | [授权矩阵](authorization-matrix.md)、[设备逐动作权限](device-commands.md) |
| 交易领域 | [商品及数据关系](catalog-contract.md)、[搜索筛选](search-contract.md)、[支付与订单完成](payment-contract.md)、[购物车/合计/库存/成单](checkout-contract.md)、[促销/税率/配送](promotion-tax-shipping-contract.md) |
| API 与后台组件 | [API 通用约定](api-conventions.md)、[Ant Design / ECharts 及 23 页承接](admin-ui-contract.md) |
| 自定义业务 | [设备命令及固定用例](device-commands.md)、[回收与邮件](buyback-and-mail.md)、[SEO 与引用监测](seo-contract.md) |
| 统计 | [Analytics 实际口径](analytics-contract.md) |
| 数据继承 | [源数据去向与转换](data-mapping.md)、[第一批36表292列设计](catalog-field-design.md)、catalog-field-mapping.json/.csv、catalog-preflight-report.json、data-table-disposition.csv、data-source-manifest.json |
| 库存/设备数据 | [第二批12表139列设计](inventory-device-field-design.md)、inventory-device-field-mapping.json/.csv、inventory-device-preflight-report.json、[语义差异](inventory-device-semantic-report.json) |
| 交易数据 | [第三批42表523列设计](commerce-field-design.md)、commerce-field-mapping.json/.csv、[字段差异报告](commerce-preflight-report.json)、[交易语义审计](commerce-semantic-report.json) |
| 结账规则数据 | [第四批33表321列设计](checkout-rules-field-design.md)、checkout-rules-field-mapping.json/.csv、checkout-rules-preflight-report.json、checkout-rules-semantic-report.json |
| 自定义运营数据 | [第五批24表308列设计](operations-field-design.md)、operations-field-mapping.json/.csv、operations-preflight-report.json、operations-semantic-report.json |

## 本轮已运行验证

```powershell
python retail/scripts/inventory-business.py
python retail/scripts/verify-baselines.py --captures logs/gin-migration/storefront-baseline-v2 logs/gin-migration/storefront-product-baseline
node --check retail/scripts/capture-storefront.cjs
node retail/scripts/capture-totals.cjs --check
node retail/scripts/capture-inventory.cjs --check
node retail/scripts/capture-commerce-rules.cjs --check
python retail/scripts/capture-schema-constraints.py --check
python retail/scripts/map-catalog-fields.py --check
python retail/scripts/preflight-catalog-mapping.py --bundle logs/gin-migration/source-20261026.json
python retail/scripts/map-inventory-device-fields.py --check
python retail/scripts/preflight-catalog-mapping.py --domain inventory-device --bundle logs/gin-migration/source-20261026.json
# 当前审计预期退出1：库存预留合计有1项真实差异，详见报告；不能视为通过
python retail/scripts/audit-inventory-device-snapshot.py --bundle logs/gin-migration/source-20261026.json
python retail/scripts/map-commerce-fields.py --check
python retail/scripts/test-commerce-audit.py
# 两项交易预检当前预期退出1：162条记录存在256个未解析引用值
python retail/scripts/preflight-catalog-mapping.py --domain commerce --bundle logs/gin-migration/source-20261026.json
python retail/scripts/audit-commerce-snapshot.py --bundle logs/gin-migration/source-20261026.json
python retail/scripts/map-checkout-rules-fields.py --check
python retail/scripts/preflight-catalog-mapping.py --domain checkout-rules --bundle logs/gin-migration/source-20261026.json
python retail/scripts/audit-checkout-snapshot.py --bundle logs/gin-migration/source-20261026.json
python retail/scripts/map-operations-fields.py --check
python retail/scripts/preflight-catalog-mapping.py --domain operations --bundle logs/gin-migration/source-20261026.json
python retail/scripts/audit-operations-snapshot.py --bundle logs/gin-migration/source-20261026.json
python retail/scripts/test-operations-audit.py
python retail/scripts/verify-field-mappings.py
git diff --check
```

Python 需 3.9+；命令中的 python 替换为本机已安装运行时路径。数据清单脚本的只读元数据查询和快照要求见 [数据映射](data-mapping.md)。

已核对 200 个源码文件哈希、86 个路由方法行、36 个 page 文件与 32 个去重模式、193 表 / 1,961 列 / 27,779 行计数，以及 84 张截图 SHA-256。截图均 HTTP 200，无检测到的横向溢出或坏图，**18 个旧页面实例有 hydration mismatch**，详见前台基线。完整性检查通过不表示这些页面没有错误或新实现可替换旧系统。

另从已安装旧系统纯函数提取 18 组人工金额样例及 10 组库存输入准备样例，固定结果及源文件哈希位于 fixtures/。已验证输出可重现；未启动应用或写入业务库。这些样例不等于促销引擎、完整结账或 Gin 实现已通过验证。

促销/税率/配送另有 46 组人工规则样例，覆盖分配、额度边界、税率优先级、地理查询和规则验证；均有独立预期且重复核对通过。完整预算事务、buy-get、provider 和结账集成尚待运行。

数据设计新增36表292列映射、349条约束/759个索引/17个枚举值；快照预检151,907个字段值、1,779个raw配对、36,535个引用通过。只有聚合结果入Git，不输出业务行；尚无MySQL导入或完整语义验证。

第二批12表139列，累计48表431列。库存/设备预检55,692字段值及2,880 raw配对；5,506引用检查中有119条已删行引用未解析并明确保留历史。独立语义审计6,429项发现1条预留合计差异、3条预留超过现货观察项，未改旧库，不能宣称已全关联对账通过。

第三批42表523列，累计90表954列。交易预检7,005字段值、428 raw配对、840引用；9类引用缺失共256值影响162行，包含未软删除行，命令明确失败。103项实收/退款/版本审计无额外金额或版本失败，9个人工审计测试通过；provider远端事实、全订单合计及MySQL仍待验，不能由子集通过推断全交易对账通过。

第四批33表321列，累计123表1,275列；预检4,729字段值、106 raw配对、543引用无失败，63项购物车分类/税区/配送规则检查无失败。52购物车中13已完成、1未完成带付款历史、38无关联付款集合；本地状态不替代provider远端结算证据。促销和税率当前空表不构成引擎验收。累计完整性检查保留既有256个交易引用失败和119个设备软删历史引用，不隐藏为全绿。

第五批24表308列，累计147表1,583列；预检802字段值、61 raw配对、72引用无失败，10项语义检查无失败，7个人工反例通过。2封旧注册验证邮件禁止重放，6张私有照片待文件校验；密钥可解密性、新信封及数据库导入仍待验证。既有库存/交易问题保留。

旧服务 `/health`、`/app` 与 `/dk-en/store` 最近检查 HTTP 200。未变更旧业务源码/数据，未构建旧开发 checkout，未连接部署服务器。新 MySQL 导入、React Admin、Astro 和完整交易仍待实现。

## 尚未完成的阶段门槛

- T001/T002：剩余命令/调用方、完整促销资格/预算/buy-get、税运费 provider 集成、PayPal 实际运行产物及后台内置功能；已有纯函数基线不替代集成。
- T003：剩余页面及登录/抽屉/错误/加载/交易状态；需要隔离夹具，不在旧业务库造订单。
- T004/T005：剩余领域授权、完整交易金额样例和状态用例运行证据。
- T006：已设计147表1,583列；剩余46表378列、全部约束转换/语义关联、私有文件清单及密文转换演练待完成；库存预留差异、设备软删历史及交易孤儿引用另行对账，全表元数据不是全字段映射完成。
- T007：已有统一 API 约定及后台组件/页面承接草案；可验证 schemas、完整原生/自定义旧入口到新 API 的映射仍待交付。

这些待办明确保留，不把未知能力当作未使用删除。后续按领域补齐后再实现及验收。
