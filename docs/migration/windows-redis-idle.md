# Windows Redis 空闲后首次请求失败

2026-11-02 实际预览健康检查在闲置后第一次返回503，紧接着重查200。原生 Redis 只读 `CONFIG GET timeout` 确认空闲超时65秒；go-redis v9.22.0 的 Windows `conn_check_dummy.go` 不会在出借连接前探测 EOF，原默认连接池空闲时间30分钟。已关闭的旧连接会在下一次命令才失败。

修复 `sessionstore.Open`：客户端 `ConnMaxIdleTime` 设置30秒，先退休空闲连接再建连。`MaxRetries=-1` 保持，不能为消除一次503而自动重放会话写命令。没有修改共享 Redis 配置、清空缓存、刷新/删除其他项目前缀或重启 Redis。

Windows 专项命令：

```powershell
$env:SESSION_REDIS_IDLE_REHEARSAL='1'
go -C retail/backend test ./internal/sessionstore -run '^TestLocalRedisIdleRehearsal$' -v -count=1
Remove-Item Env:SESSION_REDIS_IDLE_REHEARSAL
```

该测试仅在显式启用时连接本机6379，要求原生 Redis 超时60—65秒；只读配置、PING，不写键。旧/新池并行保持100秒空闲（含服务端扫描宽限），核对旧池第一次失败/第二次恢复、新池第一次成功。普通测试不会等待100秒，不会隐式访问本机 Redis。

第一次临时探针72秒复现旧池首请求失败/次请求恢复。正式对照最初失败是测试夹具复制了已初始化的 Options：`MaxRetries=0` 再传 NewClient 会重新启用默认3次重试，掩盖旧连接首错。该轮保留失败日志；随后一次同夹具运行主动停止，更正为显式 `MaxRetries=-1` 后从头执行，不将失败/中止结果计为通过。

复查范围为 sessionstore/identity/customer/cart/cmd-api 测试及 sessionstore vet。本项不涉及付款渠道、数据库结构、认证重试语义或整条交易链。运行证据与主服务更新以[报告](windows-redis-idle-report.json)为准。
