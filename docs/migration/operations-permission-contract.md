# 邮件与回收动作权限设计

日期：2026-10-27。T004/T007的新系统设计，不是旧账号授权映射，不是Gin实现。机器清单 `schemas/operations-permissions.json` 含45项：32个命令分支、8类邮件列表、5个详情/下载入口。

## 设计与覆盖

`design-operations-permissions.cjs` 使用显式人工权限映射，对照已提取的真实MailCommand、MailListQuery及BuybackAdminCommand输入枚举逐项检查覆盖，无“未知action用通用write兜底”。旧输入schema和来源哈希同时核对；工作流、入口源码另固定哈希。权限名是目标设计，不声称旧系统已有相同授权。

```powershell
node retail/scripts/design-operations-permissions.cjs --check
npm test --prefix retail/contracts
```

后者保留供应商wire schema检查，并新增64项权限契约模型检查，包括45个唯一选择器及19项越权/冲突拒绝。契约模型读取JSON，未启动Gin、SQL或worker；不存在运行时拦截证明。来源枚举全覆盖不等于完整字段schema、工作流或全站权限完成。

## 邮件边界

| 能力 | 目标权限 |
| --- | --- |
| 账户安全投影、商户凭据修改、删除 | mail.account.read / configure / delete 分开 |
| 邮件正文读取、附件下载 | mail.message.read / mail.attachment.download 分开 |
| 草稿保存、发送及失败重试 | mail.draft.write / mail.message.send 分开；retry同样需要send |
| 已读/星标/归档/移垃圾箱/恢复 | mail.message.organize；永久删除另需mail.message.delete |
| 联系人读取/批量写入/退订 | mail.contact.read / write / unsubscribe 分开 |
| 活动读取/编辑/暂停 | mail.campaign.read / write / pause；暂停权限不能恢复发送 |
| 活动queue/resume/retry | mail.campaign.send；均会继续产生外部发送，需要持久操作及当前授权 |
| 模板、投递记录、事件 | template.read/write/delete、delivery.read、event.read按能力分开 |

账户或message ID来自浏览器不代表访问权；逐对象核对当前账户范围，消息/附件/活动/联系人父子关联及批次数组每一项。当前设计不为每个旧邮箱自动创建个人owner；对象授权持久结构与角色种子仍待定义。列表范围先于count和分页；私有响应no-store且专用DTO，API key/webhook secret不回显。

发信是用户此前单独控制的外部动作；设计权限不会替代操作者明确的发信意图、收件人确认、活动同意记录、频率/退订和状态检查。迁移验证不发送任何邮件。worker在认领时重查当前授权/业务状态，撤权后旧幂等响应也不能直接交给无权请求者。

## 回收边界

读请求、读照片、照片审核、批准寄送、确认收货、关联设备、报价、登记付款、请求退回、寄回、完成退回、取消分别有独立命名权限。mark_paid使用buyback.payment.record，不能因为能读单、审核照片或编辑报价就能登记实付；仍需合法状态和付款凭证，不能用它伪造商城provider支付事实。

link_device同时验证request/item/device及当前类别、位置和其他有效关联；final_offer逐item验证，不能混入其他回收单项目。照片先验证请求可见，再验证photo属于请求。旧客户联系快照保留不等于新员工/客户因同邮箱拥有该历史对象。

## 选择与并发规则

新API收到合法化请求后，按方法/路由模板及action、operation或resource精确选择唯一条目。拒绝未知、重复、冲突选择器及没有子操作的messages/campaign；读权限、角色名admin和mail.*字符串不隐含展开为全部权限。普通员工之外的supplier/warehouse/customer/integration不能进入这组员工运营入口；集成能力将来需独立契约。

既有聚合写入携带强If-Match，批量命令需授权集合版本覆盖每个成员。四个可新建的mail save命令按有无id区分：已有id需If-Match；无id需If-None-Match:*且拒绝If-Match，结合规范唯一键/幂等处理，而不是伪造已有版本。缺前置428、版本冲突409，细化header/schema与版本查询响应仍待完成。所有命令有Origin/CSRF/Idempotency-Key，敏感配置的请求去重使用受保护版本化HMAC，不能把明文秘密或无密钥秘密哈希持久化到日志/审计/幂等账本。

鉴权顺序是当前启用的新员工 → 当前权限版本/动作 → 每个对象及父关系 → 事务内版本与幂等 → 领域状态 → 响应字段。全局permission grant不能替代对象范围，菜单隐藏不能替代API检查；Ant Design按钮、表格和抽屉使用这些具体权限反馈可操作性。

## 剩余

字段/响应schemas、集合版本语义与absence header schema、实际权限表/作用域表、原生和其他自定义领域、Gin/worker统一入口与SQL锁、撤权/崩溃/外部失败恢复仍未实现。64项模型测试只验证设计决策，T004/T007及整体迁移不验收完成。
