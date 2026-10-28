# Gin 购物车正式关系结构

2026-10-28。0025_cart_relational.sql 从已审查的 MySQL 关系 DDL 和逐字段计划中提取购物车 10 表/136 来源列。它是加购、地区重算和结账所需的正式数据基础，不是已经开放的购物车接口。当前没有 cart HTTP 路由、持有凭证或写命令，Gin checkout 继续关闭。

## 结构与边界

保留 carts、cart_addresses、cart_line_items、cart_line_adjustments、cart_line_tax_lines、cart_shipping_methods、cart_shipping_adjustments、cart_shipping_tax_lines、cart_payment_collections 和 cart_promotions。原 ID、软删除/完成时间、客户/区域/渠道/币种、地址、行配置快照、税/折扣/运费及付款/促销关联均有显式列；不是通用 payload 表。

金额及税率 DECIMAL(38,18)，raw 金额独立 LONGTEXT+JSON_VALID。metadata 与配置快照 JSON 仍为经 JSON_VALID 检查的 LONGTEXT，避免 MySQL 原生 JSON 将高精度数字字面量转浮点。购物车原 quantity 为 INT，未因前端使用字符串随意改变来源范围；运行时新增数量还需非负/上限与业务规则验证。布尔和原非负金额 CHECK 明确执行，其他来源本来允许的历史值不擅自清洗。

8 条来源物理外键保留地址 SET NULL 与行/税/折扣/配送 CASCADE。payment_collection、promotion 等来源只有语义关联的列保留原 ID，不制造父记录、不因为结构存在自动承认有效关系。promotion 领域尚未正式落地，计划明确列出依赖；写流程必须另外验证归属、渠道、父资源状态和金额，不以缺少物理外键为授权依据。

迁移先检查全部目标表和 runtime 标记不存在；同名旧原型或已有业务表直接拒绝，保留其数据，不 DROP/rename/overwrite。迁移执行器继续持有独占锁和逐语句状态，失败后不盲重试不确定 DDL。运行时 cart.Ready 进入 Gin 启动及 health，核对编译模型 SHA、版本、reads=true 与 writes=false；迁移仍由显式命令执行，不在启动自动修改表。

历史购物车结构不恢复旧账号、cookie 或持有凭证，不按 customer_id/邮箱向新账号赋权。地址/email 的历史持久化还需专用保护转换、逐行账本和持有者政策；本次没有导入真实购物车，也未更改旧数据。schema marker 的 reads 表示允许域组件检查，不表示公共匿名查询授权。

## 验证

`python retail/scripts/generate-cart-schema.py --check` 核对生成器、审查 DDL 和嵌入计划一致；Go 完整测试和 vet 检查编译、25 文件计划边界及既有纯规则。

[独占 MySQL 报告](admin-auth-report.json)的 cart_schema 对人工数据验证：完整 25 迁移应用/重复、10 表136列；超过安全整数的金额和 SKU、18 位税率、JSON 数字字面量读回；非法 JSON/布尔/负金额及物理孤儿拒绝；地址 SET NULL、税折扣及配送级联；复合键重复拒绝；来源未设物理键的缺引用继续保留；写能力不能开启、禁读和模型漂移失败；同名 carts 中已有行保留，未部分创建其余购物车表。该证据不等于真实购物车导入、加购并发或完整业务验收。

正式购物车持有凭证/会话、版本与幂等事务、服务端重选价和总共享库存验证、地区/税/促销/配送重算、付款版本/成单唯一性及前台接入继续实现。T032/T033 与完整交易任务仍未验收。
