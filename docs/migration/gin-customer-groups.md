# 新账户客户组与已绑定购物车取价

新增 Ant 客户组页面，支持创建、改名、软删除、成员搜索及批量加入/移除。成员仅来自本系统已验证、启用的新账户；不按历史邮箱、旧 customer_id 或浏览器传入组 ID 继承资格。价目表创建沿用原三步流程，以组名选择适用客户组，服务端重新校验当前有效组。

`0107_customer_groups.sql` 显式建立组、成员、不可变命令收据及默认关闭的管理门禁。读取需要 `authorization.read` 和 `price.read`，写入另需两者对应 write 权限。UUID、当前全局版本、当前权限及严格 JSON 对象共同控制写入；并发相同命令只有一次生效，丢响应沿原命令恢复。成员关系和最终收据同事务提交。

购物车通过既有 `customer_cart_ownership` 的明确新账户归属读取当前组，接入商品增改、商品价格刷新、手动固定运费及优惠计算上下文。账户锁先于组门禁，和登录刷新保持一致。关闭新增管理操作不会让已存在成员关系失效。删除组或移除成员后，后续正式购物车命令重新计算资格。金额仍采用原精确字符串计算。

接口：

```text
GET/POST /api/admin/customer-groups
GET      /api/admin/customer-group-accounts?q=...&group_id=...&offset=...
```

本机先备份专用库，执行 `go run ./cmd/migrate`，再执行 `go run ./cmd/configure-local-preview --customer-groups`；通过 `start.bat` 更新预览。应用启动不执行 DDL。

固定检查点：

```powershell
python retail/scripts/test-local-customer-account.py --customer-groups-from-checkpoint=790ce0fc9e71
```

[实际验收报告](customer-groups-native-report.json) owner `fd5a595f08db`：真实 Gin/MySQL/Redis/Ant/Astro，创建组、加入、移除各六并发且注入首次响应丢失，三个唯一命令收据；实际组名选择创建价目表，原匿名购物车登录接管后单价从 `779` 变为 `321.123456789012345678`，移除成员后数量更新恢复普通价格。中英手机购物车及中文后台截图已检查，无浏览器错误。九项 SQL 防护覆盖门禁、当前操作者、过期版本、变更重放、改名软删、外来/未验证账户、部分移除失败及末尾收据故障；全部测试变更回滚。原订单、实收、退款、库存、设备及履约指纹不变，自有数据库/Redis和进程清理完成。

Go 五个相关包测试、三个包 vet、Admin TypeScript 检查通过。之前测试选择器误用和缺少第二账户夹具的失败证据保留；最后一次还等待实际购物车渲染再截图，避免把加载状态作为界面证据。

本机专用库已备份并显式应用 0107（新增 1、跳过 106），管理门禁已启用，通过现有 `scripts/start-retail.ps1` 受控恢复三端及原支付/退款 worker。实际后台登录只读核对组目录与价目表选择，前台商店读取通过，无业务写入和浏览器错误；9100 health、9200 客户组页、9300 英文商店和中文购物车均返回 200，近期服务日志无本次异常。旧 Medusa 9001 health/app 继续按原迁移安排关闭，未作为新服务故障隐藏。

此项不代表客户组定价整体关闭：[后续商品与搜索报价](customer-catalog-pricing.md)已补登录专属展示，公开 Gin 端点保持匿名；[先登录后首次新车绑定](customer-first-cart.md)已完成局部联合，换机新商品报价和组优惠规则联合仍需补验，完整 C01—C11 继续开放。完整业务流程按用户要求留到大任务结束后一次执行。
