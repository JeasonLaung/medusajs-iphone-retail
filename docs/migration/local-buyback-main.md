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
