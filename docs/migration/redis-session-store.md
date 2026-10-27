# Redis 会话、登录挑战及限流组件

2026-10-27，T014/T019的组件实现。组件随后已接入[新后台联合路径](admin-auth-runtime.md)，旧MySQL会话运行时已移除；客户与完整权限/页面尚未完成。本文原始组件实证不独自证明HTTP能力。

`internal/sessionstore`使用锁定的`go-redis/v9 v9.22.0`。会话组件只接收显式Redis客户端和`eurorefurb:<环境名>:v1:`命名空间，后台`admin`和客户`customer`再分子空间，未知realm拒绝；没有默认连接旧Redis或读取旧会话的路径。环境名必须小写、长度受限，不能注入通配符/冒号。组件不提供FLUSH、KEYS或清空接口，不承担库存、付款、业务幂等及可靠任务持久化。

`Open`作为后续进程接入入口，仅接受显式redis/rediss URL及显式端口，拒绝URL query调参、fragment及非0—15数据库。连接/读/写/池等待有1秒上限、连接池8，Ping另有2秒context上限；启用context超时，不自动重试Redis命令，避免不确定INCR或一次性消费被客户端重做。连接失败关闭客户端并仅返回固定错误；没有内存/SQL降级。调用者负责关闭。TLS来源为rediss URL，尚未完成部署证书/ACL验证。

会话每次生成独立256位cookie capability和CSRF capability，Redis键只含cookie token的SHA256，记录只存CSRF摘要、realm、用户ID、调用者给出的64位十六进制权限/凭据版本、固定过期时间和格式版本。会话12小时绝对过期，不滑动续期；NX+TTL原子写入。失败/不确定写入不返回token，可能产生的不可达记录自行过期。只返回用户引用和版本；调用方必须每次核对MySQL当前账号状态和权限版本，不能把Redis记录当作授权事实。后台版本生成与当前账号复核随后已接入；账号更新/撤销管理事务尚未实现。

登录前挑战10分钟，cookie与CSRF分别交给后续HTTP层；正确CSRF经常量时间摘要比较后，Lua compare-and-delete单次消费。错误CSRF不删除挑战，16路并发消费只有一方成功。挑战不能用于Resolve正式会话。Gin已接入精确Origin及cookie规则；真实浏览器及React表单仍待验证。

登录限流使用15分钟固定窗口，每realm/调用者提供的身份材料最多8次；键只存摘要，Lua原子限额与首次TTL，拒绝请求不增长计数或延长窗口。24路并发仅8次获准。调用者仍需定义并组合IP、邮箱及全局限额策略；单一IP+邮箱组合不能代替全局滥用防护。Redis故障返回依赖失败，不放行；Redis丢失会话要求重新登录，限流临时计数也可能丢失，不能作为业务一致性的唯一约束。

隔离真实Redis测试：

```powershell
python retail/scripts/test-redis-sessions.py
```

仅使用本地已有`redis:7-alpine`镜像，独占所有权标签、单核128MiB、禁网、无映射端口、Unix socket、数据/tmp均tmpfs，无RDB/AOF。人工账号/值，不调用应用Redis。验证双realm及双应用前缀隔离、cookie/CSRF原文不进键/记录、NX生成、固定TTL、绝对和Redis实际到期、撤销及重复撤销、错误CSRF拒绝、16路单次消费、24路限流、坏记录拒绝、定向淘汰后不能恢复登录、连接关闭后的会话/发放/限流全部拒绝。临时容器标签核对后删除，临时编译物清理；[数量及代码摘要证据](redis-session-report.json)不含capability或私有资料。

普通Go测试另验证无效URL/namespace在连接前失败；`go test ./...`及`go vet ./...`通过。真实Redis测试当前用Unix socket验证组件，后续联合演练已验证`Open`有效TCP连接；TLS、进程启动关闭、服务端故障恢复和网络分区仍缺。故障用关闭客户端模拟，不冒充真实Redis宕机/重启实证。后续需接入新账号、版本撤回、Gin/CSRF/cookie、React Ant登录流程、真实MySQL+Redis联合HTTP验证。T014/T019继续未验收。

连接用法参考[Redis官方Go客户端文档](https://redis.io/docs/latest/develop/clients/go/)，实际使用的超时、重试和握手选项已核对本机锁定版本源码。未导入旧账号、密码或个人登录授权。
