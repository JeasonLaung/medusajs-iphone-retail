# Gin 店铺语言目录与 Astro 读取

2026-10-28。T020/T025 的局部实现；完整翻译配置、生产页头数据组装和购物车换区仍待完成。

`GET /api/store/locales` 只查询服务器 `STOREFRONT_STORE_ID` 绑定的有效店铺及其未删除 `commerce_store_locales`，明确返回 `{locales:[{code,name}],count}`。不接受浏览器店铺筛选、fields 或其他查询，不读身份 cookie，不签发会话。成功使用公共重验证缓存，失败 no-store；不返回关联 ID、时间、店铺 metadata 或私有字段。使用与目录模型一致的只读 RR 事务，按配置行 ID 稳定排序，最多100项；LONGTEXT在SQL内先截断并单独读取原始字节长度，超长数据不会无界进入Go。

原 Medusa `/store/locales` 从 `store.supported_locales.locale.name` 返回名称，受 translation feature flag 控制。当前已有业务快照只有 `store_locale` 关联表且零行，没有可迁移的自定义名称目录；原站在未配置时由页头添加 English/简体中文后备项。Gin 空目录返回200空数组，不能用依赖故障模拟未配置。

本批明确承接安装版本翻译模块的51个默认名称，保留配置的原 code 大小写和完整 name，按不区分大小写的 code 精确匹配。重复配置、坏语言标签、超过35字节、超过100项或找不到默认名称整体503；不静默丢弃或生成猜测名称。**自定义语言名称的持久目录、编辑、导入以及修改默认显示名仍待实现**，不能据此声明完整翻译模块完成。既有原 en/zh 后备仍仅在页头展示层添加，不伪装为店铺已配置语言。

`loadLocales` 使用既有匿名/不缓存/禁止重定向/5秒/128KiB 公共传输，校验数组、count、标签、名称和重复项，仅投影 code/name。404、503、坏数据和缺少源配置保持错误。真实隔离 Gin 读取空配置与人工 MySQL 的非空名称、外店铺、软删除、重复、超长、无店铺/已删店铺及未知名称拒绝分别有证据；不代表实际页面、locale cookie 或购物车更新完成。

`node retail/scripts/capture-store-locales.cjs --check` 核对原默认名称和原GET纯投影，3组人工输入，源文件SHA见 `fixtures/legacy-store-locales.json`。不启动旧服务、translation loader、数据库或外部API。新Go嵌入的 `default_locales.json` 与原51项逐项一致；该来源记录不声称采集了原真实语言配置。

数据库证据见 `admin-auth-report.json.store_locales`，真实Astro源适配器到Gin的证据见 `astro-runtime-report.json.runtime.source_adapter_real_gin_locales`。原开发服务、数据和登录材料不变；无需新增SQL迁移，继续使用已有27个正式迁移。

最终回归通过：完整后台隔离MySQL/Redis与购物车事务专项、61项Astro单元/类型/build，真实Gin语言适配器读取，40对商品/24对操作区/84对购物车/18对页脚/34对页头及真实浏览器恢复链路。原51名称/3组GET投影再次核对通过，审计零漏洞。五份报告160/103/22/268/189项源码SHA全部匹配。自有测试容器、网络和临时二进制已清理；旧health/app、商品/购物车与9300预览完整响应200，既有错误日志未新增。上述证据不等于自定义语言配置、生产页头组装或换区事务已完成。
