# 回收拒价退回与业务邮件联合

2026-11-03，C08/C10补实际回收设备入库→客户拒绝终价→退回发货→确认送达→业务邮件的联合证据。沿用原业务操作、前后台样式和人工运输登记含义，本批只扩展隔离验证运行器，不新增转账或承运商接口。C01—C11继续开放。

```powershell
# 首次联合：独立MySQL副本、Redis、Gin、Astro与React Admin
python retail/scripts/test-local-customer-account.py --scenario=buyback-return-mail
# 以后只核对保留的前后SQL，不重跑问卷、入库或退回
python retail/scripts/test-local-customer-account.py --verify-buyback-return-mail=684bfe01c6ac
```

## 页面与邮件

owner `684bfe01c6ac` 的实际浏览器完成新账号注册验证、原问卷、照片重拍/审核、寄件/收件、后台新建来源/仓库、带照片Recycled设备入库并关联验机。客户拒绝270.01 EUR终价并申请退回，后台登记RETURN-JOINT后确认送达；前后台刷新均为returned和closed_at，付款凭证为空。客户手机390px、后台桌面1440px，英文场景；没有声称本轮全语言或原站像素对照。

六个正式事务邮件事件分别是 `review_photo`（要求重拍）、`approve_shipping`、`mark_received`、`final_offer`、`mark_returning`、`mark_returned`，kind均为buyback.updated。复用现有正式投递器与独立测试进程，只替换外部Resend HTTP传输且限制example.invalid收件人。六并发领取只发送一次请求；首次拒绝后，在实际Ant后台点击重试并丢弃成功响应，再次提交同一UUID/版本恢复。冻结邮件字节与幂等key保持，最终六封sent、六条渠道收据及一条后台重试审计。原收件归属、发件配置、过期防护矩阵也通过；后台不暴露冻结信封或密钥。

## 持久核对与失败保留

[公开验收摘要](buyback-return-mail-native-report.json)保留初始运行器失败：浏览器和六封邮件通过后，SQL预期误写成approve_photos/request_retakes，实际后端动作是approve_shipping/review_photo。没有修改生产业务来适配错误预期。修正验证后，从保留的snapshot.sql和buyback-final.sql分别恢复独立临时数据库，复用相同的六事件断言并补核所有终态数据；无需重走业务主链。

持久核对通过：returned/270.01/RETURN-JOINT/closed_at且无付款凭证；三份回收照片字节；唯一实际入库设备、来源/入库收据、设备照片及回收关联，预置设备为0。设备仍保持原recycled/inspection状态，没有自动转为可售或修改数量库存。前后SQL中的付款、订单价格/身份、归属、数量库存/预留及退款指纹保持，主库目录/设备内容指纹和客户/回收计数不变。

原始浏览器报告、截图、投递日志、前后SQL及 `buyback-return-mail-verification.json` 位于忽略目录 `logs/retail-native/account-684bfe01c6ac/`；原失败报告不覆写。首次环境和两次SQL核对恢复库均清理，保留私有SQL以便后续核对；快照失败时保留隔离数据库供恢复。

Python编译、JS语法及diff检查通过。主Gin 9100 health、Admin 9200回收页、Astro 9300出售与本人回收入口均200；近期服务日志无新增异常。旧Medusa 9001 health/app仍无监听，属于已停用旧服务。本轮不发真实邮件、不转账、不调用承运商；真实邮件送达/HTTPS、正式数据与历史文件组合、其余订单事件联合及最终整期验收仍待。
