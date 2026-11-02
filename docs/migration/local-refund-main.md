# Windows Mollie 沙箱退货退款接续

2026-11-02，本次完成真实渠道pending到refunded、账号自动归属、持久恢复、负向账本及客户退款通知验证。完整C01—C11仍未完成。

`python retail/scripts/test-local-customer-account.py --scenario=refund` 创建当前恢复库的独立副本、独立Redis及19100/19200/19300服务和单独编译的正式退款worker。仅允许本机配置中的test key，SQL退款runtime仅在副本打开，主预览退款仍关闭。运行器关闭所有邮件发送，不改主订单/设备。

与维修/换机的复制订单夹具不同，本次新注册并验证账号后，通过实际商品页加购、地址/配送/付款、Mollie真实沙箱收银台PayPal测试paid回跳，创建独立订单 `order_755fe4e6-906f-434c-8dfc-450105a71286`。原项目确认页自动执行本人订单attach，接口200及客户详情200均有证据；未通过SQL赋权。首次脚本在异步attach完成前读到404，修正为等待真实归属结果；不为此重新付一笔。

此新订单在副本后台实际配货、装包、发货，本人申请return，后台扫描收机、照片检测使设备available，正式退款接口接受789.00 EUR指令，独立worker向Mollie真实测试API创建退款 `re_kM2DLostBExnPFCouZzYJ`。渠道初始回读只有一笔退款、mode=test、amount=789.00 EUR、status=pending。未对原主预览订单的远端付款退款。

Mollie退款有独立状态，pending不等于refunded；参见[官方状态说明](https://docs.mollie.com/docs/refunds)。worker保存原远端退款ID，重启/恢复后只回读该笔，不重新POST。第一次恢复owner d996b8c7712f浏览器检查现有指令金额/UUID、提交按钮禁用，尝试完成售后收到409，客户售后仍processing、没有timeline-refund；本地payment_refunds及负向order_transactions均0。

再次恢复owner d9bf3fae2419时渠道已返回refunded，正式worker将唯一退款、负向交易、汇总和通知记账。后台完成申请200、客户售后completed，退款timeline和通知跳转/已读正常。SQL核对refund_instruction、refund_ledger、negative_transaction、refunded_summary、read_refund_notice及automatic_ownership均1；原预览订单的退款指令仍0。最终独立认证GET核对Mollie仍只有同一笔退款、refunded、789.00 EUR。没有手工改本地或渠道状态来通过检查，测试后台只做过只读检查。

运行器finally将完整持久SQL保存到忽略目录中的refund-final.sql，再清理临时库、授权、Redis和进程。该文件含加密配置/测试账号等私有数据，不提交Git；保留以便渠道状态变化后恢复。若快照失败则不删除临时库，并报告清理失败。恢复必须核对专属数据库、已验证账号订单归属、固定原操作，不能恢复到主预览库。

```powershell
# 创建新的独立付款及退货退款；仅在需要新场景时使用。
python retail/scripts/test-local-customer-account.py --scenario=refund
# 继续已存在的指令，不重新购物、收机或提交退款。
python retail/scripts/test-local-customer-account.py --resume-refund=d996b8c7712f
```

恢复点及SQL/清理证据见 [报告](local-refund-main-report.json)。初轮付款归属等待、pending超时和恢复脚本断言失败均保留在忽略日志；修正测试等待/恢复账号和登录前置条件，未放宽业务条件。新建入口允许明确报告pending并保存检查点，旧联合脚本默认仍要求Posted。仅覆盖英文390客户页/1440后台；真实邮件、承运商、其他语言/宽度及完整C01—C11仍开放。读取偶发503仍待定位。

共享维修子流程owner0188e03ebe07回归通过；Python编译、三份JS语法和diff检查通过。主health、后台订单、客户订单及商店均200，近期主API日志正常；三个测试端口无监听。主预览退款开关仍关闭，本次仅在独立副本启用和运行worker。
