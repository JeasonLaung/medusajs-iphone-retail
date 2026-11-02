# Windows 原机维修主流程

2026-11-02，恢复库 customer_after_sales_runtime、after_sales_review_runtime、device_return_runtime、device_service_runtime 均关闭。本轮补专用本机的显式配置及独立维修场景，没有再跑完整购物付款流程。

`python retail/scripts/test-local-customer-account.py --scenario=repair` 创建当前恢复库副本、独立原生Redis前缀和19100/19200/19300服务；使用当前Gin binary、实际Ant后台和原Astro账号页面。副本中复制的是此前真实Mollie测试成单；测试进程在副本给新注册/已验证测试账号建立明确订单归属，**不是验证访客订单认领或已登录付款后自动归属**。正式接口没有增加按邮箱赋权入口，主预览的访客订单归属未变。

副本加入一台身份完整、S成色/90电池且与原成交SKU匹配的测试设备。浏览器实际预配、装包扣库存、发出原销售包裹，再由本人发起维修申请。后台处理为processing，扫描原机收货、上传检测照片、通过检测为repair_ready，登记维修返还单号，最后完成申请。客户刷新显示completed和维修返还SN/单号；原销售包裹及其原单号独立保留。本人站内通知包含发货、售后处理、售后完成共三条，全部标记已读后badge消失；私有检测说明/照片/员工字段不进入客户售后投影。

复用已提交的 checkout-repair 联合动作，在申请、收货、检测、寄回各首次成功响应后模拟丢响应，真实后端已提交；同一命令重试返回同结果。没有伪造业务响应或支付；这包含故障注入，不能称完全无拦截。旧联合场景已有一条通知读过，默认expectedUnread=2保留；本场景显式传3，检查本次实际未读状态。

最终 ownerfb73cd918986，[报告](local-repair-main-report.json)记录API SHA、真实SQL核对：申请completed、收机收据1、检测/返还收据2、设备shipped。源账号/回收/售后12表计数前后一致且均0，主库无测试设备。临时数据库及授权0、私有Redis11项清理，三个临时端口无监听。开发服务健康、后台订单和前台账号订单均200，近期主API日志正常。

`configure-local-preview --after-sales` 仅允许development及既定专用loopback数据库，事务锁定四个runtime并验证version1。当前主本机已执行并重放；忽略.env的 LOCAL_PREVIEW_AFTER_SALES_ENABLED=true 使start.bat下次重用，示例默认false。它只启用既有申请/审核/收货/维修返还，不写订单/设备，不启用退款worker或外部付款。开关由API实时SQL读取，无需为配置强制重启。devices/customer/CLI局部Go测试、CLI vet、Python/JS及PowerShell语法检查通过；其他需显式SQL环境的套件未在本轮全跑。

初次登录导航、测试设备空成色导致与S SKU不符的409、未读通知固定断言失败证据均保留在忽略目录，修正测试前置条件后通过，未删除或放宽业务约束。过程中也观察到开发StrictMode并发读取时权限或service GET偶发503，后续读取200且本次页面完成；原因尚未定位，不声称全程请求零错误。

本轮覆盖英文手机客户页与英文桌面后台，不声称全语言/宽度对照、真实邮件收件、承运商送达、退货退款或换机分支已验收。账号邮箱验证使用副本加密邮件意图取得链接，没有发信。独立售后主流程通过不代表完整C01—C11完成；下一步继续退货/换机及读取503排查。
