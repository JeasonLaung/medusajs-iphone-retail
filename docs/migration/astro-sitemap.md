# Astro 站点地图

沿用原 `medusa-store/apps/storefront/src/app/sitemap.xml/route.ts`：每个有效地区国家生成中英文首页、商店、关于、帮助、评价、博客及原文章、回收入口；商品handle和原规则有效的每个SKU slug、未软删除合集进入发现列表。URL去重，配置noindex或不同canonical时排除，页面与商品/合集更新时间按原规则选择，保留自定义hreflang及默认en/zh-CN/x-default。1000条一片，超过时根返回索引，显式part边界及404保留来源Number解析行为。仅发布XML，不向搜索服务提交。

Gin新增 `/api/store/seo/catalog?kind=products|collections&limit=100&offset=0`，只投影handle、更新时间和有效SKU slug；没有ID、原始metadata、价格、库存或凭据。商品受当前服务端绑定商店/活动渠道、发布及软删除约束，SKU沿用现有display解析和软删除规则，不以当前库存或选项完整性删掉来源发现链接。合集发现与商品导航读取分开，保留原站空合集发现，不修改既有导航的集合过滤。读取事务repeatable-read/read-only，最多100条/页，metadata先查总字节/单行字节及数量预算，失败不发布部分目录；未知参数、重复kind、非法分页400，绑定/依赖不可用503，均no-store。

Astro从Gin公开配置、地区、SEO页覆盖与专用目录逐页读取，检查返回分页及总量、时间、URL/语言/robots；失败返回503且不公开私有诊断，不依赖Medusa运行。集合、商品和SKU的链接均从当前数据库推导，未写死30机型或历史快照。原文案与样式未变，无新迁移、env或门禁变化。

专项实际MySQL验证103公开商品的100+3分页、草稿/其他渠道/软删除排除、有效/非法/软删除SKU slug、原微秒时间、无metadata/ID泄漏、空合集及关闭渠道/缺表503。前台测试另覆盖1000边界、去重、noindex/外部canonical、自定义语言XML转义、较新更新时间、空目录和无效part，以及分页不完整/依赖失败拒绝。源码级测试不能替代主运行及原站全量对照；完整23组回归、主二进制刷新和原22分片实际联合正在进行，当前不据此关闭C01或全C01–C11。

## 实际运行验收

上述源码阶段之后，[实际联合报告](sitemap-native-report.json)已通过：专项owner b27629c227fb、完整原23组owner b36e1b657474，1815冻结文件逐份核对，独立MySQL/Redis资源全部清理。Go全包、catalog race、catalog/server vet、221前台测试和Astro check462文件0错误/警告、17提示通过。

唯一主Gin受控binary刷新到PID40152；schema仍100，env、门禁、共享3306和Medusa未变，旧binary/facts保留。真实Chrome读取原站及Astro根索引和全部分片，均为22片/21,952唯一URL；仅归一化本地localhost/127.0.0.1的8000/9300端口，逐URL比较loc、lastmod和全部语言链接完全一致。XML通过浏览器DOMParser解析，实际导航索引及第一片contentType均application/xml；0/-1/invalid/越界分片404。分片顺序未比较，不声称字节或排序一致。每条URL的完整页面业务行为也不由XML对照推断。

主293非journal表/100迁移校验和/213FK/268原表及保留共享86数据指纹仍一致，原后端健康/Admin、新Gin和Astro商店/sitemap均200，近期SEO和目录日志无panic/fatal。没有业务写或provider调用。配置noindex/canonical/hreflang的实际Admin→数据库→Gin→Astro联合尚缺；这些覆盖规则当前有前台测试和独立SQL投影证据，不能称配置浏览器已验收。SEO后台与历史对账、地区交互及C01–C11完整联合仍开放。
