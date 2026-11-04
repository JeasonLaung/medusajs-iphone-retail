# Trustpilot 配置

本接入采用官方 TrustBox，不抓取评论，也不需要 API Key。后台入口：<http://localhost:9001/app/settings/trustpilot>（设置 → Trustpilot）。

尚未开通 Business：先注册免费 Business 账号并认领商家域名。免费套餐的 Review Collector 不等于这里需要的总评分及评论列表组件；展示权限以后台套餐为准。若改用商家 API，需要 Business 账号及 API module 权限，公开 API 同样需要 API Key。参见 <https://developers.trustpilot.com/introduction>、<https://developers.trustpilot.com/authentication> 和 <https://help.trustpilot.com/s/article/Use-Trustpilot-for-free?language=en_US>。

## Trustpilot Business 后台

1. 登录并认领自己商城域名对应的商家主页。
2. 打开 Showcase → Website widgets（TrustBox；菜单名称可能随后台语言变化）。
3. 在套餐授权范围内，选择一个能显示 TrustScore 的紧凑组件，以及一个商家服务评价列表或轮播组件。商品评价组件不适用于这里。
4. 分别生成嵌入代码，复制其中的 `data-businessunit-id`、两种 `data-template-id` 和 `data-style-height`。商家 ID 应一致。
5. 总评分区域在手机端较窄，需选择适合窄列的模板，并用实际组件检查布局。使用需要额外选项或筛选参数的模板前，需进一步适配；当前接入支持基本商家服务评价组件，不设置星级筛选。

## Medusa 后台

1. 设置 → Trustpilot → 编辑。
2. 填写 `https://www.trustpilot.com/review/你的域名`、商家 ID、总评分模板 ID、评价列表模板 ID，以及官方代码中的高度（像素）。
3. 打开启用开关并保存。配置保存在 Store metadata 的 `trustpilot` 字段，无需数据库迁移。
4. 刷新商城 `/dk-en/store`、首页 `/dk-en`、评价页 `/dk-en/reviews`，以及对应 `/dk-zh` 页面。
5. 确认分数、评论、商家链接和手机布局正确。配置未启用时不加载 Trustpilot 脚本；原来写死的 4.4 分已移除，首页保留站内评价。

启用后商城顶部使用总评分组件；首页和评价页使用服务评价组件。商品页及订单内的站内商品评价、审核和历史数据保留。组件失败或被拦截时保留商家主页链接，不以站内评分冒充 Trustpilot 评分。

接入只改变展示，不会上传历史站内评论、创建 Trustpilot 评价、发送邀请或购买套餐。没有实际账号和授权组件代码时，不能完成真实数据联调。若要将商品详情页也迁移至 Trustpilot Product Reviews，需要另行配置商品 SKU 对应关系和商品评价组件。

官方说明：
- <https://help.trustpilot.com/s/article/What-is-a-TrustBox-widget?language=en_US>
- <https://help.trustpilot.com/s/article/TrustBox-widget-placement-tips?language=en_US>
- <https://corporate.trustpilot.com/legal/for-businesses/legal-brand-guidelines/feb-2023>

## English

Open Settings → Trustpilot in Medusa Admin. In Trustpilot Business, generate an authorized compact TrustScore widget and a service review list/carousel. Copy the business profile URL, `data-businessunit-id`, each widget's `data-template-id` and pixel height into Medusa, then enable and save. No API key is needed. Check home, store and reviews pages in English and Chinese, including mobile layouts. Existing product reviews and moderation remain separate. Historical reviews and invitation sending are not migrated by this display integration.
