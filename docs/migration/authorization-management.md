# 角色及成员授权管理

2026-10-28。0024 在持久动作模型中增加 `authorization.read/write`，当前字典为 11 项。新建账号显式初始化时取得当前字典；已存在角色不因名称或升级自动获得新动作。旧账号、旧密码和旧 grant 不导入。当前仅员工全店动作管理，完整授权领域尚未验收。

## 接口及事务

GET `/api/admin/authorization` 要求 read，使用只读 Repeatable Read 返回字符串修订、完整动作字典、角色及成员明确投影，不返回邮件、密码、认证代数或私有配置。最多 100 角色/1000 主体；超限整体 503，未知查询参数 400。

POST `/api/admin/authorization/commands` 要求 write、有效登录、Origin 与 CSRF。六项必填外层字段是 operation_id、expected_revision、kind、role、user_id、roles。role.save 使用四字段 role 对象及两个 null；member.roles 使用 null role、既有员工 ID 与明确角色数组。未知/重复字段（包括嵌套）、错误 null/类型、非规范修订/UUID、重复动作和未知动作都拒绝。单个请求最大 32 KiB。

保存先锁权限运行行 FOR UPDATE，再锁读当前账号、主体和动作。当前 write 验证先于已有回执读取；撤权后不可借旧 operation_id 重放。新请求要求预期修订一致，所有授权、before/after JSON、原规范请求、操作者及结果修订在同一事务提交。八路相同请求只提交一次，其余返回同一回执；不同内容复用 ID 返回 409。任一步失败回滚全部更改，不返回成功修订。

操作者不能授予自己没有的动作；成员只能分配存在、启用且全部动作可授予的角色。角色可新增、修改及停用；成员仅管理既有 employee 主体的角色关系，不建立账号、不改变主体类别及仓库/供应商归属。变更后必须保留至少一名启用 admin 员工、有效认证代数、启用角色中的 permission write 持有者，避免最后一名权限管理员被删除。该保障覆盖本命令；后续账号停用/删除命令必须纳入相同约束。

遵守运行行锁协议的业务写入与撤权串行化；正在执行的商品/库存事务持 SHARE 锁时，权限更新等待至业务提交/回滚。一般已开始的读响应不保证即时撤回。直接 SQL 管理不属于受支持的授权写入口。

## Ant 页面及验证

React `/authorization` 使用 Ant Table、Tabs、Drawer、Form、Switch、Checkbox、Select、Alert 和反馈消息。read 控制菜单和路由，write 控制角色编辑与员工角色分配；当前动作之外的授权选项禁用。字典/数据校验失败整体关闭页面，不保留上次目录。权限不保存至浏览器存储。

每次保存固化 operation_id、修订及内容；超时或 5xx 后锁住编辑，只允许重试同一请求或关闭重新加载。409、403、404/400 分别提示后要求重新读取；成功刷新目录及当前权限。中英页面沿用项目主题，手机表格内部横向滚动。

[MySQL/Redis 测试报告](admin-auth-report.json) 的 authorization_management 覆盖角色/成员保存、8 路幂等、修订冲突、审计失败回滚、最后管理员保护、撤权重放拒绝、越权授权拒绝、作用域拒绝、业务锁串行化和严格 wire 校验。[页面报告](admin-ui-report.json) 覆盖原请求重试、冲突、只读及非法字典；[真实浏览器报告](admin-live-report.json) 覆盖 Vite → Gin → MySQL/Redis 的创建角色、分配成员、刷新持久化及中英三宽抽屉。全部使用独占可销毁数据库与人工新账号。

正式计划现有 24 个迁移；[固定商品整合](catalog-mysql-rehearsal-report.json)和[支付隔离演练](payment-mysql-rehearsal-report.json)验证迁移扩展后原来源事实、账本和缺失引用保留。未操作部署服务器或原应用数据。

尚缺账号生命周期命令、审计检索界面、仓库/供应商对象及字段范围、附件/成本权限、移动受限凭证、客户授权、集成及 worker 权限，以及其余业务动作。所有新后台组件优先使用 Ant Design；图表统一 ECharts，目前本页面没有图表需求。
