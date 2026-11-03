# Windows 真实 Mollie 整笔取消后重付

2026-11-03，固定子流程 `python retail/scripts/test-local-customer-account.py --scenario=payment-retry`，最终独立 owner `3c7378601077` 通过。主数据库111迁移、原生Redis和现有Gin API源码，专用复制库、Redis前缀及19100/19300端口。API SHA256为 `1d80b284b7d7d56bd58ff350820f7e04e115d6862749b4527a343619d4086b10`。

前置选品、地址和配送通过生产CartClient/API按需准备，不重跑账户、配货或售后。实际390宽Astro付款页创建789 EUR托管付款，点击Mollie托管页 `#cancel-button button` 整笔取消；回到正式结果页由认证渠道GET确认终止。不能把支付方式模拟页的expired/canceled选择等同整笔付款终止：此前探针选择后回到支付方式页，认证GET仍open，没有据此声称通过。

正式结果页“Return to checkout”提交原付款上下文退役命令。仅丢弃真实200/saved响应，不伪造付款状态或业务API。刷新后同UUID、版本和载荷重放，正式服务返回原收据；核对当前cart/amounts/payment-options后清理两份持久意图并返回付款步骤。再次明确确认付款才创建替代会话，原cart不变、操作UUID不同、版本比首次创建后增加2。替代会话经Mollie测试Paid，正式认证查询后唯一成单，访客确认页刷新仍读取同一订单。

SQL核对：上下文收据1、checkout attempts2、成功create effects2、成功delete effect1、completion receipt1、访客账户归属0；替代成单精确绑定上下文result_version、原CREATE input_version和effective/cart版本。实收capture仅1条、金额789.00；认证GET观察旧付款canceled、新付款paid，均789 EUR。浏览器错误0，重试请求2次且UUID/载荷逐项相同。最终报告及保留SQL、浏览器状态在忽略目录 `logs/retail-native/account-3c7378601077`，提交的去敏摘要见 [报告](local-payment-retry-report.json)。

`--verify-payment-context=3c7378601077` 将最终SQL恢复至另一独立库，重查上述收据/版本/实收及原两笔付款GET，不再创建/取消付款。源账户12表计数保持、专属进程/库/授权/Redis键清理完成；主预览三端HTTP200且近期日志无异常。本轮没有修改产品代码或主业务数据，没有真实扣款、退款或邮件，测试Paid来自Mollie沙箱。

失败证据保留：`43680ed36490` 浏览器已完成整条子流程，旧断言错要求expired；`431e663166bf` 浏览器通过，新增SQL断言未对effective_cart_version的NULL回退到cart_version。修正验收器后，两份保留快照已独立复核通过。旧浏览器原文保留，第一份摘要明确修正expiration标签，以认证GET的canceled为准。`70466f7f5678` 取消/同指令恢复完成，导航到结账后30秒仍加载，未创建第二笔付款；后两次未重现。运行器新增console/network诊断、等待目标付款岛就绪后再移除请求拦截，未凭此宣称修复产品加载问题；稳定构建及更多布局仍待。

本项补真实Mollie取消后的重付，不覆盖真实expired/failed、未知创建/取消发现、PayPal沙箱、生产webhook或完整C01—C11。连续设置保存、金额变化和失败后原设置补偿证据仍见原三个付款上下文报告；全流程统一验收留大任务结束执行。
