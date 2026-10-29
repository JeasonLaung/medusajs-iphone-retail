# SKU 单台设备批次入库

商品详情→设备追踪抽屉→设备入库现接私有 Gin GET/POST `/api/admin/products/:id/variants/:variant_id/device-intake`。本批支持同一 SKU 的1—200台整批登记（每台最多8张照片）；每台可分别选择自有来源、类别、SN/IMEI/IMEI2/EID、成色、电池、成本、币种、库位和备注。原跨SKU导入、移动入库、仓库员工专用入口及完整设备管理/配货尚缺，本批不将入库模块或C05/C07验收。

对照原 `device-operations.ts` 的 managed() 回退：合法库存SKU不强制先显式开启追踪。SKU必须管理库存、关闭超卖，并只关联一个有效库存项，required_quantity精确为1。开启追踪本身仍额外检查库存项不可被其他SKU共用；入库不会借此更改数量库存或自动开启追踪。

成色与电池从SKU选项取默认，并校验成色完全一致、实际电池达到SKU档位。显式测量值优先；iPhone是否必须提供号码由真实商品 metadata/type/title/handle 决定，保留 requires_imei:false 的优先级。SN规范化为完整Unicode大写，号码保留前导零，沿用跨字段唯一占用。SKU电池默认值为小数时，源工作流允许浮点而审核设备列是整数；新入口明确拒绝缺省的小数默认，要求提交合法整数测量值，不截断事实。这项差异仍需在历史数据/选项核对中复核。

来源必须是有效 self 来源，且其绑定仓库有效；请求不能自报 location_id。sale→available，returned/recycled→inspection。整批设备、身份占用、intake事件、修订和操作收据在同一MySQL事务提交；任何号码冲突、身份/配置错误或末尾收据失败整批回滚。成本仍按原 cost_cents 整数最小货币单位存储，0—100000000；不是商品销售价格换算。备注保留换行。

新增明确 device.intake 动作，既有角色不自动加权；读取要求 device.read、product.read、inventory.read、warehouse.read，写入还需device.intake。先当前账号/授权事务检查，再设备锁、业务事实和收据。warehouse/supplier主体不能使用此全店入口；其设备专用范围及分类操作仍待接。撤权后旧成功收据也拒绝。HTTP继承Origin/CSRF及私有响应保护。

请求要求显式字段，重复键（含每台设备内部）、未知/缺字段、除成色/电池外的null、浮点整数、附加JSON、查询参数拒绝。UUIDv4操作ID与64位状态摘要保证同请求重试恢复；摘要包含当前SKU、号码要求、选项及有效来源投影，读取后配置改变返回409。其他主体/商品/SKU/内容不得复用收据。成功收据返回原设备事实，不按刷新后的状态覆盖原结果。

照片现接 [公开设备照片上传与存储核验](gin-device-photos.md)：服务器只接受已保存、ID/URL匹配且文件头、大小和SHA256通过核验的照片；设备、事件、收据均保留照片，重新读取近期设备仍可查看。历史文件引用迁移仍待完成。

后台复用Ant Drawer/Form.List/Select/InputNumber/Table，可添加/移除批次行；丢响应锁表单并保留同请求重试，409要求重读，空自有来源禁用提交。成功显示规范化SN、仓库和状态。再次打开时读取当前SKU最近50台有效设备（明确上限），不投影成本/事件/客户等字段；当前仅提供近期列表，完整搜索分页及设备操作后续实现。前台旧页面不改版。

验证使用独立暂存源码快照，排除本地未提交Mollie通知。Go全包与配置/严格JSON用例、58项迁移及AdminAuthentication/InventoryStockEdits/ProductInventory三套SQL/HTTP回归通过。覆盖未开启追踪的合法SKU、成色/电池默认与测量校验、商品号码要求、配置变化、4请求唯一创建/重放、同批跨字段冲突整批回滚、撤权后重放、仓库主体拒绝、末尾收据故障、三种类别状态、刷新后近期设备及库存完整快照不变。

真实联合：`python retail/scripts/test-warehouse-inventory-joint.py --device-intake`。浏览器用新账号登录，在仓库页创建设备来源，在商品页开启追踪，上传照片后再提交销售/退回两台批次，结果与刷新后的最近设备可见；中英与390宽度通过。回查MySQL要求设备2、intake事件2、入库收据1、身份占用4、照片文件1且设备引用1，设备位置均等于来源仓库；原精确库存 `100.000000000000000001` 不变，库存修改收据0。MySQL/Redis单核768/128MiB，独占桥接网允许外连，API随机端口只绑定127.0.0.1；未验TLS安全Cookie。清理归属容器、网络与临时二进制后才写报告。

合成API浏览器另验丢响应同请求恢复、只读隐藏、空来源、配置冲突重读、加载恢复和前导零；不是Gin联合证据。Admin类型检查/构建与原19项浏览器回归通过。证据见 device-intake-report.json、device-intake-joint-report.json。历史照片迁移、跨SKU导入、仓库员工设备操作、配货/发货/取消释放和前台履约仍未完成，整个前台目标保持未验收。
