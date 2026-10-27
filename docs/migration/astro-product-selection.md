# Astro 商品选择与深链数据组合

2026-10-28。新增独立 product-variants 和 product-page 模块，承接原商品页/操作/图库的纯数据行为。旧页面、CSS 和服务未改，新模块尚未挂载到 Astro 业务页面；不将 URL 字符串测试视为真实 React 岛、原生 History 或页面保真验收。

## 选择与可售

选项排序保留 storage → condition → battery → color → colour，其余维持来源稳定顺序，不改变输入数组。完整变体要求 Gin 的 configuration_complete=true，并具备商品所有维度的非空值。改变一组值时只受此前的选项约束，允许重选后面的电池/颜色等配置；在可选组合中选择保留当前其他值最多的一项，评分相同保留原顺序。未知选项返回无候选。

可售判断使用 Gin availability.can_fulfill 及 available/backorder/untracked 状态，且要求配置完整，不以 pooled 库存或 allow_backorder 单独绕过冻结位置。这是已记录库存保护规则的前端承接。指定一个有效但无货的变体仍保留选择，以便显示缺货；无定位时先第一个可售完整变体，再第一个完整变体；全不完整时不选默认。

原选择优先级保持：路径 slug（不区分大小写匹配）→ v_id → 初始 ID → 默认。选项值定位要求全部维度和值精确相等。图库与购买默认分别解析：明确定位变体且有变体图时按原变体图片顺序，否则使用模型图片，不因默认购买选择而悄然替换初始模型图；过滤无 URL 的图片。

selectionURL 保留原 slug 或 v_id 更新方式：有 slug 时删掉全部 v_id，使用该 slug 路径；无 slug 时保持当前 pathname 并写入原变体 ID，保留其余重复 query 及 hash。selectionMetadata 提供原切换后的标题及 canonical 路径，仅有 slug 时更新。模块只返回目标数据，尚未调用 history.replaceState、注册 popstate 或修改 document，因此相关实际浏览器行为仍待 React 岛接入验证。

## 服务端页面数据组合

loadProductPage 在本次请求内读取当前区域，按国家语言段取真实国家归属，不保存进程级国家 Map、不回退默认币种。根据原 productHandleFromSlug 语法从组合 URL 推导 swappie 模型 handle，再用 Gin 的精确 handle + region_id 读取；不存在或未知国家返回 null，依赖/DTO/模型歧义失败抛固定错误，不伪装 404。

组合 slug 必须匹配模型下唯一的完整变体，未知组合或不完整组合返回 null，重复匹配拒绝；不默默使用第一项。普通模型 URL 的未知 v_id 继续原有默认行为，不作为新业务规则改成 404。读取最多两个同 handle 商品以发现歧义；区域和产品报价币种不一致拒绝。元数据沿用原页面标题、description、canonical、OpenGraph 标题/缩略图；模型页的 v_id 不改变服务器默认 canonical，slug 页保留来源 slug 大小写。全局 SEO 覆盖及翻译替代链接尚须另接现有契约。

此模块未绑定 Astro secret origin 或国家重定向 middleware，未来页面调用方必须使用服务端配置；不接收浏览器任意上游地址。页面数据中明确区分 requestedVariantID、购买初始变体及图库图片，以保留原操作与图库分别定位的行为。

## 证据与剩余

[Astro 运行报告](astro-runtime-report.json) 当前 16 项单元测试。新增五组中，以原 product-variants.ts 作为独立纯函数参考，320 组选项变化对照候选、最优选择和稳定顺序；并核对原 ID/slug 优先级、图片回退、无货/不完整、不可用冻结、query/hash、模型和组合不存在、重复 slug、区域归属及依赖异常。测试仅动态加载旧纯函数并擦除 type import，不启动或依赖 Medusa/Next 运行时。报告记录参考 helper、原 page、actions、gallery 的源码哈希。

独占真实 Gin/MySQL/Redis 串联额外验证按 dk-zh + fixture-b 的国家/handle 数据组合、未知国家/商品及全不完整变体无默认；这是源模块集成，生产 Astro 构建仍只有 health。完整配置和 slug 的人工行为覆盖不冒充真实浏览器商品页验收。原后台浏览器链路继续回归。

原页面 DOM/CSS/资源/字体、React 岛和 History/后退、cart 数量上限/加购、评论/关联商品、搜索筛选排序和完整交易仍待迁移。Ant Design/ECharts 仍仅是后台选型，不替换前台样式。
