# 路由权限注册与继承候选

日期：2026-10-27。T001/T002/T004/T007的静态证据，不是实际HTTP权限验收。

## 可重复扫描

```powershell
node retail/scripts/inventory-route-policies.cjs --check
```

`route-policy-registrations.json`登记根middleware导出实际展开的数组：原生525条、项目66条，共591条注册，其中356条有policies。扫描沿require别名、导出数组及嵌套展开追踪，不以磁盘中存在任意middlewares文件代替根配置注册；不加载业务模块、不执行验证器、不访问数据库。PolicyOperation默认值来自实际安装源码，Entities字符串来自编译枚举，未知表达式失败。

原始method/methods声明另交给实际纯defineMiddlewares辅助函数规范化，并逐项比对。处理方法按实际导出函数收集；安装的Express 4依赖path-to-regexp 0.1.13匹配代表路径：无methods的app.use按前缀、指定方法按完整路径。没有调用HTTP或Express应用，且没有完成所有参数值、排序/终止next、动态注册与源覆盖验证。

542个文件哈希包括注册源、处理器、查询枚举及框架loader/router/sorter/helper/匹配器。8项扫描器人工检查覆盖未识别符号/动态调用/正则须失败，以及全局前缀、相似名称、指定方法完整匹配和星号边界。文件数量不代表新增业务数。

## 覆盖结果

| 来源 | 处理方法候选 | 其中Admin | 匹配资源policy的Admin |
| --- | ---: | ---: | ---: |
| 安装版Medusa原生 | 487 | 410 | 379 |
| 项目自定义 | 86 | 52 | 11 |
| 已配置Analytics插件 | 3 | 3 | 0 |
| 合计（含重复候选） | 576 | 465 | 390 |

包括9组原生/项目同方法同路径候选，清单逐项列出。源routes-loader的registerRoute按matcher/method覆盖前项，但当前扫描没有运行整套sourceDirs加载顺序，不能据此宣称其中哪一个是已验证生效处理器。购物车付款session的项目包装也在重复清单内，后续实现不能漏掉包装语义。

本次插件范围明确为已配置的Analytics三个查询端点；支付provider的运行时入口、框架动态路由和完整插件发现顺序仍需单独核对。没有把磁盘中全部未配置插件算成实际业务。

## 自定义方法的继承候选

| 方法/路径 | 代表路径匹配的资源权限 |
| --- | --- |
| GET /admin/inventory-items | inventory_item:read |
| GET /admin/orders、GET /admin/orders/:id | order:read |
| GET及POST /admin/orders/:id/after-sales | order:read |
| POST /admin/orders/:id/cancel | order:read + order:update |
| POST /admin/orders/:id/fulfillments/:fulfillment_id/cancel | order:read + fulfillment:update |
| POST /admin/orders/:id/fulfillments/:fulfillment_id/shipments | order:read + fulfillment:update |
| POST /admin/orders/:id/fulfillments/:fulfillment_id/tracking | order:read + fulfillment:update |
| GET /admin/stock-locations、GET /admin/stock-locations/:id | stock_location:read |

项目根middleware明确写policies的仅跟踪号修改一项；其他匹配来自原生注册。POST售后入口匹配的是order:read，不能推导已经有新的售后写权限；Gin需明确区分查询和命令，仍按对象/状态规则授权。

其余41个自定义Admin方法没有匹配到这份根注册清单中的资源policy。它们仍可能有默认身份、供应商/仓库guard、领域工作流和其他检查；“未匹配policy”不等于公开或可越权。新Gin对所有员工入口声明明确动作权限，不能沿用“没有policies也有登录所以够了”的隐含边界。

## 默认鉴权与例外

实际framework router对/admin默认使用user身份、bearer/session/api-key；它根据routesFinder查询当前方法路径，再尊重路由AUTHENTICATE=false。安装原生仅GET /admin/feature-flags及POST /admin/invites/accept导出该false。新系统邀请仍需有效邀请能力和确认流程，不能把原生opt-out当成无凭证注册员工。

项目86个自定义方法未导出AUTHENTICATE=false，但位于/mobile-intake、/mail等非admin/store前缀的入口并不会仅凭该默认值自动得到Admin鉴权。应使用已登记的签名、能力、归属和回调验证。store前缀默认publishable key和可选customer，另叠加各领域明确身份；可选身份不表示私有购物车/订单可按ID读取。

启用RBAC时policy middleware由framework wrapper执行；policy-only配置会由middleware loader添加next的占位函数，仍包裹权限检查。不能因middlewares数组为空而漏算policies。尚未验证真实RBAC grant、动态字段filter、缓存撤权及完整HTTP顺序；[匹配函数32项证据](native-permission-contract.md)也不能代替本层运行验证。

新后台Ant Design菜单/按钮使用Gin明确动作集合，查询DTO按角色裁剪。迁移目标仍要求默认拒绝、当前授权、逐对象范围、准确金额及完整命令状态；这些规则不依赖Medusa运行时。
