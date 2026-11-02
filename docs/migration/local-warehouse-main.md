# Windows 仓库员工履约主流程

2026-11-02，当前恢复库的仓库用户及范围runtime均关闭，实际 `/api/admin/warehouse-users` 返回503。新增 `configure-local-preview --warehouse-users`：严格限定既定development、127.0.0.1:13307及eurorefurb_local_trial，事务核对两个runtime版本1并启用。当前本机重放两次，主warehouse_user_accounts仍0。忽略.env的LOCAL_PREVIEW_WAREHOUSE_USERS_ENABLED=true由start.bat显式读取，示例默认false；不自动创建员工、权限、设备或改库存。API实时读取SQL配置，无需重启。

可独立运行 `python retail/scripts/test-local-customer-account.py --scenario=warehouse`。运行器复制当前恢复库，启动19100/19200/19300专有服务及Redis前缀，测试结束清理。副本保留真实Mollie测试订单，给新注册验证账号明确建立测试订单归属；这不是本次重新付款或自动归属验证。两台同SKU S/90测试设备仅存在于副本。

管理员实际预配原机、装包，创建只授权European Warehouse及履约读取/配货/发货三项动作的员工。独立390宽浏览器登录员工账号，读取全店订单/详情、仓库用户管理及实际其他仓库均403。范围接口只返回European Warehouse。员工在原包裹将原机换为备用机、再还原原机；释放原机取消包裹、恢复库存；管理员重新预配、装包；员工通过仓库专页登记物流、整包发货。平台初次预配/装包与员工包裹调整/释放/发货沿现有职责，本次不声称新增了员工直接创建初始订单/包裹的入口。

配货调整、释放、发货各在真实成功响应后丢响应，再按相同命令重试；请求内容及操作ID一致，实际后端响应没有伪造。员工刷新后仍见物流号、发货按钮禁用；客户本人订单见发货时间线、设备SN和物流链接、唯一发货通知并标记已读，移动页面无横向溢出。新测试复用原verifyCheckoutShipment客户验证。

最终owner96f2ab55d2af，SQL核对员工1、已取消包裹1、已发包裹1、原机及分配shipped、备用机available、库存49/预留0。源库存为50/预留1，说明取消恢复后重新装包净扣一次，配货替换及发货未重复扣库存。源账号/回收/售后12表前后一致且均0，13项Redis键及副本/授权/进程清理。详见 [SQL与浏览器报告](local-warehouse-main-report.json)。

初轮关闭runtime、Ant受控checkbox的即时状态检查、故障注入路由尚未完成即卸载、重新预配后历史/当前同device_id两行造成严格定位失败均保留忽略证据，修正测试等待和定位，不降低业务约束。Go仓库用户/授权及配置命令检查、配置命令vet、Python/JS和PowerShell语法检查通过；共用维修入口另做必要回归。开发StrictMode读取偶发503仍待定位。

本次仅英文桌面管理员/手机员工与客户，未验证实际承运商、邮件收件、全语言宽度和全部仓库业务。主本机开关可用不等于C04/C07或C01—C11整体完成。

共用维修回归ownerfad9c8169097通过且清理完成。主本机登录后只读打开仓库用户页面，新建按钮可见、空目录API200且员工仍0；health/后台仓库用户/前台订单均200，近期主API日志正常，临时端口无监听。
