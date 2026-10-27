# 迁移基线索引

日期：2026-10-27。总任务见 [任务清单](../gin-react-astro-tasks.md)，实现状态见 [retail/MIGRATION.md](../../retail/MIGRATION.md)。本目录是业务核对证据及契约草案，不是业务迁移完成报告。

| 范围 | 交付物 |
| --- | --- |
| 全量入口与承接任务 | [业务基线](business-baseline.md)、[源码追踪](source-traceability.md)、source-manifest.json |
| 页面与状态 | [32 种 URL + 7 个 handler](frontend-routes.md)、[84 张本机截图索引](frontend-baseline.md) |
| 身份与数据范围 | [授权矩阵](authorization-matrix.md)、[设备逐动作权限](device-commands.md) |
| 交易领域 | [商品及数据关系](catalog-contract.md)、[搜索筛选](search-contract.md)、[支付与订单完成](payment-contract.md) |
| 自定义业务 | [设备命令及固定用例](device-commands.md)、[回收与邮件](buyback-and-mail.md)、[SEO 与引用监测](seo-contract.md) |
| 统计 | [Analytics 实际口径](analytics-contract.md) |
| 数据继承 | [源数据去向与转换](data-mapping.md)、data-table-disposition.csv、data-source-manifest.json |

## 本轮已运行验证

```powershell
python retail/scripts/inventory-business.py
python retail/scripts/verify-baselines.py --captures logs/gin-migration/storefront-baseline-v2 logs/gin-migration/storefront-product-baseline
node --check retail/scripts/capture-storefront.cjs
git diff --check
```

Python 需 3.9+；命令中的 python 替换为本机已安装运行时路径。数据清单脚本的只读元数据查询和快照要求见 [数据映射](data-mapping.md)。

已核对 200 个源码文件哈希、86 个路由方法行、36 个 page 文件与 32 个去重模式、193 表 / 1,961 列 / 27,779 行计数，以及 84 张截图 SHA-256。截图均 HTTP 200，无检测到的横向溢出或坏图，**18 个旧页面实例有 hydration mismatch**，详见前台基线。完整性检查通过不表示这些页面没有错误或新实现可替换旧系统。

旧服务 `/health`、`/app` 与 `/dk-en/store` 本轮检查 HTTP 200。未变更旧业务源码/数据，未构建旧开发 checkout，未连接部署服务器。新 MySQL 导入、React Admin、Astro 和完整交易仍待实现。

## 尚未完成的阶段门槛

- T001/T002：剩余命令/调用方、原生合计与促销税运费、PayPal 实际运行产物及后台内置功能。
- T003：剩余页面及登录/抽屉/错误/加载/交易状态；需要隔离夹具，不在旧业务库造订单。
- T004/T005：剩余领域授权、完整交易金额样例和状态用例运行证据。
- T006：逐字段目标模型与关联、私有文件清单及密文转换演练；现有全表元数据不是全字段映射完成。
- T007：统一 API 约定、可验证 schemas、完整原生/自定义旧入口到新 API 的映射；目前只有领域草案。

这些待办明确保留，不把未知能力当作未使用删除。后续按领域补齐后再实现及验收。
