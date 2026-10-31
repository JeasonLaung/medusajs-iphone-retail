# 新购物订单到同 SKU 换机发货

本模式继续上一批新购物订单的实际履约联合，复用原换机命令和后台操作，不预造换货、退货、商品行、分配、包裹或通知。支付和邮件仍使用隔离适配器，不能据此验收真实外部收款、补差、退款或收件，也不能标记 C01–C11 全部完成。

`CHECKOUT_EXCHANGE=1` 与维修模式分开运行，要求 `CHECKOUT_FULFILLMENT`、`CHECKOUT_ACCOUNTS`、`CHECKOUT_FRESH_CART`、`CHECKOUT_PRODUCT_BROWSE` 都为1。隔离栈仅额外准备四台同 SKU 可售替换设备；原四个新注册客户仍在实际 Astro 完成注册/验证/登录、商品加购及 EUR158.75 付款成单，再经实际平台装包、仓库解除/重配/发货。

客户本人选择已发货原机申请换货；平台实际受理、按 SN 扫描收回，再在既有抽屉选择同 SKU 替换设备。原 Gin 命令在同一事务创建原生换货/退货单、两个订单版本与操作、新商品行和替换分配。平台给替换设备装包，由原指定仓库账号发货。原机单独上传检测照片并通过检测后完成申请。客户刷新本人订单，查询两个包裹及各自物流、原机 returned 与替换机 shipped/replacement_of 的关联、完成状态和通知，并标记已读。

申请、收回、替换分配、替换发货和检测均在首次成功提交后丢弃响应，实际页面重试完全相同的命令正文；不替代实际业务 API 响应。重试等待失败反馈与 loading 结束，避免把提交中的临时重试按钮误当作已失败状态。英文/中文 × 390/1440 各运行一次；切换其他账号与退出后的订单拒绝仍沿原联合检查。

后置SQL按同一四单验证唯一申请/收回、换货与关联退货单、两个已确认变更、唯一替换分配、检测照片、两个发货收据、一取消及两发货包裹、本人四条已读通知。初次与替换发货净扣8，新增预留最终0，原机检测不增加数量库存。等价换机保留 EUR158.75 实收、退款0、差额0，不创建额外未付集合；不能用手工状态模拟补款或退款。

运行：

```sh
CHECKOUT_PRODUCT_BROWSE=1 CHECKOUT_FRESH_CART=1 CHECKOUT_ACCOUNTS=1 \
CHECKOUT_FULFILLMENT=1 CHECKOUT_EXCHANGE=1 \
STOREFRONT_TEST_NODE=/opt/homebrew/bin/node \
ADMIN_TEST_BROWSER='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' \
python3 retail/scripts/test-customer-orders-live.py --checkout
```

独立报告为 `shopping-exchange-joint-report.json`，只能在脚本完整通过后生成；失败日志不代表已验收。仍须继续非零差额在线支付/真实退款、仓库专用售后及全部交付范围其余闭环。

## 本机验证结果（2026-10-31）

最终交换模式完整脚本退出0：四个中英/390与1440新购物订单，浏览器错误0，后置SQL与完整 TestOwnedCartRuntime 118.16s通过。报告绑定21份相关源码SHA-256，逐项核对当前文件一致；自有容器/网络/二进制/浏览器资源清理完成。Go全包、customer/devices/orders/identity vet通过。实际中英手机/桌面截图已复核，原销售与替换物流保持可读、无横向溢出。

初版SQL误把原机检测通过后的类别预期为returned，实际原业务 `serviceState(pass,false)` 为sale/available；最终改为校验该真实语义，同时保留原分配returned与数量未回补断言，没有修改业务代码迎合测试。共享重试helper现等待实际失败反馈及loading结束，搜索候选后使用现有键盘交互，避免提交中误重试和下拉虚拟列表视口竞态。

最终同版源码关闭换机、打开原维修模式重跑，中英×390/1440、后置SQL与完整TestOwnedCartRuntime 101.52s退出0，资源清理完成。维修报告同步由脚本更新；换机/维修两报告21份相关源码SHA逐项匹配当前文件。旧单独履约/购物账号及完整SQL报告保留上轮独立快照，本批未伪造更新；原health/Admin/store均200，近期日志正常。
