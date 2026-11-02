# Gin 公开 SEO 配置（源码与隔离 SQL 阶段）

2026-11-01。实际主99库没有seo_pages/seo_settings，不能把既有关系设计当作运行表。已与获用户授权协调的另一任务确认0100未占用，新增显式 `0100_public_seo.sql`，从原关系设计逐字段保留两表：原ID、完整path/key/文本、可空原始JSON、时间和软删除，settings value为既有merchant-config加密包络。没有登录数据或SEO provider凭据表，不在启动时建表。

新增 `internal/seo` 与API入口注册 `/api/store/seo?path=...`、`/api/store/seo/pages`。前者只读取原站publication公开15项key（包括有效站点origin），只解密is_secret=false、未软删除且无merchant hold的行；所有非secret值仍按原导入器merchant-config/table/column/row上下文打开。secret、非allowlist、历史hold都在解密前排除。没有全表秘密解密或原文日志。保留原path正规化、无请求cookie/客户凭据依赖，返回no-store，参数400与依赖503区分；失败不公开数据库/包络详情。

页面DTO只含公开title/description/canonical/og_image/hreflang/structured_data/robots/path，发现列表只含path/robots/canonical/hreflang/updated_at。完整key/path重复不会任意选一行，返回不可用，保留来源完整文本而非截断。canonical/hreflang/robots等运营输入约束还需后续Admin写入时复用来源验证；本轮不提供任意写接口或自动外部提交。商户keyring未配置时允许空配置读取，存在需解密的公开设置则失败，不以明文或cookie-secret降级。

## 实际数据库证据

[完整报告](seo-public-source-report.json)保留两次不同owner的冻结与资源清理。ownercca0488d510a在专属私有socket MySQL、原生外键启用环境实际建立99基线，再执行100迁移（1 applied/99 skipped），重放0 applied/100 skipped，另fresh100。测试实际加密与读取原中英文/JSON、有效origin、公开字段白名单、secret/非公开/hold/软删除排除、发现投影、重复key/path、跨行密文移用、HTTP400/503/no-store与错误隐藏。

owner85413f54d3b3完整原23组SQL在100计划全部通过，39个专有数据库名字受guard约束，资源终止/清理证明齐全。identity原测试Applied/Skipped固定计数及schema嵌入计划断言明确更新100，未移除或放宽断言。控制器原SUITES仍是23组，新增SEOPublication仅可专有diagnostic执行，不把1组称完整回归。第一次focused清理socket后直接复用被控制器正确拒绝；保留原证据并新建资源身份，没有禁用guard。

Go全包默认测试、SEO/schema race、SEO/API vet通过；默认测试中跳过的opt-in SQL不计真实SQL证据，上述隔离报告独立记录。命令：`python3 retail/scripts/test-native-sql-local.py prepare --isolated-mysql`，再 `run --isolated-mysql --suite SEOPublication`，完整回归使用新owner的 `run --isolated-mysql`。

## 前台与当前运行边界

新增 `lib/seo-config.ts` 解析器/配置合成器，尚未挂入seo-head：只接受原公开key和匹配path，检查HTTP URL/语言/robots/结构化数据，保留页面覆盖/默认优先级、私有robots优先级、显式analytics/GSC开关及来源measurement格式；读取失败可回退已验证默认SEO。首页title和OG的已验证区别继续保留。213前台测试通过，Astro check453文件0错误/警告、17提示。配置测试使用合成输入，不能替代实际已配置数据库→Gin→Astro浏览器联合。

主预览仍99/PID372，不应用100、不替换binary、不改env/门禁。readonly verifier确认291业务表与fresh演练一致、268原表不变、99校验和/213FK无孤儿、共享86保留数据仍一致。9001健康/Admin、9100健康、Astro商店/关于正常，商店初始HTML仍30款，近期非空日志无panic/fatal/意外5xx。

下一步需要基于主99备份的100升级恢复演练与对账，协调受控主升级，再接实际页面配置与Admin必要管理、SEO历史导入/hold核对、robots/llms/sitemap分片及Bing/IndexNow验证文件，完成真实新账户与浏览器联合。不能使用现有binary-only refresh脚本绕过新增schema，不能将新库SQL通过等同主数据升级或C01完成。整个C01–C11目标保持未完成。

## 2026-11-01 实际主100升级及配置接入补验

上述99状态为先前阶段。[实际升级报告](local-preview-100-report.json)记录停写后的新鲜99备份/恢复演练owner c85ad7182784，100迁移幂等、291原业务表完全不变、只增加两张空SEO表及213外键无孤儿。唯一主Gin由372更新为25637，配置/门禁/共享3306/Medusa未变；升级后只读verifier确认293非journal表与演练一致、100项校验和及268原表不变，共享86指纹也一致。`schema-upgrade-100`目录保留旧二进制/事实和受保护备份；失败回滚代码存在，本次成功更新未触发，不声称已实测回滚。

Astro head已接公开配置读取；新一轮58页面/16公开页元数据对照通过。但实际检查发现主预览没有STOREFRONT_ORIGIN，已部署Gin配置接口503，页面使用默认SEO回退。该浏览器结果只证明默认行为，不能证明配置生效。原站存在默认本地origin，已在Gin源码补上迁移后localhost:9300默认值，避免为SEO读取启用其他commerce配置。专项真实SQL通过，完整原23组回归及唯一主二进制更新进行中。

新增原有robots.txt、llms.txt、BingSiteAuth.xml和api/seo-artifacts/[key]路由，保留原文、显式开关、key规则和no-store；配置依赖失败返回503且不公开诊断。216项前台测试及check459文件0错误/警告、17提示通过；首次sandbox测试两项本地监听受限，允许回环监听后的完整216通过。实际文件入口仍受上述主接口503影响，待修复更新后再验收。站点地图、Admin配置、真实配置浏览器联合和SEO历史对账仍未完成。

### 默认origin修复已实际运行

[运行联合报告](seo-artifacts-native-report.json)：专项owner8ffabf579819、全新完整23组owner9269d431f503均通过，1813冻结文件、逐组日志SHA和私有MySQL/Redis清理核对完成。Go全包、SEO/schema单元及SEO/API vet通过。`refresh-local-preview.py`扩展到与已审计运行schema一致的99/100，只允许迁移文件完全相同的binary刷新，保留旧binary和事实；100首次刷新从schema_upgrade记录取实际前任源码owner，后续刷新记录自己的schema版本。未放开schema升级或配置变更。

主唯一Gin现PID32874，配置接口200；默认origin为localhost:9300，不设置或启用commerce环境。四个实际入口对照Next通过：robots/llms仅归一化两个本地端口后正文与Content-Type一致，浏览器真实打开两个文本；Bing与不匹配/未启用IndexNow均404，Astro均no-store。未启用GA/GSC、未向provider提交。新一轮58页/16公开元数据对照在配置接口可用时通过，异常0；仍是空设置/空页面默认行为，不能替代真实已配置值联合。

更新后293非journal表与演练完全一致、100校验和/213FK/268原表及共享86指纹通过。旧Admin/后端、新Gin、Astro商店均200，初始HTML30机型；近期SEO请求200、无panic/fatal，cart/checkout预期关闭门禁的503保留。已部署的origin问题解决，前述503阶段保留作历史记录。Admin/配置值浏览器、站点地图与历史SEO/全C01–C11仍未完成。


### 101后台及已配置值联合补验（主预览保持100）

[SEO后台与实际联合](gin-seo-admin.md)、[证据报告](seo-admin-native-report.json)补齐公开15项设置和页面基础写入、独立权限/默认关闭状态/原UUID收据。当前公开DTO新增原始`structured_data_json`以免Astro舍入JSON-LD数字，原对象字段继续保留。专项及完整23组101回归、实际Ant/Gin/MySQL/Redis/Astro的15操作/15收据、四文件/配置head/sitemap和中文手机联合通过；100备份恢复至101的隔离演练通过。主实例尚未升级101，没有自动授予SEO权限或启用编辑。历史数据与其余页面/业务闭环未完成；详见新报告，不以早期空配置默认回退作为本次已配置值证据。


### 主101运行补验

[主101升级及恢复报告](local-preview-101-report.json)记录实际唯一Gin76194、101校验和、214FK零孤儿、295非journal表与停写克隆演练一致；原角色grant/门禁/env保持。已实测克隆100精确逆向恢复和101重放，再执行主升级，主回滚分支未触发。四文件及58页/16元数据严格重跑通过；原Next中文help首轮双JSON-LD观察及三次正常单页探测保留，未放宽断言。主SEO权限/写门/keyring没有自动启用，历史SEO与整个C01–C11仍待。
