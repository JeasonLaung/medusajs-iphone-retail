# Windows 真实换机补款与替换机发货

2026-11-02，从真实 Mollie 已付款检查点 `790ce0fc9e71` 恢复到专有数据库，原已验证客户、订单及成单归属保持。没有重新支付原789 EUR、按邮箱绑定游客单或SQL重建归属。沿既有管理员实际页面发货原机，客户申请换机，管理员审核/扫描收回后分配同SKU替换机。

副本专有测试税率1%仅在原机发货后加入，原订单快照不改；原生换机按新税配置产生商品779 EUR的7.79 EUR差额，当前订单796.79 EUR。税率只是构造原流程允许的欠款场景，不是商店实际税率或主库配置。未付清前，实际装包页禁用替换机，直接正式装包命令409。

原客户实际确认补款，正式POST202成功后丢首次响应，同UUID/同金额重试恢复一条指令。生产 `process-order-payments` 在专有进程创建真实Mollie测试会话，客户跳转真实托管页面，选择测试付款方式及Paid返回本人订单；worker认证GET后消费真实paid观察，产生唯一7.79实收、通知及零差额。没有把浏览器Paid选择直接记账、模拟提供方或改SQL付款状态。原生Mollie test模式所列PayPal仅为Mollie的托管测试方式，不是本项目独立PayPal集成验收。

结清后实际管理员装包/发出替换机，原机拍照检测并恢复可售，完成换机。客户看到原/新两台设备及物流、补款已收妥与五条本人通知，全部已读；390宽无横向溢出、浏览器错误0。实际使用既有Ant/Gin/Astro界面和业务命令，本批不改UI样式。

固定子流程入口：

```powershell
python retail/scripts/test-local-customer-account.py --supplement-from-checkpoint=790ce0fc9e71
python retail/scripts/test-local-customer-account.py --verify-supplement=c0094daec6c1
```

第一个入口会新建一笔真实Mollie沙箱补款，原未履约订单只能在自有副本中继续；第二个恢复已有最终SQL检查点，验证唯一事实，并对该已完成状态运行原生产worker `--once`。本轮第二入口未重放付款或仓库流程。若第一入口中断，保留最终SQL和已持久化补款ID，不能为重试检查而直接再造另一笔补款。

执行owner `c0094daec6c1`，原订单 `order_2f0cb924-7757-418e-87b3-542ede6b250a`，唯一补款指令 `7cccdcee-f937-495f-a898-f9a9abd25f08`。SQL核对唯一已应用paid补款、订单scope唯一渠道CREATE、原成单收据1、付款捕获2、正现金交易2、收款通知1、原归属1、完成换机1、已发包裹2。实收合计796.79 EUR，差额0、退款0。完成状态worker重启后十张交易/库存/通知表指纹不变，副本/授权清理。源12张客户/回收/售后表前后0，执行专属进程/数据库/8条Redis键清理。详见 [执行及重启SQL证据](local-exchange-supplement-main-report.json)。

第一次后置验证误用不存在的交易payment_id，MySQL将其解析为外层列造成不相关捕获计数16；已改为 payment_captures→payments→order_payment_collections 显式关联，保留原执行证据，仅恢复原快照重新核对。首次报告沿用exchange/人工归属字面量，源码现改为supplement及原归属；实际执行始终为resume原客户，原报告保留并注明。不能按该旧字段把实际归属说成SQL夹具，也不能覆写原报告假装未发生误报。

本机运行新增 `configure-local-preview --order-payments`：限定既定development、127.0.0.1:13307/eurorefurb_local_trial、Mollie test key及本机无webhook模式，要求已配置独立补款和通知适配器；事务核对补款/通知runtime均v1并启用，不创建指令或付款。忽略.env显式ORDER_PAYMENTS_ENABLED、MOLLIE_NOTIFICATIONS_ENABLED、LOCAL_PREVIEW_ORDER_PAYMENTS_ENABLED为true；示例默认false。start.bat构建并启动独立原生worker，不把worker塞进API进程。再次启动先读取记录PID、核实精确.exe路径，再停止旧worker并创建一个新实例；实际连续两次启动确认旧PID退出、仅一个worker存活。PID被其他程序复用时拒绝终止，不按全局进程名批量杀进程。

主补款指令/客户归属仍0，库存50/预留1，未制造测试税、订单或设备。退款/邮件worker仍不启动。loopback不能接收Mollie外部回调，本次通过已有worker持久恢复及认证GET完成；两次worker查询日志间隔约一分钟，客户页面期间明确显示等待，未声称即时webhook可用。

当前只验收英文390客户及1440管理员、单次成功Mollie补款，未知创建结果/付款失败后新尝试、生产webhook、真实邮件与承运商及全部语言布局仍有待继续。先前原付款检查点的旧open且不可取消会话风险仍保留，不因补款成功而宣称已经对账。C01—C11保持未完成。
