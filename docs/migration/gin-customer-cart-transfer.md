# 匿名购物车登录转移与自动订单归属

## 2026-11-02 接续：已有付款的匿名车首次绑定，金额不变

首次绑定现在可在原checkout冻结下，以同一个MySQL事务完成当前账户/双凭证验证、新归属、结账邮箱、商品价格/运费/税/客户条件优惠刷新和收据。进入刷新前认证原CREATE与加密会话、检查唯一collection/唯一session及无实收；刷新后用精确总额与币种决定是否保留。只有金额与币种完全不变才保存：原payment session、collection和CREATE原输入版本不变，checkout的effective版本与cart一起递增。持久归属仍写新账户专用表，不修改legacy customer_id。

生产客户HTTP使用已经配置的付款keyring/adapters校验原会话；没有付款配置时仍拒绝该分支，同客户无变更确认和普通未付款转移保持兼容。全程没有provider调用，不在SQL锁内发起外部请求。已付/需审核或未知CREATE未激活的状态不因登录获得变更权限。

这里仅完成“完全相同金额时保留付款”的分支。客户条件折扣、商品现价或低于分的精度变化仍要求旧付款失效/未知恢复/补偿协调，当前会整笔回滚归属、邮箱、商品与优惠等写入，返回冲突；这个限制继续是C02/C06未完成项，不标已有付款账户转移整体完成。

固定子流程新增：

```sh
node retail/scripts/test-context-payment-joint.mjs --scenario unbound-unchanged
node retail/scripts/test-context-payment-joint.mjs --scenario unbound-precision
```

实际浏览器验证先从原结账页创建匿名付款，再从原账户页登录，丢首次成功响应并刷新重放，核对新归属、版本6、原CREATE输入4/cart5/effective6和完整付款日志保留；另一账户仍拒绝。金额变化以及`120.000000000000000000001`商品现价不能被渠道分位舍入掩盖。专项MySQL检查收据末尾故障全表回滚、仅绑定客户后才生效的自动折扣导致全表回滚，以及已付状态拒绝。证据见[本项报告](customer-payment-attachment-report.json)，不包含成单、发货、售后或最终完整交易链。

## 2026-11-02 接续：同账户冻结结账确认

来源已明确同客户跳过邮箱更新和商品/付款刷新。本次允许已保存的`customer_cart_ownership`与当前登录账户相同的冻结购物车确认：先锁购物车，再验证店铺/代际/有效账户和两个Redis凭证，保留当前checkout并仅写幂等收据。购物车版本、客户编辑过的结账邮箱、商品/税/配送/优惠、付款请求/会话/金额和原CREATE版本保持不变。账户停用及另一个付款刷新操作仍拒绝，已关闭或失效的购物车不获得额外权限。

前端原先遇到任何付款日志便阻止登录转移；现在在原CREATE日志有效且私有payment-options确认已有冻结结账后，可以向服务器请求同账户确认。日志与付款绑定保留不动，不按浏览器提供的账户ID授予归属。首次已有付款的匿名车金额不变时现可按本文顶部接续绑定；另一账户接管仍拒绝，金额变化的付款失效/补偿协调和跨设备合并尚未交付。

使用[固定付款子流程](context-payment-subflow.md)，没有运行注册→支付→配货→售后全链。人工新账户通过正式注册服务创建，测试只把专用账户设为已验证，随后原页面登录、签名前台桥接、Gin客户HTTP、双Redis凭证、MySQL事务全部实际执行。不会复用旧账号，也不实际发信。

```sh
node retail/scripts/test-context-payment-joint.mjs --scenario same-account
node retail/scripts/test-context-payment-joint.mjs --scenario unbound-account
```

同账户场景先在付款前保存归属、修改结账邮箱，再从原结账页面生成唯一付款，移除客户cookie以模拟会话到期后重新登录。首次同账户确认成功后丢失响应，刷新由现有`current()`恢复同UUID收据；随后另一账户登录被拒绝。匿名已有付款场景首次绑定被拒绝。MySQL专项还校验收据末尾故障全表回滚、六并发唯一收据、禁用账户重放拒绝及另一个付款刷新冻结不被绕过。

本项证据见[去敏报告](customer-frozen-cart-transfer-report.json)：10状态英文390px局部回归，以及同账户中英×390/1440四组合从头通过；19项前端局部单元测试、Astro470文件0错0警告和Go cart/customer/identity通过。两轮冻结源码、执行二进制及自有容器/网络/数据卷清理均复核。原9001/8000和主预览仍是接续前离线状态，历史服务日志没有本项新运行记录。这里的同账户确认不代表“已有付款的首次归属转移”整体完成；后者继续使用下文协调要求推进。

矩阵中的独立场景各用独立限流键范围，仍执行原Redis八次/十五分钟限制，session/CSRF与真实HTTP逻辑保持原样。首次矩阵在同一回环IP连续登录后耗尽共享限流预算，第四组未通过；修正测试场景隔离后从头运行矩阵，不把此前三组拼接为整轮通过。另保留账户签名桥配置遗漏和手机概览容器本身无尺寸造成的早期测试失败记录，均未作为业务通过证据。

本批继续 C02/C06 的来源登录购物车转移。新功能默认关闭；主 `eurorefurb_local_trial` 仍为 86 项迁移，未升级或启用。新增 `0095_customer_cart_transfer.sql`，与并行商品/评价迁移一起在专有 95 项迁移副本验证。[机器报告](customer-cart-transfer-report.json) 分别记录实际联合与最终完整 SQL 回归证据。

## 行为与来源

原 storefront `src/lib/data/customer.ts` 登录完成后调用 `transferCart()`，把当前 cart ID 转给来源 transfer-cart-customer workflow。原 workflow 同客户保持已编辑结账邮箱；不同客户更新绑定和邮箱并刷新商品。refresh-cart-items 的 `force_refresh` 不等于强制无地址计算税。

原生实现只关联当前持有的匿名购物车与已验证的新账户。固定客户 POST `/api/store/customers/cart-transfer` 只接收操作 UUID 和购物车版本；服务器依据客户和购物车的独立凭证决定账户、店铺、购物车及代际，先锁购物车，再检查账户与两个 Redis 凭证。浏览器不能提交归属账户 ID 或邮箱。持久绑定不使用历史 `carts.customer_id`，不按旧 ID 或相同邮箱继承任何历史订单。

初次关联在一个事务内保存绑定、账户邮箱、商品价格、已选人工运费、税费、优惠、金额、新版本和持久操作收据。无收货地址时保留来源税计算语义。原账户再次关联不覆盖客户随后编辑的结账邮箱。不同账户关联被拒绝。未知响应保留原 UUID/输入版本；明确失败与成功按约定清除浏览器日志。该日志仅含标识和操作身份，不含口令、邮箱或 cookie。Web Locks 与现有购物车命令共享锁，支付命令也拒绝未解决的转移日志。

付款成单在原完成事务中消费该绑定，原子写入新账户订单归属，无需付款 worker 持有客户 cookie，也不需要成单后的人工 attach。关闭“允许新转移”门禁后，已保存的绑定仍能被成单消费。归属写入末尾 SQL 故障会使整笔完成事务回滚。退出成功清除客户 cookie 及四个购物车 cookie，防止同一浏览器切换账户继承持有能力。Astro bridge 对旧 86 部署允许完整三项客户退出删除；新原生实现退出必须同时删除七项，部分购物车删除、发放购物车 cookie 和其他路由夹带均拒绝。

## 验证证据

专有 owner `1a34a50dc73c` 使用本机独立无网络 MySQL Unix socket（native foreign keys、foreign_key_checks 开启）和专有 Redis 命名空间；生产来源字节冻结，只有测试端口/连接适配。实际 Astro、Gin、MySQL、Redis、Chrome 联合，无业务 API 响应替身。付款是隔离 provider 适配器和 hosted 页，无真实支付或实际发信。

- 英文/中文 × 390/1440，共四个实际新注册验证账户：匿名商品加购 → 登录 → 转移 → 结账。每次故意丢失首次真实成功转移响应，再次登录均使用原 UUID/输入版本重放；保持同一购物车、商品和金额。四个唯一订单自动归属，人工 attach 审计数为零；本人无 cart 凭证可查、外人及退出后拒绝。浏览器异常为零。
- 同次完整 `TestOwnedCartRuntime` 41.15 秒通过，额外覆盖默认关闭、空购物车、无地址有商品、六并发一次关联、邮箱保持、外账户拒绝、关闭门禁后成单、归属末尾失败回滚。
- 最终 storefront 200/200 单元测试通过；Astro 检查 420 文件，0 错误、0 警告、7 hints。
- 源码 2456 文件逐字节 SHA 校验归档、执行二进制原始 SHA、SQL/浏览器日志和四张截图保留于 ignored `logs/gin-migration/native-sql-local-1a34a50dc73c`。全部自有进程终止、MySQL 数据和 socket 删除、74 个专有 Redis 键删除。仅删除已终止且可重建的二进制/dist，源码压缩归档逐文件复核。

两次此前联合失败独立保留：`7bbf52ccaf51` 的测试 PORT 未随动态端口适配，未进入业务 UI；`7b1f45a9e31a` 的 native Window.fetch 接收者错误导致登录转移失败，修复 fetch 包装并加接收者断言后实际联合通过。另一次沙箱回环监听拒绝发生在数据库初始化之前，单独留清理回执。失败不拼接为成功证据。

**最终 95 项迁移完整原 23 SQL 组已实际通过。** 空间恢复后，由协作线程单独执行 owner `91570716d79a`，controller 退出 0，23 组无跳过；1754 文件冻结源码、23 份原始日志及执行二进制逐 SHA 复核。37 个自有数据库与本次 Redis 前缀清理，独立 MySQL 终止、数据和 socket 删除回执均真。见[完整报告](native-sql-95-report.json)。本批生成器独立检查完整 suite 顺序、各组日志、实际 binary、全部冻结源与清理，并验证本批后端字节与四客户联合源一致，才输出 `full_23_sql_suites_passed=true`。

首次恢复运行 `a7df385f58c9` 在 PromotionUsage 本身通过后，因 Redis 16379 停止、清理连接拒绝导致 controller 退出 1；完整失败记录保留，恢复原 Redis 后从头运行原 23 组，没有拼接早先 94、22+1 或失败轮证据。此前磁盘阻塞记录保留为历史，不再是当前阻塞。

## 仍未完成

### 已有支付会话的来源核实

新增可重复执行的 `node retail/scripts/capture-customer-cart-transfer.cjs --check`，直接提取已安装来源的纯回调，核对两个安装副本的三份 workflow 原始 SHA 一致，保存 [9 个来源案例](fixtures/customer-cart-transfer-source.json)。不启动应用、数据库或 provider，不能据此宣称真实工作流执行通过。来源匿名 cart 即使已有 collection 仍进入转移；相同账户仍跳过邮箱/刷新。不同账户刷新后，collection 精确金额和币种都不变时保留支付会话；任一变化才删除会话并更新 collection，低于分的精度差也不能忽略。当前原生“凡有 collection 一律拒绝”仍是缺口，不能当作最终交付。

继续实现时应复用现有 `PreparePaymentRefresh` / `StagePaymentRefresh`、独立 provider effect 收据和补偿流程，不在购物车 SQL 锁内调用渠道。至少还需实际验证：金额不变保留同一会话、客户条件折扣改变金额后的旧会话失效、响应未知禁止重发、部分失效补偿、回调成单与转移并发、最终失败不留下半归属。已有 `checkout_attempt_id` 冻结和支付 effect 未知状态必须纳入协调，不能仅去掉 `lockCurrent` 检查或 collection 拒绝分支。这些条目尚无本批真实 SQL/浏览器证据。

已附着支付会话或已冻结结账的匿名购物车，在金额不变且原付款可认证时可按本文顶部接续首次绑定；金额变化的首次归属转移仍未接通。已绑定同账户的冻结购物车可进行无变更确认。来源完整支付会话刷新/失效协调、跨设备购物车合并尚未交付。当前默认关闭；需要显式迁移、后台门禁和 storefront `PUBLIC_CUSTOMER_CART_TRANSFER_ENABLED=true` 才能启用。主 MySQL 的已有外键配置问题未被本批修改；独立 native-FK 验证不证明主配置能拒绝所有非法写入。

C02/C06、真实渠道/邮件及 C01–C11 的其余业务验收均继续开放。共有文件混有并行商品/评价变更，暂未提交；完整回归已通过，仍待双方提交范围协调形成完整提交。
