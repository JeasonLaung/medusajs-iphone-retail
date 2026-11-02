# Windows 原生回收主流程

2026-11-02，当前恢复库的 buyback_request_runtime / buyback_admin_runtime 均关闭，且申请和价格表为空。本轮补显式本机启用配置，并将原回收联合场景接到 Windows 原生 Gin / Vite Admin / Astro、独立 MySQL 恢复副本及原生 Redis 私有命名空间。业务 API 没有浏览器拦截或模拟。

两个独立场景均从正式注册、加密邮件意图的验证链接和登录开始，经原型号/容量/九步问卷、联系地址提交申请。实际浏览器上传照片，后台拒绝未审核照片直接批准寄件，要求正面重拍；客户补传后重新提交，后台审核并批准寄件，客户填写单号，后台确认收到。随后关联身份/型号/容量匹配的待检测设备，发送270.01 EUR报价：

- 接受分支：客户接受，后台登记 TRANSFER-JOINT，双方刷新保持 paid 和 closed_at。
- 拒绝分支：客户拒绝并申请退回，后台登记 RETURN-JOINT，再确认送达，双方刷新保持 returned、退回单号和 closed_at。

两个场景均检查站内通知合并、照片刷新可解码、关联不发送通知、报价提醒唯一，零浏览器异常。额外 SQL 核对最终状态、精确金额270.010000000000000000、closed_at、三份私有照片字节及一条设备关联。最终 owner50cc76a235f7/d38cb72ff6e0 的[报告](local-buyback-main-report.json)记录 binary SHA 和各11项 Redis 键清理。临时数据库/授权均0、19100/19200/19300已无监听，源账号及回收九表计数前后一致且均0。

运行命令（根目录，依赖与[账号子流程](local-customer-account.md)相同）：

```powershell
python retail/scripts/test-local-customer-account.py --scenario=buyback
python retail/scripts/test-local-customer-account.py --scenario=buyback-return
```

场景复用已提交的 buyback.joint / buyback-inspection 页面操作，测试报价294 EUR、设备SN-JOINT及人工IMEI只在自有副本插入，未修改主库产品或设备。原目录有30款商品，Ant Select虚拟列表不保证iPhone13已渲染，因此使用既有搜索输入选择型号；没有改页面或取消虚拟列表。第一轮运行器入口/生成问题及直接点击未渲染选项的失败证据保留在忽略目录。

`configure-local-preview --buyback` 仅允许 development + 127.0.0.1:13307 + eurorefurb_local_trial，锁定并核对两个runtime version1后在同一事务启用。当前本机已执行并验证幂等重放；忽略的 backend/.env 增加 LOCAL_PREVIEW_BUYBACK_ENABLED=true，start.bat下次启动重用，.env.example默认false。该配置不插入价格、设备或申请，不发信，不转账；缺价继续显示原“待估价”行为。CLI测试编译/vet、buyback/customer局部Go测试、Python/JS/PowerShell语法检查通过。无需重启当前API，正式处理器从SQL读取开关；健康、后台回收、前台问卷和账号回收页均200，近期日志正常。

这是本地申请到终态的业务登记验证，**没有实际转账、承运商寄送或邮件收件**。注册验证链接由测试进程认证解密副本的邮件意图取得，不代表客户收到邮件。两个分支是英文手机客户页和英文桌面后台；不宣称本轮全语言/宽度对照。主库仍需实际发信配置、业务回收报价及真实设备登记；外部履约凭证、历史图片迁移和C01—C11整体交付保持开放。

## 2026-11-03：实际设备入库接入回收验机

新增独立子流程 `python retail/scripts/test-local-customer-account.py --scenario=buyback-intake`。保留原来的两个预置设备场景，新增场景只在副本设置294 EUR测试报价；设备及来源通过现有后台正式操作创建，不再用SQL插入。客户仍从注册验证、问卷申请、照片重拍审核、寄件到后台收件；随后后台创建自有来源及关联仓库，选择 iPhone 13 / 256GB SKU，上传设备照片，登记 SN-JOINT / 000000000000001、类别 Recycled 的待检测设备，再回原回收单关联、发270.01 EUR报价。客户接受后后台登记 TRANSFER-JOINT，双方刷新保持 paid / closed_at。

最终 owner `ee4f2f66a1e6` 的[报告](local-buyback-intake-report.json)记录实际创建的设备、来源、仓库和照片标识及运行二进制SHA。SQL核对一台匹配身份的 recycled / inspection 设备、一张设备照片、唯一入库与来源命令回执，以及回收单关联同一实际设备；预置 `isolated-buyback-device` 为0。回收私有照片3份、设备关联1条、最终金额精确为270.010000000000000000。客户页、回收后台和入库页均监听浏览器异常，本轮为0。

主库16张目录/库存/设备表内容指纹不变，12项客户/回收/售后表计数前后均0；独立数据库、授权和11项Redis键已清理。API健康、后台回收、前台问卷和账号回收页均200，近期主服务日志正常。第一轮 `182567b67f43` 因目标SKU不在默认第一页而停在脚本定位，清理正常；脚本使用现有分页参数读取100项后，`989106db5aa2` 通过，补齐新入库页异常监听后的最终版本再次通过。未修改业务页面、服务或样式，未重跑购物车付款与发货主流程。

本轮证明实际入库设备可进入回收验机和付款登记，设备终态仍为 inspection，未新增转为可售或数量库存变更。没有真实转账、承运商寄送或邮件收件；拒绝退回分支沿用先前预置设备验收，尚不声称以实际录入设备重新覆盖该分支。C01—C11总体交付继续开放。

## 2026-11-03：实际设备拒绝报价并退回

新增 `python retail/scripts/test-local-customer-account.py --scenario=buyback-intake-return`，复用实际来源创建、照片上传、Recycled设备入库和验机关联；终价270.01 EUR由客户拒绝并申请退回，后台登记RETURN-JOINT，再确认退回送达。前后台刷新保持returned与closed_at，客户可见退回单号；没有登记付款，SQL付款凭证为空。实际运输和送达仍为人工业务登记，不调用承运商或自动修改设备状态/数量库存。

最终 owner `f3c35a20dc5f` 的[报告](local-buyback-intake-return-report.json)记录二进制SHA和实际设备/仓库/来源/照片ID，SQL核对设备仍recycled/inspection、照片1张、入库及来源回执各1、同一设备回收关联1、预置设备0、回收私有照片3份。浏览器异常0，主库16表内容指纹不变及12项计数前后一致；自有数据库/授权/11项Redis键清理。本轮仅跑回收退回子流程，未重跑支付或仓库发货链路，未改产品页面或样式。此前“实际设备拒绝退回尚未覆盖”的记录已由本轮补齐；真实邮件、历史文件组合、HTTPS及C01—C11总体验收保持开放。
