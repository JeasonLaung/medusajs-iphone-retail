# Windows 仓库与设备入库主流程

2026-11-02，实际本机warehouse_management_runtime保持version1/revision1，但enabled=false；仓库员工开关已启用并不启用仓库管理。实际GET /api/admin/warehouses返回503，仓库来源按钮不可用，因此现有带照片入库实现无法由用户完成前置操作。

新增configure-local-preview --warehouse-management，严格限定development/TCP 127.0.0.1:13307/eurorefurb_local_trial。同一事务检查仓库version1/正revision及既有设备version1/正revision/identity_enabled/commands_enabled，开启仓库管理；不重置revision，不自动开启未就绪设备开关，不创建仓库、来源、设备、照片、库存或预留。production和错误数据库实际命令拒绝。本机忽略.env设置LOCAL_PREVIEW_WAREHOUSE_MANAGEMENT_ENABLED=true，start.bat接显式配置。当前通过同一命令应用，无需重启已有服务。

固定子流程：`python retail/scripts/test-local-customer-account.py --scenario=device-entry`，复用商品录入运行器，仅副本开启仓库管理。成功owner fefa0afb0618，通过真实Ant登录及已有商品/SKU/价格命令创建独立iPhone SKU，继而创建自有设备来源（自动新仓库/渠道/人工配送提供方关系）、开启单台追踪、上传PNG由原组件压成JPEG，实际GET核对JPEG头与类型。

实际整批登记两台SN/IMEI：NATIVE-INTAKE-SALE/000000000000081为sale/available，NATIVE-INTAKE-RETURN/000000000000082为returned/inspection；前导零保留。第一次成功POST故意丢响应，原页面“重试本次保存”复用完全相同请求，只有一条入库收据、两台设备。刷新后读取原照片，中英手机实际入库页没有页面横向溢出。

设备身份登记沿原行为不增加数量库存。在该来源新仓库用原库存界面另录现货1、在途0，公开API库存1，同商品Astro /dk-en及/dk-zh手机页可购买；API价格129.123456789012345678保持精度。此轮没有创建付款/订单、实际分配或出库；可购买不证明设备已配货。已有付款检查点的仓库配货/发货证据仍独立存在。

SQL核对产品/SKU/价格/库存层收据各1、追踪1/整批入库1/sale available1/returned inspection1/照片1。源十六张商品/价格/库存/仓库/来源/设备/收据表数据指纹不变，十二账户入口表仍0。副本、授权、三个进程与4条Redis键清理。源设备0、照片0、来源2及原European库存50/预留1保留。

早期运行ee6cf9cf0d7f的真实503暴露仓库门禁缺失；修复后fbde643c1938实际录入成功，但页面goto等待所有资源load超时。改为等待DOM就绪，再继续显式标题/购买按钮/无溢出断言；最终运行直接通过。失败记录保留，没有用超时重启服务或篡改业务状态。

主后台 /warehouses 中英1440/390只读检查：仓库目录和设备来源API200/private-no-store，新建设备来源入口可见后取消；仓库/来源写请求0，浏览器错误0。Go配置/devices/warehouse测试、配置vet和JS/Python/PowerShell语法检查通过。主服务健康和近期日志另作收尾核对。完整仓库范围、入库设备到实际配货、历史文件及C04/C05/C11其余要求继续开放。
