# 原生RBAC匹配与处理器保护

日期：2026-10-27。T004/T007补充。安装版本2.21.2，测试实际安装文件，不依赖网络文档推测行为。

## 执行证据与限制

`retail/scripts/capture-native-permissions.cjs` 执行framework安装包的 `dist/policies/has-permission.js` 和 `dist/http/middlewares/check-permissions.js`，固定32项人工用例及三个文件哈希于 `fixtures/legacy-native-permissions.json`。实际hasPermission、resolvePermissions及wrapWithPoliciesCheck逻辑参与运行；feature router、query.graph及useCache为人工端口。useCache替身每次直接运行查询，未验证缓存命中、撤权失效或跨进程行为。

```powershell
node retail/scripts/capture-native-permissions.cjs --check
```

此验证不验证真实JWT、会话、数据库、路由策略挂载或Gin。项目配置 `medusa-config.ts` 明确 featureFlags.rbac=true 并加载RBAC模块；不能把测试中的关闭开关分支理解为本项目实际未启用RBAC。

## 已确认的组合语义

- 资源、操作均精确且区分大小写；`*`是完整通配值，`ord*`不是前缀模式。
- 多角色授权取并集；同一请求的多个action、同一action的多个operation均须获准，不能以“任一命中”放行整个命令。
- resolvePermissions将通配授权展开到给定universe，仅输出其中的具体resource:operation；后台菜单使用的扁平权限不代表额外对象范围。
- 底层hasPermission在roles=[]、actions=[]或rbac开关关闭时返回true。resolvePermissions在RBAC开启且roles=[]时返回空集合，与hasPermission空角色分支不同。
- 处理器wrapper在非空policy列表下先要求auth_context.app_metadata.roles非空，再调用hasPermission；故无身份/无角色会被wrapper拒绝，并非底层空角色true就能越权。项目hasWarehousePermission也预先检查查询得到的角色非空。
- wrapper遇空policy列表直接进入handler，未验证身份；这不是该handler公开安全的证据，仍须结合真实authenticate及范围保护。wrapper会将policy登记到req.policies，供后续处理使用。
- 查询或缓存端口故障向上传递；wrapper将错误交next且未进入handler。不能把依赖故障当作空权限配置后允许。

## 缓存、撤权与角色来源

实际底层函数按每个role ID查 `rbac_role` 的policies，传给useCache的TTL为604800秒、provider=cache-memory，并设置role/policy相关tags。测试确认这些调用参数，但不证明真实变更是否正确失效，不能仅凭TTL宣称存在七天撤权漏洞。

原生wrapper从auth_context.app_metadata.roles取角色；仓库函数则通过当前user及rbac_roles关联查询角色。两者的角色刷新时机、认证元数据更新及真实cache invalidation仍待追踪。新Gin以当前账号enabled/权限版本及持久grant为权威，不能仅相信旧JWT中角色或长寿命进程缓存。停用、撤权以及对象来源/仓库变更要在新命令提交前重查，缓存/权限服务故障拒绝。

## 新系统承接

新登录账号与旧账号、旧secret key、旧个人授权分离。Gin路由声明明确非空的领域action；未知身份/角色/动作默认拒绝，不能复制底层空roles/actions即true的宽松分支。公开目录、回调、短期能力入口使用独立显式声明，不能靠“没有policy”等于公开。

角色授权之后继续校验仓库、供应商来源、订单/包裹/设备归属及字段DTO；具体交易还需状态、付款、库存和并发约束。Ant Design菜单、按钮、表格和抽屉使用同一权限词表供交互反馈，但每次API仍执行服务端检查。

后续[路由注册静态追踪](route-policy-contract.md)已展开591条middleware注册及576个处理方法候选，记录默认身份、policy继承候选及9组覆盖。它不执行HTTP排序或角色检查。

剩余：全部原生/自定义/插件路由的运行时policy及authentication组合、资源字典、角色种子、真实缓存失效与会话撤权，以及Gin持久RBAC/HTTP/事务。当前32项不是全站权限验收，T004/T007不勾选完成。
