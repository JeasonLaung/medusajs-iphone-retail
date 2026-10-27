# Gin 请求追踪、错误与私有诊断边界

2026-10-27，T010的局部实现。原Gin草稿默认Logger会记录原始URL/query，默认Recovery会输出panic及请求上下文，GORM默认错误日志可含SQL绑定值。支付回跳、移动凭证、会话与配置接口不能沿用这些输出。

`platform.RequestBoundary`替代Gin默认访问/异常日志：每次请求生成新随机UUID请求ID，写入`X-Request-ID`与错误体；当前没有信任上游代理，不沿用来路请求ID。访问记录只有request_id、已匹配路由模板、白名单方法、状态、耗时与panic布尔标记；未匹配路由使用固定`unmatched`，未知方法使用固定`OTHER`，不记录原始路径或参数值。丢弃panic值、错误列表、请求头/体、cookie、query与SQL原始错误；已经提交响应后的异常不追加第二份JSON，日志仍标记panic，已发出的状态不能伪改为500。

错误结构为`{error:{code,message,request_id}}`；404/405、未登录、Origin拒绝及数据库故障使用同一结构。数据库无记录为404，其他查询异常对外为固定500，不把原始错误附加到Gin上下文。现有草稿handler的错误code尚未全部迁移成领域命名，字段错误清单和领域错误表仍缺。

当前JSON API请求体上限1MiB，已知Content-Length过大在handler前返回413，未知长度由MaxBytesReader限制；登录和商品修改解析错误识别MaxBytesError，也返回413。该限制不是未来图片上传实现，T016须单独注册领域大小/文件头校验及上传路径策略。后台已匹配路由的登录、鉴权及业务响应设置`private, no-store`；全局提前拒绝的413不包含私有业务响应。安全头保留nosniff及Referrer-Policy。

健康检查数据库ping增加1秒context上限，不改schema，不启动新数据库。MySQL连接使用静默GORM日志，避免默认插值SQL错误泄漏；API及账号初始化的连接失败日志使用固定诊断语句，不能包含DSN/驱动原始错误。后续连接池/Redis、关闭过程、指标和受控错误分类仍属T010/T019待办。

验证运行`go test ./...`及`go vet ./...`。`internal/server/router_test.go`在真实Gin路由分发下，以无数据库连接的人工请求验证：成功、404/405、普通panic、提交后panic、数据库错误/无记录、缺少依赖、未登录、非法JSON、已知长度与分块超大请求；伪造请求ID每次被替换，错误体与日志相同ID，各次ID不同；有秘密标记的路径、query、header、cookie、body、数据库错误、panic和未知方法均不出现在日志/错误体。日志逐字段白名单且每次只有一条JSON。未运行生产登录、真实数据库HTTP请求或真实付款。

本次不验收T010/T014/T015：后台登录随后已接入[Redis及MySQL联合路径](admin-auth-runtime.md)，旧MySQL会话运行时已移除；客户认证、动作/对象范围、账号修改/权限撤回事务及浏览器仍缺；目录handler仍读旧payload草稿，与审查版159表不兼容。上述接口不能用于生产或宣称已经完成Gin业务迁移。
