# 后台真实浏览器登录链路

2026-10-28，使用本机 Edge 显式覆盖路径，运行 `retail/scripts/test-admin-live.py`。
浏览器访问独占 Vite 19200，同源代理到实际编译的 `cmd/api`，再连接真实 MySQL 8.4
和 Redis 7。测试没有拦截或伪造 API 响应，没有新增测试认证路由或运行时绕过。

数据库仅包含九文件迁移和一个人工新账号。Go 测试准备器只创建临时数据，随后由
正式 Gin 二进制接管请求。MySQL 单核/768MiB、Redis 单核/128MiB；两容器使用
自有网络命名空间，专用 Docker bridge 只发布随机主机回环 API 端口，数据库和
Redis 无映射端口。Bridge 未限制出站流量，不声称禁网。先前 internal 网络试验
未提供端口映射，已经按所有权清理；没有改动旧开发服务或真实业务数据。

[报告](admin-live-report.json)记录一项端到端浏览器用例及实现摘要，验证：

- 错误密码后清空输入，并通过新挑战再次登录；真实账号资料从 Gin 返回。
- 会话 Cookie 为 HttpOnly、SameSite Strict；CSRF 可读；一次性挑战 Cookie 清除。
- 页面刷新保留登录；账号资料和凭据不进入 local/session storage。
- me 响应为 private/no-store，包含服务端请求编号。
- 缺失 CSRF 和错误 Origin 的退出请求被拒绝，原会话继续有效。
- 正常退出清除会话 Cookie，重新提交已捕获旧 Cookie 仍返回 401。
- 浏览器无未捕获运行时异常；所有自有容器、网络和临时二进制清理完成。

本次为显式回环 HTTP，因此只验证开发模式 Cookie；生产 HTTPS/Secure、跨浏览器、
代理链信任、客户认证、权限对象范围和全部后台业务仍未验收。先前
[七项人工 HTTP UI 测试](admin-login-ui.md)继续保留，用于失败状态、语言与响应式。
两种证据范围不同，均不能代替全量迁移验收。后台组件优先 Ant Design，图表统一
ECharts；当前没有统计业务，不制作虚构图表。

2026-10-28补充：后续[商品详情](admin-product-detail.md)使此浏览器演练改用完整
159表审查结构；九项基础migration之后，七张草稿领域表在临时库中改名保留，
再建审查表并插入人工商品/选项/变体/图片。正式cmd/api与新账号仍在真实浏览器
链路中；这一显式临时准备步骤不是正式数据库升级或全域schema验收。

后续[0010迁移](catalog-schema-migration.md)已替代上述临时准备：当前脚本仅调用
完整10文件迁移并插入人工行，七旧表改名由有前置条件的migration执行，不再由
浏览器脚本直接执行RENAME或审查SQL。全域schema及写入仍未验收。
