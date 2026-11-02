# Windows 真实付款部分执行后的补偿恢复

2026-11-02，`python retail/scripts/test-local-customer-account.py --scenario=payment-context-restore` 通过，owner `790ce0fc9e71`。复用专有数据库/Redis/Gin/Astro的原连续上下文及真实Mollie付款路径；没有模拟业务API、手工设置paid或修改主本机价格。

在首次真实付款创建及两次金额不变保存后，购物车版本7、总额789 EUR。在副本将测试SKU779调780，并安装只匹配该购物车的BEFORE INSERT触发器，使最终上下文收据result_version写NULL，故意触发数据库约束失败。该触发器仅存在于独立副本，非产品功能/正式迁移。真实协调器处理旧会话后，最终保存503；SQL核对购物车版本仍7、原checkout绑定及refresh冻结仍在、上下文收据仍2、成功delete effect 1。未将目标中文或790金额误报为已保存。

实际390宽浏览器刷新、读取原持久意图，点击“Restore previous preferences”。正式补偿协调器按原冻结付款请求创建替代的789 EUR收银台，恢复英文原设置；此时保存失败触发器仍存在，说明恢复不依赖补造一条成功的目标保存收据。首次恢复200响应再次被丢弃，刷新后用相同操作ID和内容重试，200返回原恢复结果，购物车仅前进一次到8，原save意图最终清除，付款意图绑定该恢复操作/input version7。随后清除副本触发器并恢复目录原价，沿替代收银台实际测试Paid、认证查询、成单和客户自动归属完成。

订单 `order_2f0cb924-7757-418e-87b3-542ede6b250a` 仅在副本存在，本人订单页可读。SQL最终为上下文收据2、已完成补偿1/result version8、checkout attempts2、成功create effects2、成功delete effect1、按替代input7/cart8且completed9的唯一成单1、本人归属1。浏览器错误0。原保留引用认证GET为789 EUR/open/isCancelable=false；新引用为789 EUR/paid。本轮delete effect表示旧会话本地退役，不能据此声称远端已取消。旧链接对账及真实可取消DELETE/未知取消仍开放。

`--verify-payment-context=790ce0fc9e71` 可将保留的最终SQL恢复到新的专有库，只核对正式SQL事实及认证GET观察，不再创建付款；本轮也已重放通过。副本触发器已移除，副本、授权、专有服务、20个Redis键及复核数据库清理完成，源十二表计数不变。忽略目录保留SQL/浏览器状态和截图；提交[报告](local-payment-context-restore-report.json)不包含付款ID、密钥、cookie或原始渠道响应。第一轮多语句CREATE TRIGGER被mysql客户端分割导致测试准备失败，后改为单语句、限定cart的约束故障；失败运行保留且未冒称通过。

该验证覆盖真实渠道创建替代付款、故意最终SQL失败、原设置恢复和丢响应刷新重试。没有覆盖渠道取消结果未知、创建响应未知的发现恢复、金额/币种全部分支或生产webhook。C01—C11保持开放；主本机代码/运行配置未变，三端服务正常。Python及JS语法、差异检查通过。
