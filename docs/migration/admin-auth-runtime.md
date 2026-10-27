# 新后台账号登录：Gin、MySQL 与 Redis 联合路径

2026-10-27，T014/T019的后台路径实现；客户认证、完整权限体系及React页面尚未验收。不迁移Medusa员工/客户账号或个人授权。

`cmd/api`要求显式`SESSION_REDIS_URL`、`SESSION_REDIS_PREFIX`、`ADMIN_ORIGIN`与cookie配置。新Redis连接成功并验证配置后才创建路由；生产必须开启Secure，HTTPS Origin也必须配Secure。开发HTTP只允许localhost/回环地址。Origin必须是没有路径、查询、身份或fragment的精确来源；不由请求Host/Forwarded头生成可信来源。Redis关闭在HTTP优雅关闭后进行，健康检查同时核对MySQL与会话Redis；进程真实启动/信号关闭仍需独立验证。

旧`identity.New`工厂保留为无配置拒绝入口，不能签发会话；正式进程使用`RouterWithAuth`。已删除MySQL会话读取/写入及单进程内存登录计数。旧`admin_sessions`表暂留历史schema兼容，不承担新会话，不读取其中旧cookie。

## HTTP流程

| 方法与路径 | 行为 |
| --- | --- |
| POST `/api/admin/auth/challenge` | 精确Origin校验、按实际连接IP限制挑战分配；返回10分钟CSRF挑战及HttpOnly预登录cookie |
| POST `/api/admin/auth/login` | application/json；仅email/password，拒绝重复/未知字段、多文档、非字符串及超72字节密码；消费预登录cookie+`X-CSRF-Token`，再做IP及规范化邮箱两项限流、MySQL账号/KDF验证，签发新Redis会话 |
| GET `/api/admin/auth/me` | Redis解析后台会话后，每次读取MySQL新账号及撤销版本；返回私有账号DTO |
| POST `/api/admin/auth/logout` | 同样鉴权，另要求精确Origin及会话CSRF；Redis删除成功后清cookie；依赖失败不伪报成功 |

登录失败需要重新获取一次性挑战；错CSRF不消费挑战，已消费挑战不能重用。限流为各IP/各规范化邮箱15分钟最多8次，挑战分配另按IP8次；没有成功后清零绕过。当前不信代理，因此IP来自直接连接，未来代理环境必须显式配置可信链及评估共享出口策略，不能盲信X-Forwarded-For。分布式滥用/全局预算策略仍待补。

所有后台匹配路由设`private, no-store`。Secure环境cookie使用`__Host-retail_admin_session`、`__Host-retail_admin_prelogin`及`__Host-retail_admin_csrf`，Path=/、不设Domain、SameSite=Strict；前两者HttpOnly。开发非Secure去掉`__Host-`前缀，仍与客户会话分开。CSRF cookie可读，用于React刷新页面后读取header值；服务端与Redis中的CSRF摘要比较，不信任cookie本身作为认证证据。登录响应也返回CSRF，日志不记录响应体。浏览器写接口必须传`X-CSRF-Token`，只带CSRF cookie无效；重复同名认证cookie拒绝。

成功登录每次生成新session/CSRF；如果请求带旧后台session，其撤销成功后才交付新cookie。旧session撤销失败时不交付新能力，并尝试撤销刚生成的临时session。Redis丢失会话不从MySQL恢复，要求重新登录。

## MySQL权威与撤销

新增显式`0009_admin_auth_version.sql`为新本机账号增加非零`auth_version`（BIGINT UNSIGNED，默认1）；账号初始化命令显式赋1。旧迁移文件不修改，实际显式runner空库应用及重复执行9文件通过。

会话版本由账号ID、密码hash、角色、enabled和持久撤销代数编码后SHA256生成，不返回前端。每次鉴权从MySQL读取当前行并核对，角色暂只允许admin；仓库/供应商仍拒绝，不能因此称完成动作/对象范围权限。KDF完成后再读账号，拒绝计算期间的账号变化；之后发生的撤销由下一次鉴权发现。

未来所有密码、角色、权限、enabled变更及“撤销全部会话”必须在同一MySQL事务增加auth_version。仅布尔状态反复切换不足以永远撤销旧会话，因此临时测试明确执行递增，再验证停用→启用、角色改回及密码改回后旧会话仍拒绝。生产账号修改入口和完整权限撤销事务尚未实现，不能把测试中的SQL更新当作已经提供的管理功能。已开始执行的业务事务如何与撤权排序仍归T015领域实现。

无效账号/密码统一401；无会话401；CSRF/Origin拒绝403；限流429；MySQL或Redis故障503。数据库查无账号与数据库故障区分，后者不能返回“密码错误”或放行。响应不含password_hash、auth_version或驱动错误；既有请求边界负责request_id及日志隐私。

## 验证及限制

执行：

```powershell
python retail/scripts/test-admin-auth.py
```

只用人工新账号，真实Gin路由分发（httptest，无监听端口）、MySQL8.4及Redis7。MySQL独占禁网namespace，Redis只共享该独占namespace的loopback；无外部网络和端口映射。MySQL单核768MiB，Redis单核128MiB，临时数据目录tmpfs；按独占标签反向删除两容器及临时编译物，不连接本项目旧服务/业务库或任何部署服务器。

[联合实证](admin-auth-report.json)覆盖9文件schema执行/重复及CHECK，真实TCP `sessionstore.Open`、登录/当前用户、重复cookie/CSRF/Origin/错误密码/未知账号拒绝、一次性挑战不可重放、旧session轮换、退出、后台商品写入前CSRF、8次邮箱限额、停用/角色/密码/代数撤销、旧MySQL cookie及客户realm拒绝、依赖健康检查及关闭Redis/MySQL客户端后503。真实Redis组件的16/24路并发另见[组件证据](redis-session-report.json)。普通Go测试补充重复JSON字段、Origin/cookie配置及会话版本；`go test ./...`、`go vet ./...`通过。

尚缺浏览器真实cookie/代理/HTTPS、Ant登录页面、客户注册/验证/会话、完整权限/数据范围、账号编辑与撤销事务、真实服务宕机/网络分区/重启、进程关闭验证。故障测试关闭客户端/连接池，不能冒充服务端崩溃；schema0001及目录payload依然未验收，不能用于生产切换。T014/T019保持未验收。
