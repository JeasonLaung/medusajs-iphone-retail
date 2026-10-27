# 实际加载器与隔离HTTP验证

日期：2026-10-27。承接[路由注册清单](route-policy-contract.md)的9组覆盖候选，推进T004/T007；不代表新Gin实现或真实登录/业务工作流验收。

## 验证方式

```powershell
node retail/scripts/inventory-route-policies.cjs --check
node retail/scripts/test-route-loading.cjs --check
```

34项结果见 `fixtures/legacy-route-loading.json`。测试先核对注册清单中542个来源哈希，然后直接执行安装版以下代码：插件解析getResolvedPlugins、Medusa API入口loader、ApiLoader、RoutesLoader、MiddlewareFileLoader、RoutesSorter、RoutesFinder、defineMiddlewares、hasPermission及policy wrapper、wrapHandler。使用实际安装Express及其路径匹配，绑定127.0.0.1随机端口，结束在finally关闭监听及连接。没有连接业务数据库/Redis，也没有创建第二个Medusa运行实例。

人工替身边界：路由模块返回人工处理器，中间件业务函数为next，认证读取仅测试专用请求头，角色/授权查询及缓存为人工端口，CORS/bodyparser/publishable key/locale/restricted-fields为替身。目录枚举按已校验manifest返回候选，未运行全部文件动态导入。没有验证真实密码、cookie、CSRF、邀请内容、库存、设备、订单、付款或provider。

插件配置从项目medusa-config.ts的AST读取resolve字面量，不执行config或加载.env。实际getResolvedPlugins只读取安装包package.json和插件静态options；模块枚举端口返回空，因为本轮只验证API来源顺序，不启动任何模块。输出只包含非秘密来源和测试结果。

## 已执行的加载顺序

实际Medusa API loader传给ApiLoader的顺序为：原生API → 配置中的Analytics插件 → PayPal插件 → project-plugin(src/api)。project-plugin由实际getResolvedPlugins在isMedusaProject=true时追加，项目入口因此最后覆盖同方法/路径原生处理器。实现代码优先于源码注释：原生loader注释提“plugin before core”，实际sourcePaths.push先放core；本结论来自运行结果。

真实RoutesLoader逐sourceDir扫描、按matcher/method登记并覆盖；真实MiddlewareFileLoader则累计各sourceDir中间件。真实sorter/finder及Express注册后，对9组候选发送人工请求，全部命中项目人工处理器：

- GET /admin/inventory-items
- POST /admin/orders/:id/cancel
- POST /admin/orders/:id/fulfillments/:fulfillment_id/cancel
- POST /admin/orders/:id/fulfillments/:fulfillment_id/shipments
- GET /admin/orders/:id
- GET /admin/orders
- GET /admin/stock-locations/:id
- GET /admin/stock-locations
- POST /store/payment-collections/:id/payment-sessions

不必删除原生文件才让自定义文件生效；新Gin仍须承接项目包装实际业务，不能只迁原生同名接口。当前在运行的旧进程可能有HMR后加载状态差异，本轮没有用业务写请求探测旧进程。

## 权限顺序的执行结果

对上述8个有资源policy的Admin覆盖路径，人工已登录但无grant返回403，未到达处理器；给齐代表路径清单中的grant后200且登记的policy集合相符。证明在这套隔离加载下，handler覆盖不会自动移除原生继承policy；不证明真实账号持有哪些grant。

匿名GET /admin/orders在人工身份端口被401阻止。原生feature-flags及invites/accept的AUTHENTICATE=false由实际finder识别，匿名人工请求命中handler；邀请处理器已经替换，不证明无有效邀请也能注册员工。真实邀请能力、验证、过期与防重放仍须另验。

POST售后在order:read单独授权时到达人工handler，说明当前继承注册层并不提供独立售后写授权；领域工作流和范围检查未运行。POST跟踪号仅order:read时403，仍要求fulfillment:update。新Gin必须显式区分查询/售后命令/跟踪修改，并叠加对象及状态校验。

Analytics三个实际配置入口均由插件人工handler命中且未登记resource policy。这说明本次已加载注册中无相应policy，并非插件统计对匿名公开：Admin身份端口仍存在，项目供应商/仓库保护和真实插件业务函数未运行。新后台ECharts统计需明确统计权限与范围，不能以“已登录员工”替代运营统计授权。

## 尚未验收

真实认证/会话刷新、CSRF、原生缓存撤权、项目guard、中间件短路、字段过滤、插件运行时注册、全部动态路径值、完整交易状态和故障恢复仍待验证。新Gin/SQL权限及React页面尚未实现。这34项只将覆盖候选和基础注册顺序从静态推断推进为隔离执行证据；T004/T007及整体迁移保持进行中。
