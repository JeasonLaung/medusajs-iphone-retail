# 手工配送方法保存阶段

2026-10-29：`ReplaceManualShippingMethods`参与调用方事务，接受选项ID和manual provider方法数据，重查当前持有者/版本、全部配送分组、运价及整车共享库存后才写入。manual注册名单必须来自服务器；没有HTTP入口或独立提交，不能拿内部阶段代替完整配送命令。

行为参考实际安装`cart/workflows/add-shipping-method-to-cart.js`及其add/remove steps：从服务器报价取名称、金额与含税状态，数据来自manual方法输入（未提供默认为`{}`），同incoming profile旧方法和无可识别profile旧方法软删除，其他profile保留。原workflow支持局部incoming；本协调器当前要求全部当前配送profile有选择，部分更新的原接口语义仍须在完整命令中实现。数据必须为UTF-8 JSON对象且最多64KiB，保留原JSON文本，不把大整数转成浮点；不复制选项私有data或接受浏览器金额。

金额通过既有sqlAmount保存精确raw及截断到18位的兼容标量。全部输入和价格先验证再写入，选项按profile稳定顺序写入新方法。活动方法总数最多100，与税行刷新上限一致。旧方法软删除而非硬删，其调整与税行历史留在原方法下；有效金额投影按活动父方法读取，随后完整刷新负责重建应生效税费和促销。

内部方法不更新购物车版本、不生成持久幂等回执、不使支付有效性发生转换，也不预留库存。调用者必须在任何错误时回滚整个事务，并完成价格/税费/促销/配送刷新、支付失效、回执与版本更新后才提交。直接反复调用该阶段会生成新方法ID，不代表可公开重试的幂等命令。

实库人工验证进入`admin-auth-report.cart_shipping_methods`，与实现SHA一起核对：超SQL小数位raw精确、含税偏好、manual数据前导零和大整数、同profile/未知profile替换、无关profile保留、旧关联历史保留、插入第二条时故障与后续刷新故障完整回滚、无效JSON/过期版本不变、阶段不增版本和autocommit拒绝。只有最新报告实际包含该项且SHA匹配才构成通过证明。

仍待完整幂等配送命令/专用能力、局部选择协调、provider配置、动态/总额运价、完整促销税费刷新、支付失效及Astro真实选择。T032/T033和checkout未验收。

来源模型`@medusajs/cart/dist/models/shipping-method.js`未为tax_lines/adjustments声明删除cascade；本阶段仅软删父方法，保留关联历史，活动父方法筛选用于合计读取。完整税费/促销刷新另行负责重建当前结果。
