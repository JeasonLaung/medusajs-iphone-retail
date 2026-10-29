# 事务内促销属性用量读取

2026-10-29：`promotion.ReadAttributeUsages`从同一调用方事务中读取ComputeCandidates所需的私有属性用量。输入是ReadActive得到的内部候选及服务端可信购物车上下文。读取沿用调用方快照，不开新事务、不加当前行锁、不预留预算、不提交。最终登记必须继续调用已有当前锁读入口。

只对有应用方式且预算为use_by_attribute的候选查询；共享budget ID去重，冲突属性名/值整体拒绝。customer_id使用顶层非null值，否则回退customer.id；customer_email只使用email。空值、数字、未知属性路径均拒绝，不从metadata、旧账号或匹配邮箱推导新账号身份。ComputeCandidates复用同一个属性投影函数，避免读取和计算分别解释客户来源。购物车所有权与新登录态仍必须由外层命令验证，本函数不能授予身份。

查询只选择用量ID与精确scalar/raw金额，budget_id有效条件加完整BINARY attribute_value匹配；大小写、尾空格和超过索引前缀的长文本不合并。最多读取两条，发现两个有效记录即拒绝，承接结构阶段尚无完整文本唯一约束的hold。软删除记录不匹配；缺行返回明确Found=false而非省略map项。非法raw不回退四舍五入scalar，任何错误不返回部分结果。输入不超过1000候选、1MiB上下文、65535字节属性值，raw读取有上限。

返回值包含客户ID/邮箱等私有事实，只供同一命令内部计算使用，禁止充当公开DTO、日志、回执或缓存键。报告只记录布尔验证范围，不记录属性文本。预算登记/回执继续使用其自身锁后事实和历史delta，不复用本预览快照作为授权。

## 证据

完整隔离`test-admin-auth.py`的PromotionSchema suite运行verifyPromotionAttributeRead：验证无候选不加载、autocommit/schema gate、非法/空上下文、明确缺行、长文本/大小写/尾空格、共享预算去重、raw超18位精度、两个有效行拒绝、软删除/重建、非法raw拒绝、RR快照期间外部更新不改变本次用量、customer_id回退与空字符串不回退、email来源、未知属性和冲突候选。测试仅写独占人工数据库的attr-*行并清理，不接触应用客户。

另在同一只读事务调用ReadActive和ReadAttributeUsages，供真实配置到ComputeCandidates的精确金额测试使用，不再手工伪造Found=false。以admin-auth-report.promotion_attribute_read及implementation_sha256匹配为通过证据。

仍缺购物车服务端事实装配、新登录态关联、旧调整项移除/新调整项保存、命令版本与幂等、checkout库存支付协调和Astro接入。T032/T033未验收。
