# Windows 商品运营入口与实际联合验证

2026-11-03：当前专用预览的商品图片、共享选项、商品选项、SKU 编辑和库存项关联已有实现，但 catalog_command_capabilities 中五个命令仍关闭。本次接通既有编辑入口，未改业务规则和界面样式。

`configure-local-preview --catalog-operations` 仅允许 development、127.0.0.1:13307、eurorefurb_local_trial；事务锁定五条命令并校验 version=1，仅更新 enabled。缺失或版本不符整批回滚。启动脚本在 `LOCAL_PREVIEW_CATALOG_OPERATIONS_ENABLED=true` 时显式执行，示例默认关闭。本机忽略配置已开启。连续执行两次，18张商品/价格/库存/订单/设备及SKU价格运行计数表指纹相同，不重置编号或修改内容。API按原门禁读取即可，无需重启业务服务。

## 局部验收

`python retail/scripts/test-local-customer-account.py --scenario=catalog-operations` 创建独立SQL恢复副本、专属Redis及19100/19200/19300三端，复用原商品录入流程。所有业务API和页面使用真实Gin/React/Astro，未mock。

最终 owner `011fe7390a47`：实际后台创建发布商品、Storage选项、ERP SKU、精确EUR价格129.123456789012345678及3件初始库存；创建共享选项并读取核实；上传2×2 PNG、设置商品缩略图、逐字节读取；原商品选项追加512 GB，SKU选256 GB并修改材质，ERP编号保留；关联SKU图片及缩略图；原库存项关系required_quantity改为2，link_id不变，公开可售数量变为1，价格精度保留。中英文390px实际Astro商品页加载上传图片并保持购买按钮可用，无溢出或浏览器错误。

SQL创建收据各1；图片收据2（商品及SKU），商品选项/SKU编辑/库存关联收据各1。主库29张商品/设备/图片/选项/库存关联相关表及12项账户/回收/售后计数均保持不变；副本、授权、4条专属Redis键和测试进程清理完毕。报告见 [local-catalog-operations-report.json](local-catalog-operations-report.json)。早期失败仅为无效图片夹具、Ant下拉定位和decimal断言，保留忽略目录原始失败日志；修正后完整局部场景通过。

主预览现有商品仅通过 `retail/admin/tests/local-catalog-read.live.mjs` 登录读取，阻止业务写请求；核对五项能力及SKU图片，打开编辑器再取消，覆盖中英文1440/390px。此检查不创建或修改主库商品。

本次不是完整迁移终验。C03其他状态与C01—C11剩余交付保持开放；历史图片引用等C11对账仍需继续。
