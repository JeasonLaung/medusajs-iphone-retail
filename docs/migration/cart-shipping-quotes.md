# 购物车保存上下文与手工运费

2026-10-28：`cart.ReadManualShippingQuotes`在调用方事务内复核持有者、版本、店铺和渠道，解析当前配送分组，再读取购物车保存的配送地址。地区、币种和渠道来自锁内购物车投影；地址仅选国家、省、市和邮编，不读取姓名、电话、街道或客户元数据，也不接受浏览器覆盖规则或价格上下文。

只处理服务器显式注册的manual provider固定运价，沿用已有候选/provider仓库关系/精确价格查询。结果逐组保留行和报价，无法报价的组返回空报价数组，不能只返回有价格的组而暗示整车配送覆盖。没有写入运费、更新购物车版本或预留库存，结果仅为内部阶段数据。

来源为实际安装的`cart/workflows/list-shipping-options-for-cart-with-pricing.js`：地理过滤从`cart.shipping_address`取值；`setShippingOptionsContext`结果才用于选项规则，默认补`is_return`和`enabled_in_store`；固定价格上下文从cart取值。新阶段不会把地址伪装成自定义hook返回值；因此依赖`address.country`等自定义上下文的规则在未注册hook时不匹配。原未提供地址时不施加地理过滤的行为保留，不能将此结果直接作为结账地址验收。原购物车合计/自定义价格规则、翻译及动态provider仍未完整接入。

地址已引用但不存在/删除时拒绝。读取每字段最多4097字符，让候选层按4096字节上限拒绝超长或多字节过大的值，避免把截断值当成有效地址继续匹配。当前目录查询基准使用克隆Session保留共享锁，覆盖事务早期已有RR快照的当前读取需求。

实库人工验证随`test-admin-auth.py`运行，证据项为`admin-auth-report.cart_shipping_quotes`及实现SHA：两组中一组缺价仍保留、精确超SQL小数位报价、含税偏好、地址国家变化、无地址原始行为、未注册provider、自定义规则上下文不伪造、删除/超长地址、过期版本、autocommit及零时间拒绝。只有最新报告包含该项且SHA匹配才构成通过证明。

未开放HTTP。完整provider配置注册、额外价格/规则上下文、按配送仓库的库存提示、跨行共享库存预留、运费保存/替换、促销税费刷新、支付版本失效和正式Astro交互继续待办，T032/T033未验收。

本次来源文件SHA256：`ae956f0b891c4b53d587cad67ff4e0d0bbbc0d1e736b346ebd3650b04e999b6b`。源码检索`medusa-store/apps/backend/src`未发现`setShippingOptionsContext`或`setPricingContext`自定义注册；这不替代插件/provider的完整迁移验收。
