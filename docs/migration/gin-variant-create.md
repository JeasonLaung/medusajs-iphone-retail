# 现有商品下新建 SKU

商品详情的「新建 SKU」使用 Ant Drawer、Form、Input、Select。选择该商品现有选项的完整组合，填写规格名称；SKU 可手填或留空生成。前台页面、布局及交互未改动。

## 继承的业务

原 `medusa-store/apps/backend/src/modules/sku-numbering/migrations/Migration20261019090000.ts` 规定空 SKU 生成 ERP 编号，明确填写 ERP 编号则预留该号段。`workflows/hooks/variant-sku-inventory.ts` 将实际编号同步到默认库存项，`lib/inventory-titles.ts` 生成「商品 / 规格」库存名称。本次 Gin 命令沿用这些行为；计数器同时核对已导入的规格与库存项（包括软删记录），避免重新自动分配已有 ERP 编号。正式导入时仍须核对原计数器的高水位，不能以现存记录最大值替代已被永久删除的历史编号。

本入口创建受库存管理、不允许超卖、需要配送的实物 SKU，以及数量系数为 1 的独立库存项。每个商品选项须选择一个该商品当前已关联的值，拒绝漏项、外部选项、同维度多选及已有完整组合。非空 SKU 按现有区分大小写的字段语义检查未删除的规格和库存项，不覆盖或复活历史关系。

新库存项没有库存层、预留或价格，不虚增库存。保存后可以沿用已有价格抽屉新增基础价及设备追踪/登记入口；新库存项的仓库库存层建立与实物数量联动尚需后续接通，不能据此宣称新 SKU 已可完整购买。商品创建、选项/选项值管理及其他 SKU 类型仍待实现。

## 接口和事务

- GET `/api/admin/products/:id/variant-creation` 返回当前商品选项、命令开关及修订摘要。
- POST `/api/admin/products/:id/variants` 严格接收 operation_id、expected_revision、title、sku 和 option_value_ids，拒绝重复/未知键、null、重复值ID、附加正文、无效UTF-8及超大请求。
- 当前 product.read 和 product.write 均必需；沿用已有 Origin、CSRF 和会话校验，仓库/供应商主体不能访问此全店入口。
- 0063 新增命令开关/序号和不可变操作收据，默认关闭；不自动打开正式数据库。已提交迁移共 62 个，不夹带原工作区未提交的支付通知迁移。

事务中先锁定当前账号及授权，再锁全局 SKU 命令行和商品行；相同操作重试要求原账号、商品及完整请求一致，且目标 SKU/库存关联仍存在。新操作检查修订、完整选项、组合与全局编号唯一性，最后同时写入规格、选项关联、库存项、数量关联、计数器及收据。末尾写入失败时整体回滚。创建不隐式发布商品或修改历史价格、库存和设备。

界面在未知提交结果时锁定原输入，重试使用同一 UUID、修订及全部内容。冲突要求关闭重新读取；权限撤销和目标删除拒绝重放。

## 验证与边界

隔离 MySQL/Redis 通过四并发同操作唯一创建、完整选项、重复组合/编号拒绝、显式 ERP 预留、软删导入编号高水位、过期表单、权限撤销、目标删除和末尾收据失败回滚。新增浏览器用例验证丢响应后完整请求保持不变及最新授权，原有 21 项后台回归通过。

真实 React → Gin → MySQL/Redis 联合通过：登录、新建完整规格、自动编号、继续新增精确基础价格、公开商品API读取同一SKU及报价、刷新、中英及390px布局。没有API模拟、真实客户或生产数据。只验证公开 Gin 商品读取，未扩大为整条 Astro 购物/库存/配货验收。

证据：[variant-create-report.json](variant-create-report.json)、[variant-create-joint-report.json](variant-create-joint-report.json)。旧后台健康/Admin与前台列表、商品、购物车均返回200；旧前台日志仍有既有 `transformAlgorithm` 异常和 `/store/locales` 404，本批未改动旧服务。
