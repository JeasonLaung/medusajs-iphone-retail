# 正式合集页与目录筛选

`/{countryCode}/collections/{handle}` 已接正式 Astro SSR 与 Gin 公开目录，承接原 `app/[countryCode]/(main)/collections/[handle]/page.tsx`、CollectionTemplate、PaginatedProducts、RefinementList、SortProducts、Pagination、ProductPreview、Thumbnail 和 PreviewPrice。原布局、商品图/占位、颜色、价格/促销参考价、两列桌面/单列移动列表及每页12件保持；排序仍使用源页面的英文文字，中文国家语言路径沿用原行为。原页隐藏选项选择器，仍保留已有URL参数解析。

合集通过 handle 精确查询。Gin 公开范围仍限服务器绑定商店、启用渠道与已发布未删除商品；未知合集404，目录失败503，真实空列表200。无效或重复的page/sortBy返回400，避免NaN/负数页产生错误结果。SSR输出首屏商品、原合集标题/description及canonical。公开读取不转发用户Cookie、认证或浏览器请求头；筛选与排序导航保留其他参数，排序清除page。分页和排序使用站内完整导航替换Next路由依赖。

## 原排序和选项语义

原 `listProductsWithSort` 只取100件商品再进行价格排序与12件分页。继续保留这个现有上限：最新选择由Gin在LIMIT前按创建时间降序、ID稳定补序；价格排序保持原默认ID工作集顺序后在Astro按最低variant价格排序。金额使用精确十进制比较，不转Number。无报价变体保留原排序键0、无变体保留原Infinity位置，但展示仍为无报价，不伪造价格。普通金额与原sortProducts源码执行结果对照通过；超出Number精度的值另外验证。

Gin `/api/store/products` 新增受限 `order=-created_at` 和至多100个重复 `option_value_id`；详情接口拒绝这两个列表参数。选项语义以本地安装的Medusa 2.21.2 `product/dist/utils/build-option-value-filter-query.js` 为准：同一个未删除SKU需要满足每个已选option组；同组值任选其一，未知值不形成新组，全未知返回空。额外检查本期商品的有效option/value成员关系，防止历史异常引用参与筛选。仍不能查询其他渠道、任意SQL排序或未发布商品。

## 验证与边界

- 独立暂存内容快照含40份已提交迁移，不含工作区未提交0037付款通知；Go全包通过。MySQL/Redis由测试独占、无发布端口、无外网，限制各1CPU/768MiB和128MiB。`python retail/scripts/test-admin-auth.py --store-collections-only` 覆盖目录/价格/地区/库存/分类/搜索和本次合集查询：筛选后计数、排序在分页前、同SKU跨组匹配、不同SKU拼凑拒绝、删除成员/值/选项/变体拒绝，读取前后指纹一致。两个测试容器与临时二进制清理完成。
- 完整23套件尝试在ProductPrices阶段Docker观察超时，容器已清理；不记录为完整回归通过。将最终修改独立重跑ProductPrices专项通过，相关SQL证据见 `store-collections-runtime-report.json`。新专项选项便于重现，不更改其他套件门槛。
- Astro check通过：260文件，0 error/warning、2旧hint；103项单元测试及构建通过。
- 正式构建Astro运行于测试独占19374，Chrome与合成API验证SSR、排序/翻页、清除页码、保留重复选项和其他参数、原商品链接、促销价、商品原图实际解码、占位图、404/400/503与空列表、中英路径及390/1440宽度，无浏览器异常。截图存于忽略目录 `logs/gin-migration/collection-*.png`，人工检查移动/桌面布局。
- 浏览器使用合成API，SQL专项使用真实Gin/MySQL，两者不等于同一浏览器联合验收；尚未做原页面同数据像素对照。全集商品运营、购物车到结账/成单、真实支付/通知、客户订单/回收/售后仍按本期清单继续，不因合集路由完成宣称全站闭环。
