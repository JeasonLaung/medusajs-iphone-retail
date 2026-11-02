# Windows 本机客户账号子流程

2026-11-02，使用当前 Gin 预览 binary、Astro 页面与本机恢复库的独立副本验证注册至账号使用。英文1440、中文390各自独立数据库和 Redis 命名空间，实际业务请求没有被浏览器拦截或模拟。本轮没有改业务代码或样式，也没有再跑购物付款长链。

通过：注册202、未验证登录403且不发客户会话、验证链接200、登录200、本人订单空列表、不能读取已有 Mollie 访客订单404、姓名修改后刷新保持、地址创建/刷新读取/删除、退出后订单接口401、重新登录及手机无横向溢出。两组均零浏览器异常，各有一个通过正式接口验证的新账号。没有按邮箱授予历史订单归属。

**没有发送邮件。** 测试进程仅在独立副本读取注册产生的加密邮件任务，使用已有本机 keyring 做完整 AES-GCM 上下文认证，将链接写入 Git 忽略的证据目录，再由浏览器访问正式验证页；未通过 SQL 直接改 verified_at，也没有给正式 API 增加返回 token 的接口。这证明验证后的账号链路，不能证明收件或客户能自行取得验证链接。当前主预览发信 worker 关闭；需要可用的已验证发信域名与 Resend 配置，随后单独验收真实收件。

固定复现命令（仓库根目录）：

```powershell
python retail/scripts/test-local-customer-account.py
# 仅重跑受影响语言
python retail/scripts/test-local-customer-account.py --language=zh
# 回收主流程作为独立场景，详细范围见 local-buyback-main.md
python retail/scripts/test-local-customer-account.py --scenario=buyback
python retail/scripts/test-local-customer-account.py --scenario=buyback-return
# 原机维修，范围见 local-repair-main.md
python retail/scripts/test-local-customer-account.py --scenario=repair
```

依赖 Windows、现有专用 MySQL 容器及 `/tmp/local-root.cnf`、原生 Redis6379、`logs/retail-native/retail-api.exe`、前台依赖、Chromium1234、Node24与 Python cryptography。可通过 STOREFRONT_TEST_NODE 和 PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH 指定现有运行时。脚本只接受既定 development/loopback/专用源数据库配置，19100/19300有占用会提前拒绝。

前端复制到忽略的独立目录，依赖和公共资源使用 junction，Astro/Vite生成状态在副本内；不覆盖或重启用户9300服务。每种语言使用独立副本，避免一次连续测试消耗同IP八次/15分钟的认证挑战配额；保留原限流。初轮错误包括启动入口路径、Astro已有开发服务锁、依赖真实路径的Vite许可、两个场景共用挑战配额以及手机概览隐藏桌面主体的断言，原失败证据保留在忽略目录。手机重新登录以可见账号导航和真实 me200判断，不更改原响应式行为。

[最终报告](local-customer-account-report.json)对应 owner90c84af12fa4/5da372cb0b82，API SHA256已记录。两个临时库、数据库授权、各8项私有 Redis键和服务进程均已清理；复核残留库/授权均0、19100/19300无监听。源库账号/验证/邮件/地址/订单归属五表前后计数均0。9100健康、9200后台、9300账号及商店均200，近期服务日志正常。

仍未验证实际邮件收件、已登录客户使用真实 Mollie付款后的订单归属、全部中英文×双宽度组合或C01—C11完整交付。此前已有独立账号订单联合证据，本轮仅补当前Windows恢复环境的账号子流程，不把空列表等同于有订单查询验收。
