# Gin API 通用约定草案

日期：2026-10-27。T007 进行中。本文件固定跨领域规则供后续 schemas/实现使用；不能据此认定全部 API 契约已完成。

## 路径与边界

- 新 JSON API 分 `/api/store` 与 `/api/admin`，同源由前台/后台代理到 Gin；最终不调用 Medusa SDK 或服务。领域路由单独注册，不能用任意 resource/action 反射映射数据库 CRUD。
- 后台身份、客户身份、匿名购物车凭证、短期移动凭证、支付/邮件商户回调分别验证，不能接受一种 token 访问所有命名空间。
- 浏览器外部已保存链接按兼容清单继续处理：国家语言页面、variant slug/v_id、订单转移、验证邮件、`/api/mollie-return`、`/api/payment-return`、PayPal/Mollie webhook、`/mail/webhooks/:account_id`、移动入库及 SEO 文件。保留路径不等于信任旧登录 cookie/token；在途交易恢复另行对账。
- GET/HEAD 不发起付款/退款、发信、变更状态或消费一次性凭证。旧有 GET 内执行业务的兼容入口必须转受保护确认流程，不能让页面预取或截图触发。只读搜索、报价计算可使用显式 POST，但不与业务写接口共用宽泛例外。

## 数据类型与响应

所有 ID、展示号、SN/IMEI/EID 为字符串；金额/数量中允许精确小数的字段是十进制字符串，字段 schema 单独规定正负/范围/精度。禁止 NaN、Infinity、科学计数法和前后空白进入业务 API。设备数量/星级等明确整数仍 JSON integer。没有价格用 null，零价为合法的 `"0"`，不能靠 truthy 判断。

时间使用 UTC RFC3339（明确至微秒的持久化精度）；日期区间明确时区、包含起点/不含终点。编号生成用 Asia/Shanghai 与数据库持久计数；客户端不生成业务展示号。

列表统一 `{items,count,limit,offset}` 作为新跨领域基础。为减少前端迁移风险，各领域可使用已约定的具名数组（products、orders、requests 等），但必须在 schema 明确，不让组件猜键；搜索批量结果另定义。limit 默认20，上限100；某些旧运营列表上限200需显式契约，禁止通用客户端静默截断。无结果返回 `[]`；count 是全部过滤及数据授权后总数。排序白名单并附稳定 ID 次序，无法识别过滤条件返回400。

不支持任意 Medusa fields 深层关系语法。只提供界面需要的 DTO 与有界 include 白名单，公开、客户、仓库、供应商、员工 DTO 分开。源 payload/metadata 保留在内部不自动公开。private 响应 Cache-Control: private,no-store；公共缓存键包含店铺/渠道/地区/币种/语言及相关版本，禁止以用户 cookie 填充公共缓存。

## 错误与请求追踪

```json
{
  "error": {
    "code": "inventory.insufficient",
    "message": "Not enough stock for the requested quantity.",
    "fields": [{"path": "items[0].quantity", "code": "exceeds_available"}],
    "request_id": "req_example"
  }
}
```

code 是稳定领域码，前端中英翻译依 code/field 映射；message 提供可读后备，不把数据库/SDK 原始异常返回。400 语法/字段无效，401 未登录/已过期，403 无动作权限，404 不存在或归属不可见，409 状态/版本/幂等冲突，413 文件/请求超限，415 类型不支持，428 缺少必需版本前置条件，429 限流，502 外部服务明确失败，503 依赖暂不可用，500 未分类内部错误。库存不足统一409及领域码，避免前端继续匹配英文错误字符串。

X-Request-ID 由服务端生成或严格验证可信上游值；响应与日志一致。日志只记录规范化路由、状态、耗时、请求 ID 和必要的非秘密主体引用；禁止记录原始 query（含验证/支付/转移凭证）、Authorization、cookie、请求体、数据库绑定参数、签名下载 URL 或 provider 原始错误体。

## 登录、权限与 CSRF

后台和客户使用不同 cookie 名及会话命名空间；HttpOnly、SameSite 与生产 Secure 明确配置。Redis 为活跃 session/限流，丢失需重新登录；MySQL 账号停用/权限版本为权威，权限查询故障默认拒绝。密码/会话/旧个人授权不导入。新账号同邮箱不能认领旧订单、回收、评价已购资格或邮件身份。

浏览器写操作使用可信站点 Origin 与会话 CSRF token，登录同样防跨站；反向代理只信明确配置代理，不能任意信 X-Forwarded-Host 构造返回地址。支付和邮件 webhook 不使用浏览器 CSRF，而是对应验签/商户读取与事件去重。身份校验、动作权限、对象范围、字段裁剪均须服务端覆盖，详见 [授权矩阵](authorization-matrix.md)。

## 幂等、版本与外部副作用

涉及创建交易、支付/退款、履约、设备批量命令、入库、回收、发信和任务触发的写请求要求 `Idempotency-Key`；简单设置保存可依资源版本保持幂等，但不能将所有重复 POST 当不同操作。

操作账本存 MySQL：scope（店铺/身份类型/主体/命令）+ key 唯一，记录规范化请求 hash、目标资源、状态、可重放响应、attempt/租约和外部引用。JSON key 顺序不影响 hash，金额先合法化，上传 hash 替代原文件内容。相同 key/相同参数返回既有结果；同 key/不同参数409；仍处理中的操作返回202与受相同授权保护的查询位置/Retry-After。不能仅 Redis SET NX 后直接调用 provider。

请求重放仍验证当前身份/对象权限，避免撤权后取得私有旧响应。支付/退款等持久业务去重键不随缓存 TTL 消失；不同 key 也受 cart 唯一成单、有效分配唯一、退款余额等数据库约束保护。跨 HTTP、worker、回调共用同一领域命令入口，不能各自幂等后重复产生业务事实。

后台可修改聚合携带 version/If-Match，读时返回版本；更新事务内检查版本、业务状态和数据范围，成功加版本并记录准确 before/after 审计。客户端不能通过并发打开两个抽屉无提示覆盖另一操作。交易全程用稳定锁顺序，死锁有限重试同操作键，不重新创建外部支付。

外部 API 不持有长 MySQL 事务。先持久化意图/outbox → worker 认领租约 → 调 provider 稳定幂等键 → 记录结果/事实/后继事件；超时保留 unknown 并查询对账。原生流程的补偿必须有持久状态，不能在 catch 中无条件退款/释放已经被其他成功订单使用的预留。

## 文件与任务

上传按领域限制真实文件头、大小、数量、对象归属与类别；存储 key 由服务器生成。公开产品图和私有回收/邮件附件分别处理；私有对象不挂 static，下载或短时 URL 每次授权。移动入库短凭证不能变成永久公开上传能力。

长任务返回 operation/job ID 与当前状态，后台轮询/订阅受动作与对象权限保护；任务状态、进度、失败和重试保留 MySQL。worker 关闭停止认领并在宽限期结束后释放/等待租约恢复；失败不能只写日志。ECharts 展示 loading/empty/error 要根据这些真实状态，不能把查询失败画成零值。

## 仍需交付

本约定是新实现设计；现有 `retail/backend` 草稿未满足它。[请求基线与SDK承接](request-validation-and-sdk-contract.md)已提取35份旧输入结构、72处额外规则、35个人工用例，并为36个SDK候选设计27个新操作；[直接HTTP](http-caller-contract.md)和[全部86个自定义方法](custom-route-contract.md)已有设计引用。这些不是完整的新API schemas或全部原生入口。[原生保护](native-guard-contract.md)另登记25条挂载及46个隔离样例，不能当作处理器或授权验收。待补原生后台/插件、多动作/资源分支的完整映射、请求/响应字段、领域错误码表、CSRF/会话生命周期及契约测试。相应更改进入T010—T019时逐项实现，不修改旧Medusa运行约定。
