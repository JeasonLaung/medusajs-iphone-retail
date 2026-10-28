# 手工配送固定运价

2026-10-28：`ReadManualFlatQuotes`在同一调用方事务内组合ReadCandidates、有效配送分组、provider启用、显式服务器manual provider注册名单、仓库/provider有效关联及ReadFlatPrices。结果按选项ID稳定排列，保留全部有效发货仓库。候选的provider data仅内部使用，不新增HTTP、不透传私有数据。

manual注册名单必须来自可信服务器配置，不能按ID前缀猜测能力，也不从浏览器接受；当前尚未接应用配置/完整购物车协调器，调用方需用当前商品解析配送分组。分组ID必须全部存在且未删除，超限/重复拒绝；不匹配选项排除。禁用、缺失、未注册provider以及未关联仓库不提供报价。仓库/provider校验比原列表workflow更严格，避免把目录关系误当成可执行配送配置；完整provider注册与第三方实现仍待完成。

仅承接manual的flat路径，calculated不会用固定价冒充；原手工provider的canCalculate=false、calculatePrice明确抛错。`capture-manual-shipping.cjs --check`调用实际安装服务的3组人工数据验证、选项目录与计算能力，不访问数据库/网络、不创建发货；源码SHA和输入输出固定在[manual-shipping.json](fixtures/manual-shipping.json)。其原validateFulfillmentData原样返回输入，保存方法数据的事务接口仍待实现。

价格上下文必须与渠道一致，购物车配送不接受行项目数量，沿用原缺省数量分支。精确地区/币种/含税与共享price set复用已有引擎，零金额合法，缺价不产生报价。返回不是结账授权，未覆盖额外总额选价上下文。

人工MySQL验证纳入admin-auth-report.manual_shipping_quotes：停用/未注册provider、仓库关联缺失或软删除、有效配送分组/其他分组/空分组/缺失和删除分组、精确超18位运价、地区币种差异、含税、零价、动态计算排除、渠道不一致、数量注入及autocommit拒绝。具体通过范围与最新实现SHA一起核对；Go单元的跳过实库部分不构成上述证据。

仍待：从当前购物车商品解析完整profile覆盖、provider注册配置/适配器、按配送仓库的库存提示和最终共享库存预留、总额条件运价、动态provider、配送保存/替换/重算、支付版本失效及前台实际选择。没有创建/扣减/预留库存、没有调用真实物流服务，完整配送和checkout未验收。
