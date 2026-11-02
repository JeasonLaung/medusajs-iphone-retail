# 实际设备入库接续已付款订单发货

2026-11-02，此轮从原已付款客户检查点790ce0fc9e71继续，不重走购物/创建远端付款。复用原订单order_2f0cb924-7757-418e-87b3-542ede6b250a及自动归属，关闭副本付款创建/上下文入口。实际成功owner a27835e587f6，业务API未替身，没有SQL插入设备、来源或身份归属。

新增固定入口：

```powershell
python retail/scripts/test-local-customer-account.py --intake-warehouse-from-checkpoint=790ce0fc9e71
python retail/scripts/test-local-customer-account.py --verify-intake-delivery=a27835e587f6
```

原paid-warehouse使用SQL准备设备，不能证明实际入库到订单。新场景跳过那段准备，实际Ant创建自有来源/新仓库、现有SKU单台追踪、上传照片、整批登记两台销售available和一台退回inspection，前导零IMEI保留，另通过原库存界面记录新仓现货2。批次、照片和后续配货来自实际命令。

过程中发现新来源虽自动关联渠道和manual_manual提供方，但没有配送方案关系，无法满足原装包必须匹配实际仓库/服务区域的规则。新增仓库表“配置配送”抽屉，显式关联已有shipping配送方案；不自动给所有仓库绑定、不创建新配送价格或放宽装包条件。原Medusa device-source工作流只创建仓库/渠道/provider，原平台仓库配送配置需单独操作；Gin现补此必要运营入口。

私有GET/POST /api/admin/warehouses/:id/delivery-sets复用仓库读写权限和门禁、全局revision与warehouse_changes收据，限制当前仓库/已有未删除shipping方案，严格JSON/UUID/版本，当前权限先于重放检查。事务写关联、前后投影、不可变操作收据并推进revision。软删组合主键关系恢复原行，不新造重复关系；晚失败回滚。仅提供关联已有方案，本轮不实现配送方案创建、删除或配送价格编辑。

实际UI给新仓库关联European Warehouse delivery，首次成功响应故意丢失后同指令重试，一条关联、一条收据。订单管理员按SN配货，创建包裹；新限定仓库员工不能读取全店订单/其他仓库，实际替换备用销售设备再还原，释放整包后管理员重新配货装包，员工发货并原指令重试。客户查到同一原订单的设备SN、物流链接/时间线和通知已读。新仓数量2→1，备用设备available、退回设备inspection；原European stock保持50，预留2→1仅消耗此订单，保留其他订单的1。取消包裹不展示为有效发货。

SQL核对仓库用户1、取消包裹1/有效发货1、发货设备1/备用available1/新仓stock1、批次1/照片1/退回inspection1；既有付款、订单身份及自动归属保护指纹不变。10条专属Redis键、进程、副本及授权清理，最终SQL检查点保留于忽略目录。原付款检查点及主订单均未发货。

最终源代码增加软删恢复后，没有重跑整条业务。恢复同一完成快照，只运行当前warehouse生产服务：六并发同操作fresh1/replay5、旧revision拒绝、同UUID改内容拒绝、仓库主体全店入口拒绝、写关联后收据失败完整回滚、恢复软删关系仅1。付款/归属保护指纹仍不变，无付款/配货新执行。Go严格wire拒绝重复/缺失/未知/null/无效UTF8/路径不符/非规范revision及超限正文测试通过。

最初ec838cb97a69继承旧快照库存创建门禁false，停在库存表单；场景现显式配置该门禁。类型检查误用pnpm触发自动依赖整理，导致7712b61daa9a隔离Admin启动失败，已恢复原npm包目录、移走意外锁文件并使用node直接调用tsc。失败报告保留，不作为成功证据。现有主服务通过start.bat受控更新，未清构建缓存或运行第二主库API。

主后台新抽屉中英桌面1440/手机390只读检查通过：实际仓库/配送目录API200/private-no-store，原方案可见，打开取消、零写请求/浏览器错误/页面溢出。主库设备/照片仍0、来源2，原库存50/预留1、配送关联3/仓库变更收据0保持；补款/退款worker各1，邮件worker关闭。Go warehouse/server测试及vet、Admin TypeScript、脚本语法与diff检查通过。真实承运商、真实邮件、配送方案新建/其他运营数据和C01—C11其余要求仍待。
