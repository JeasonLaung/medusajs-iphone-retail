# Astro 商品与精确价格适配

2026-10-28。新增独立 TypeScript 商品 DTO、Gin 匿名传输和精确金额工具，供后续原页面迁移复用。它们不导入 Medusa SDK、Next 或 Ant，不回调旧服务，不改变旧前台页面。T025/T026 及整体商城仍未验收。

## 读取边界

`loadProducts` 对接固定 `/api/store/products`，接受现有 Gin 支持的 limit/offset/q/handle/category_id/collection_id/region_id/quantity；`loadProduct` 按原商品 ID 请求详情，只接受区域和数量。调用方提供服务端配置的 origin，不能传入客户端币种、渠道、任意 fields、Cookie 或认证头。查询编码和输入范围与当前接口一致；未知字段、空筛选和非规范数量拒绝。

请求不缓存、不带身份、不跟随重定向，超时五秒，商品响应实际字节最多 8 MiB；要求成功 JSON、有效 UTF-8。详情 404 返回 null；列表依赖失败、详情非 404 失败、200 的 null/损坏 DTO 均报固定错误，不用空商品掩盖故障，不公开上游响应体和地址。服务端秘密 origin 尚由未来 Astro 页面调用方绑定；这些纯模块本身不读取浏览器配置。

投影明确列出商品、选项/值、图片、变体、精确价格和可售观察字段，丢弃 metadata、预留量及其他扩展。校验关联归属、ID 唯一、已声明完整变体的维度、变体图片与商品图片的一致性。来源重复维度等异常可继续表现为 incomplete，不能因为公开子集看似完整而升级状态。检查分页计数及回显参数、最多 1000 变体/图片和 10000 条响应关系，损坏的一页整体失败。

库存数量保持十进制整数字符串或 null，检查单仓容量不大于汇总库存，untracked/incomplete/available/out_of_stock/backorder/unavailable 与跟踪标记、配置完整性和本次数量一致；不由配置价格推导可售，不把观察视为下单授权。未请求区域或变体不完整时不能有报价。capabilities 继续要求 checkout=false。

## 金额与旧交互承接

旧 get-product-price/money 依赖 number；新工具使用有界十进制字符串和 BigInt 对齐，精确比较最低价、保留选中原 ID/SKU、计算带符号的选项差价及百分比，不经 Number、不除以 100、不修改输入或对原数组排序。原价为 null 时展示回退到现价，缺价返回 null，真实零价保留；零原价的百分比为 null，不输出 NaN/Infinity。最低价仍在所有完整且有价变体间选择，与库存可售观察分别处理。

金额显示沿用原 Intl 货币与 locale 习惯，默认 en-US，商品详情可显式使用 en-IE/zh-CN；只在显示时按币种小数位舍入，不改变业务金额。直接传精确字符串的依据是 [ECMA-402 ToIntlMathematicalValue](https://tc39.es/ecma402/2025/#sec-tointlmathematicalvalue) 和 [NumberFormat.format](https://tc39.es/ecma402/2025/#sec-intl.numberformat.prototype.format)：字符串作为数学十进制值处理。当前 Node 22.23.4 运行时验证 `9007199254740993.005` 显示为 `€9,007,199,254,740,993.01`，`1.005` 为 `€1.01`。未来 React 岛仍须在支持的实际浏览器验证，不能据本轮 SSR 单元结果宣称浏览器兼容已全验收。

## 验证与剩余

[Astro 报告](astro-runtime-report.json) 记录 11 项单元测试（含后续展示扩展）、类型/build、依赖审计和生产 SSR 健康验证。新增七组用例覆盖低位小数/大额、零价/缺价、货币和语言格式、精确差价、完整与不完整关系、异常状态和上游失败、响应上限及未知覆盖拒绝。直接用源适配模块连接独占真实 Gin/MySQL/Redis，验证公开列表、区域/数量详情、不完整变体无报价、404 与依赖错误区分；完整可售/报价样本的前端语义目前由人工响应测试覆盖，不能将其描述为真实数据或生产页面验证。

生产 Astro 的实际入口仍只有 health，其构建产物当前只调用区域读取，商品模块尚未被业务页面引用。原国家路由、商品筛选/排序、slug、展示扩展编辑、页面/CSS/字体/资源、React 选择及购物车等仍待逐项承接；未迁移页面保持 404，不创建替代商城或重新设计页面。

2026-10-28 [展示 DTO](store-display.md)已增加明确颜色、消费规格、来源提示、容量上市价及变体 slug/示例提示，Astro 同步验证，上市价 helper 保留容量/币种和历史参考回退。商品模块仍未进入实际页面。
