# 后台订单履约详情

2026-10-28。Gin 私有 GET `/api/admin/orders/:id/fulfillments` 与 React 订单详情的 Ant Design 履约区域承接订单关联包裹、商品明细、物流标签、配送地址及独立的记录时间。页面使用现有主题、Descriptions、Table、Tag、Alert、Empty、Spin、Button 与 Typography，保持原商品、成交行、金额及支付区域。

## 查询语义

一次只读 Repeatable Read 事务核对订单和履约模型、读取当前有效订单及有效 order_fulfillment 关联，再按包裹读取明细、标签和有效地址。响应 `order_id/version` 用于阻止前端混用不同订单版本；来源关联本身没有 version 字段，展示的是订单整体关联履约历史，不伪装为仅当前版本产生的包裹。

每条关联包含原包裹 id、availability 与 nullable record。`available` 保留实际字段，`missing` / `deleted` 保留原编号且 record 为 null；页面明确提示记录缺失/已删除。已经软删除的订单关联不展示。地址引用保留 delivery_address_id，地址已删除或不存在时 address=null；地址没有引用时显示“未记录配送地址”，两者不同。

物品保留原 title、SKU、barcode、line_item_id、inventory_item_id 与精确字符串 quantity。数量沿用原 raw_quantity 优先及严格精确十进制规则，不经过浮点数；未关联成交行或库存项保持 null，不自动推断。包裹保留仓库、nullable provider/配送选项、是否配送及创建/打包/发货/送达/取消时间，各时间单独展示，不推算状态或库存。

标签保留原 tracking_number、tracking_url、label_url。浏览器只为不含用户名/密码的绝对 HTTP(S) URL 提供新窗口链接，带 noopener/noreferrer；其他原值显示为转义文本。不会自动请求标签地址、嵌入文件或执行 provider 返回的 HTML。地址逐字段投影，保留 SQL NULL、空字符串、前导零邮编与电话。

最多 100 条订单关联、全单各 1,000 条有效商品明细与标签；超界整体 503，不截断后返回完整成功。未知或重复查询参数 400，订单缺失或删除 404，数据库/结构/非法原始数量等固定 503。响应使用认证、Origin 与 private/no-store 边界，SQL/JSON 均排除 provider data、metadata、创建人、操作人及密钥。operations_enabled 始终 false。

前端使用独立请求和重试，不因履约失败隐藏其他订单事实。401 结束会话，请求取消不写入旧状态，版本变化单独提示。校验 envelope、可用性/record 一致性、精确数量字符串、nullable 地址/时间、地址 id 关联和全单数组上限，并显式重新投影字段后存入页面状态。

## 验证

[人工 SQL 报告](admin-auth-report.json) `order_fulfillments` 验证当前/其他订单隔离、有效/缺失/删除包裹、地址及子项软删除、原始长编号和超 JS 安全整数且含 18 位小数的数量、地址字段别名、SQL NULL/空串、非法 raw 拒绝、禁用结构拒绝、子项上限拒绝及全业务指纹不变。

[固定来源整合报告](catalog-mysql-rehearsal-report.json) 在商品/库存/客户/订单/支付/履约共同导入后，通过实际查询逐字段与独立 SQL 比较 13 个有效订单的履约关系及 3 个包裹、3 个地址、3 条明细。查询与重放前后全部业务、回执、hold 指纹不变。固定来源中物流标签为零，长物流单号与不支持的 URL、缺失/已删除包裹由人工样本验证，不冒充实际来源样本覆盖。

[UI 状态报告](admin-ui-report.json) 的 16 项浏览器测试覆盖独立失败/重试、空态、版本变化、数字形式数量拒绝、会话过期、转义文本和安全标签链接；此报告使用人工 HTTP 响应。

[真实浏览器报告](admin-live-report.json) 使用本机 Edge 经 Vite 到实际 Gin + MySQL + Redis，无响应拦截；验证真实履约查询、Ant 明细、原数量/SKU/物流单号、地址、缺失/已删除提示、未知参数 400 和退出后 401；同时回归既有商品、库存、订单与支付操作，中英文及 390/768/1440px 截图检查。

## 剩余范围

目前只有新 admin 身份的读取边界；逐角色/仓库/供应商授权、完整履约导航和筛选、IMEI 分配、部分履约、库存原子联动、发货/取消幂等命令、真实物流 provider/标签生成与回调、正式全域导入协调器及 Astro 尚待实现。没有恢复旧登录账号、调用物流服务或操作生产/部署服务器。T038 和整体迁移继续进行。
