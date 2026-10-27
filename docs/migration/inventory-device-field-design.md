# 库存、设备与售后逐字段目标设计

日期：2026-10-27。T006 第二批覆盖12表139列，与目录批合计48表431列；其余145表仍待展开。设计清单：[JSON](inventory-device-field-mapping.json)、[CSV](inventory-device-field-mapping.csv)。当前没有已验收的 MySQL 表或导入结果。

## 覆盖范围与规则

| 源表 | 目标表及关键保留内容 |
| --- | --- |
| inventory_item | inventory_items：原ID、SKU、物流尺寸/原产地、需运输标记、单位、metadata、软删 |
| inventory_level | inventory_levels：inventory_item/location、stocked/reserved/incoming及各raw值；数量独立于设备登记数 |
| reservation_item | inventory_reservations：行项目/库存项/仓库、quantity/raw、backorder、外部及操作人历史引用 |
| stock_location/address | inventory_locations / inventory_location_addresses：仓库地址/metadata与软删，不忽略已删来源仓库 |
| device | devices：SN/IMEI/IMEI2/EID、SKU/来源/仓库、成色/电池/成本、类别/状态/照片和历史 |
| device_source | device_sources：self/supplier、location，保留渠道/仓库域引用 |
| device_variant | device_variant_tracking：设备追踪SKU与inventory_item配置，不能根据是否现有可售设备反推 |
| device_supply | device_supply_commitments：来源+SKU承诺数量，设置语义，不能直接覆盖数量库存 |
| device_assignment | device_assignments：设备、订单、行项目、来源、履约/退回/售后、replacement_of及物流号 |
| device_event | device_events：actor历史引用、动作、JSON证据、原时间，不重放旧事件 |
| after_sales_request | after_sales_requests：原订单与历史客户引用、return/exchange/repair、状态、reason和items封装 |

精确数字、UTC时间、JSON文本及字符比较采用 [目录字段设计](catalog-field-design.md) 的共同规则。本批库存数量为 DECIMAL(38,18) 设计，device.cost_cents 是源int32最小货币单位整数，目标BIGINT保留原值；不能当商品主要单位金额乘除100。设备供量quantity保持整数。源NULL保留，导入不补默认值；新命令默认值与业务合法性另外校验。

## 身份、历史与活动唯一性

源device.serial_number允许NULL是历史结构事实；新入库要求SN、trim并大写、最大100字符；IMEI1/2分别15位、EID32位数字文本，iPhone按产品策略要求至少一个无线标识。导入保留原文本和NULL，并单独报告不符合新写入规则的历史，不能虚构号码或删除设备。SN目标存储长度191，给历史数据保留空间；新API仍保留100限制。

源DB分别对未删SN/IMEI/IMEI2/EID建唯一索引，但跨IMEI列冲突及同机号码重复由工作流额外检查。新MySQL要通过事务中的身份注册关系或等价完整约束同时保证：同设备不重复、不同设备跨列不冲突、并发只成功一次；软删后重用与恢复冲突明确。不能仅复制四个UNIQUE就宣称身份规则完整。

活动设备分配仅status为reserved/packed/shipped且未删时对device_id唯一；returned/canceled保留历史不阻止后续合法分配。来源location、device_variant两个方向、supply(source,variant)、inventory_level(item,location)、inventory_item.sku、stock_location.address都是活动唯一。售后每order只允许一条pending/processing未删申请。MySQL活动生成键需覆盖这些条件，NULL语义、历史恢复和并发进入活动状态都测试。

device_event.actor_id、reservation.created_by、after_sales.customer_id只保留原主体历史引用，不建到新登录用户的外键。原系统system/sentinel引用允许作为历史，不能自动建员工账号。历史售后/订单不可按同邮箱赋权给新客户。

## 关系和JSON语义

订单行关系要通过order_item的order_id/item_id/版本核对，order_line_item本身是成交行快照；不能用order_item.id代替分配的item_id。换机replacement_of指向旧分配，不是旧device.id；保留原订单/售后/return及新订单行关系，维修返还不生成新销售。

售后items存储为对象封装，当前create-after-sales写`{lines:[...]}`，serializeAfterSales仅对外解包lines。不能把原JSON直接替换成数组并破坏工作流读取。新数据可能扩展跟踪/事件字段，未知合法键保留内部，nested item/device ID与当前订单逐项校验。照片保留文件ID、URL及顺序，公开/私有属性依文件清单验证；不能因有URL便直接公开。

库存预留与device_assignment是两套事实：前者记录库存item/仓库/数量，后者记录具体单机。登记、检测、供应商供量不自动改变库存；发货/取消按既有履约语义只调整一次数量。不能使用max(device_count,supply_quantity)或设备可用数替换源inventory_level。

## 实际快照预检及未解决差异

```powershell
python retail/scripts/map-inventory-device-fields.py --check
python retail/scripts/preflight-catalog-mapping.py --domain inventory-device --bundle logs/gin-migration/source-20261026.json
python retail/scripts/audit-inventory-device-snapshot.py --bundle logs/gin-migration/source-20261026.json
```

前两项检查字段设计和源标量/关联。快照55,692字段值、2,880 raw配对、5,506引用检查已运行；活动记录无所检查的缺失引用，**119条软删历史引用未解析**：device.source_id34条、reservation.line_item_id85条。全部属于已删行；设备的仓库仍存在，预留行引用也未在cart_line_item/order_item中找到。不得自动生成假来源/假订单行或丢弃历史。

迁移策略是保留原source_id/line_item_id文本，并为这些允许存在已知软删孤儿的关系增加可空resolved_source_id/resolved_line_item_id及独立migration_issues记录。设计清单的reference_policy只允许这两个字段在软删行有未解析引用；活动行同样问题仍为失败。可空resolved字段不替代原始引用，也不能被当成可操作关联。source值仍按字节保留；新写入必须有可解析对象。

第三项执行6,429次快照语义检查：活动唯一键、设备无线标识、预留库存位置、预留合计、配货订单归属/替换链和售后行归属。报告 [inventory-device-semantic-report.json](inventory-device-semantic-report.json) **有阻碍项，命令有意退出1**：一条inventory_level.reserved_quantity不等于有效reservation_item.quantity合计。另有3条reserved>stocked作为backorder待核对观察项，不能自动认定全部非法。

没有修改旧库存、预留或订单。未来隔离导入必须保存双方原值与问题账本，不能取较大值、覆盖合计、或重新跑旧reconcile“修好”数字。受影响item/location在新系统完成逐项对账前禁止新增库存消耗/履约写入；只读历史可继续查询。后续需要在隔离副本追踪reservation的已删/取消/履约关联及backorder，形成有证据的处置；当前不等于业务阻塞或需要用户先修生产库。

报告只有表/字段和计数，没有设备号、订单号、照片、地址、姓名、邮箱或原始行。所有当前结果针对固定快照，不保证运行中的旧库随后数据仍相同。新MySQL导入、身份唯一注册事务、私有文件、全部嵌套JSON和售后全流程仍待验证。
