# 购物车换区、语言与地址准备规则

2026-10-28，T025/T032 的中间实现，完整换区事务尚未验收。

`retail/backend/internal/cart/context_plan.go` 将原 `update-cart` workflow 的地区、币种、语言、配送地址协调和重算判定移植为独立 Go 准备步骤。没有 Medusa 运行时依赖，不读写数据库，不处理账户、客户邮箱或销售渠道变更，也不授予购物车归属。当前尚未接入写命令或生产页头。

`ContextField` 区分省略与显式清空，结果深拷贝语言及地址，避免修改调用者输入。目标地区必须由后续事务从可信目录解析；传入目标与请求不一致会拒绝。地区币种来自解析后的地区，不能使用浏览器报送的币种。

保留原实现的细节：

- 先验证显式地址国家属于目标地区，国家代码精确比较。非法国家不能被单国家地区默认值覆盖而绕过校验。
- 换到单国家地区时，将配送地址替换为只有国家的对象，即使请求同时提供合法城市和详细地址，也会丢弃这些字段。这是原代码行为。
- 换到多国家地区时，没有明确提供合法国家就清空旧地址；提供合法新地址则保留。没有换地区时保留部分地址更新的语义。
- 显式请求当前地区也要求刷新，但只在实际换地区时删除自定义价格行。
- 语言变成不同非空值要求刷新；显式 null 或空字符串沿用原判定，不单独触发刷新。实际保存 null 和省略字段仍有区别。
- 国家、province、postal_code、city 或地址 ID 变化要求重算税费，普通姓名变化不单独触发。税区比较对国家忽略大小写，地址国家归属校验仍精确比较。显式 null 和省略按原 JS 语义分别处理。

`capture-cart-region.cjs` 从安装的原 workflow 提取实际纯准备回调，以及税区变化、地区变化、新语言和 refresh 输入的四个 transform，使用人工数据执行。固定完整来源 SHA；不启动旧应用，不调用 provider，不读取业务库。fixture 版本 2 包含 12 组准备样例和 16 组 workflow 判定；`context_plan_test.go` 读取该 fixture 比较 Go 输出，并验证输入不变、输出不共享指针及目标解析错误。准备样例只对比本步骤负责的地区/币种/语言/地址投影，未覆盖客户和销售渠道更新。

验证命令：

```text
node retail/scripts/capture-cart-region.cjs --check
cd retail/backend
go test ./internal/cart
go test ./...
go vet ./internal/cart
go build ./internal/cart
```

本批以上检查通过。未配置人工 SQL 环境的 `go test ./...` 不作为数据库集成验收；数据库现有购物车能力的隔离回归单独使用 `python retail/scripts/test-admin-auth.py --cart-runtime-only`。

后续必须在同一 MySQL 事务内完成持有者/代次/版本和能力校验、当前目录解析、独立持久幂等回执、地址变更、自定义价格行移除、所有有效行重新选价及 tax/promo/shipping 协调，最后同时提交版本和回执。任何失败整体回滚，同键重试不重复修改；不同请求复用键拒绝。Redis 丢失不得推断历史客户归属。重算完成前不能返回换区成功或导航；生产 locale cookie 与真实购物车必须协调恢复。当前没有新增 SQL migration、HTTP 路由或 enabled capability，不能据本准备步骤宣称换区、税费、促销、配送或结账已经完成。
