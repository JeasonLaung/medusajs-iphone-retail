# 购物车页面的保存数据投影

2026-10-28。原购物车 `CartLine` 消费名称、商品 handle、缩略图、变体配置及行金额；金额接口原先只有 SKU/数量/价格，不能通过当前公开目录重新猜测已经保存的行。新增私有 `GET /api/store/carts/:id/view`，为原页面迁移提供保存快照，不新增数据库迁移。

响应只包含 `amounts` 与 `items`。amounts 使用原精确金额 DTO，items 明确投影 id/title/product_title/product_handle/thumbnail/variant_title/option_values；不暴露 metadata、客户/地址、库存位置、支付、归属代数或凭证。nullable 来源字段保留 null，NULL 配置输出空对象，按创建时间降序及 ID 稳定排序。商品已下架、变体已删除时仍保留保存事实，不把不可买的行变为空购物车；移除能力沿用已验证的独立命令。

`ReadAmounts` 与 `ReadView` 共用当前归属校验及只读 REPEATABLE READ 事务包装。view 在同一快照读取金额和保存展示，验证两者行 ID 集合完全一致，保留原金额接口响应不变。当前持有凭证/店铺/未撤销代数、未完成/未删除草稿、能力与模型检查先执行；无重新选价、续 Cookie、预留、写入或 provider 操作。

展示字符串限 UTF-8 4096 字节并拒绝控制字符；配置最多64项、来源JSON最多32KiB，整体编码不超过4MiB；超过限制整体失败，不返回部分金额或部分商品。未来页面将通过文本节点和明确本地路由/图片协议处理来源文本，不直接插入 HTML。

`CartClient.retrieveView` 使用同源 HttpOnly possession，GET 不发送 CSRF 或手动身份头；严格拒绝未知字段、重复/缺少/外来行、错误标量和配置类型、超限、checkout=true 等不可信协议。401/404 可表示当前 ID 已不可访问，依赖/网络/协议错误继续抛出，不当空车。无 SSR 私有数据序列化或浏览器凭证存储。

## 验证边界

真实自有 MySQL/Redis/Gin 专项检查已下架 SKU 的保存名称/配置/精确金额、删除行排除、空配置/非法JSON类型、无凭证/其他车/查询参数/撤销拒绝、只读指纹及金额与展示之间并发提交仍保持单一快照。首轮发现 Gorm 对嵌套 DTO map 误作持久列，已用明确忽略标签修复；另修正测试复用 JSON 目标 map 导致的旧键残留，保持空配置断言。

Astro客户端增加一项严格 DTO 正反例，单元总数40；真实浏览器用正式客户端读取加购车的保存数据及删除后的空展示。结果及源码 SHA 见 cart-runtime-report、admin-auth-report、cart-client-report、astro-runtime-report 和 admin-live-report；未通过整组时不据旧报告声明本实现通过。

联合浏览器首轮发现原可购测试商品没有配置选项，不能证明保存配置承接。现补充真实容量选项、商品允许值及变体选值关系，断言精确等于 `Storage: 128 GB`。修正关系字段时上一轮测试二进制已编译，仍用旧测试 SQL 而失败；运行结束并清理后重新编译再跑，不弱化配置断言，也不以旧成功报告替代。

本批仍未迁移原购物车整页 DOM/CSS、导航或数量交互协调。当前公开 items/checkout 保持 false，金额是保存快照，不代表实时税费促销配送已重算或允许付款。T025/T026/T032/T033 继续待完整验收。

最终整组通过：完整及购物车专项数据库回归、Go全量测试/vet、40项Astro单元/类型/build、40对展示区及24对操作区比较、零漏洞完整审计、3项真实浏览器和Astro原按钮两宽验证。真实客户端精确读回 `Storage: 128 GB`，删除后items及金额lines均空；保存展示不改变既有三车/七操作回执/零预留SQL期望。五份报告157/101/11/175/176项源码SHA全部匹配，自有容器/网络/二进制完成清理。旧9001 health/app与8000商品/购物车完整响应200，旧stderr大小/修改时间未新增，既有locales 404仍保留。

## 原页面接续要求

本批已核对原 `app/[countryCode]/(main)/cart/page.tsx`、`modules/cart/templates/index.tsx`、`templates/summary.tsx`、`components/cart-line/index.tsx`、`components/cart-interactions/index.tsx`、`i18n.ts`、`styles.module.css` 及 `lib/data/cart.ts` 的更新/删除调用。

- 展示继续使用原两列布局、空车/三项说明、商品图/等级/电池/描述、继续购物与登录提示，CSS及中英文复制留有来源；不因 Ant 后台选型替换前台控件。
- 行按创建时间降序显示，数量与摘要均来自保存行/精确金额，不能把缺失目录 SKU 隐藏。原当前变体查询用于库存提示、最大数量和配置链接；读取失败时保留原未知库存提示，服务端提交仍作最终判断。
- 数量步进/输入失焦/回车与移除按钮继续使用全车 busy，非法数量在控件附近显示原文案。新实现需在请求前锁定并持久化操作键及预期版本，提交后重新读取联合 view；响应丢失、刷新或跨标签竞争不得盲目换键重复执行。
- 摘要保留商品小计、未选择配送时的结账计算提示、折扣、税费和总额。金额显示使用现有精确字符串格式工具，不能用浮点乘数量重造行小计。结账链接待完整能力可用后接入原 checkoutStep 地址/配送/支付分支。
- 原公共 main layout 还包含 Nav、Footer、FloatingActions、客户/购物车地区不符提示及免邮提示；它们和根字体/SEO必须另行承接，局部 CartTemplate 对照不证明整页完成。
- 保留 PageEntryScroll 的内容挂载后无 hash 才滚顶行为。独立比较需覆盖中英、390/768/1440、空/多行、原输入/忙碌/失败/缺货与移除后空车，再用真实 Astro→Gin→MySQL/Redis 验证成功链路与恢复。

2026-10-28 当前变体观察需要补充 nullable product_id：在相同只读快照中从保存行显式投影源 ID，保留已删除商品及 null，不通过目录 join 丢行；非法 ID 拒绝整个响应。客户端将字段加入严格私有 DTO，并使用原 ID 读取 Gin 公共商品详情，见[购物车当前变体](cart-current-variants.md)。
