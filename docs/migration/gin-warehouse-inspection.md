# 仓库员工退回设备检测

2026-11-03，补 C05 中实际缺失的“退回入库→检测→转销售可售”操作。原行为来源为 `medusa-store/apps/backend/src/workflows/device-operations.ts` 的 inspect 分支及 `src/api/warehouse-guards.ts` 分类授权。不是由通用库存业务推导的新功能。

## Gin 与 Ant

增加私有 GET/POST `/api/admin/warehouse/locations/:location_id/devices/returned/:device_id/inspection`。仓库员工须有当前退回读取/写入及仓库范围，员工全店商品接口仍拒绝。复用已有 device_service_runtime/version1、设备身份锁、严格 ServiceInput JSON 解码、照片验证、单台库存SKU校验及 warehouse_device_changes 唯一收据；无DDL，不自动给旧账号增加权限。

设备须为 returned 且处于 inspection/repair，没有 reserved/packed/shipped 活跃分配，当前位置有有效来源。支持原 pass/repair/retire/supplier_return 结果，记录目标SKU、实际成色/电池、检测说明及检测照片。pass 复核配置和号码占用后转 sale/available；正在 processing 的 repair 原机必须保留原SKU并进入 repair_ready；supplier_return 仅原来源为供应商时允许。数量库存、预留、支付和售后申请状态不由本命令修改，检测照片记录在事件中，不覆盖原入库照片。

当前设备位置、分类、最新 returned 分配及关联维修申请状态参与读取/修订。事务内重新核验账号/范围，再查已有收据；移仓或撤权不能利用旧收据继续操作。原UUID/正文重放恢复原结果，异正文/过期版本拒绝。设备字段、检测事件、全局修订和收据一笔事务，末尾失败回滚。

“仓库设备”退回列表增加“检测处理”，Ant Drawer复用照片上传，按权限使用仓库限定的SKU搜索/分页，不读取全店商品、价格、客户或成本。表单保留原照片/数量库存语义，中英反馈与同命令重试；不改变既有列表和其他操作的样式。

## 局部实际验收

运行 `python retail/scripts/test-local-customer-account.py --scenario=warehouse-inspection`。使用自有SQL恢复副本、独立Redis和19100/19200/19300服务；需先编译当前API至自有logs目录并设置 `ACCOUNT_TEST_API_BINARY`，例如 `go -C retail/backend build -o ../../logs/retail-native/retail-api-inspection.exe ./cmd/api`。

最终 owner `481b7a19157b`：真实管理员创建商品/SKU/精确价格、来源自动建仓、照片及两台设备入库，登记1件数量库存；创建只授权本仓销售读取、退回读写的员工。实际手机页面在其他SKU下记录repair及照片，再选回原SKU通过检测，刷新后销售列表见同一设备 available，前导零IMEI保留。并发六次首次命令只有一个非重放；保存成功后丢响应，原请求重试及六路重放均不增写。异正文、过期修订、跨仓、全店商品及自有来源退供应商拒绝。

隔离触发器只在特定说明的最后收据写入故障，检查同一修订未变；SQL最终恰3份检测收据/3个inspect事件。原销售设备和处理后的退回设备均available，退回待检0，照片2。管理员移除退回写权限后旧会话401，重新登录只读且同收据POST403。中英文390px列表无页面溢出或浏览器错误。检测前后完整SKU数量库存/预留API响应一致；实际Astro中英文商品页仍可购买。

随后仅在外层回滚事务加入明确的repair订单/分配夹具，直接调用同一Gin业务函数：目标其他SKU具备匹配的成色/电池仍拒绝，原SKU进入repair_ready且同命令可恢复，维修申请状态变化使旧摘要失效，移仓及撤权后旧收据拒绝。补核全部回滚，不计入最终3份收据；不是本次重新付款或真实客户维修。

主库29张商品/库存/设备相关表指纹及12项账户/回收/售后计数不变，自有副本、权限、进程及6条Redis键清理。Go相关包测试/vet、Admin类型检查及脚本语法通过。报告见 [local-warehouse-inspection-report.json](local-warehouse-inspection-report.json)。早期两轮文件输入定位、mysql CLI复合触发器分隔符错误保留忽略日志，修正后实际联合通过。

仓库员工一般设备编辑、批量操作及C11历史照片对账等仍未完成；不据此关闭C05或整期。

当前本机通过原 `start.bat` 对应脚本受控更新三端及原支付/退款worker，无新迁移。主只读仓库用户页面200；全店管理员访问仓库专用检测接口403，证明新路由注册且不误授角色。主devices0/sources2/warehouse_users0/业务邮件队列与投递0，四项native健康/后台/商店/商品地址200，近期日志正常。旧Medusa 9001/8000按此前用户切换保持停止。
