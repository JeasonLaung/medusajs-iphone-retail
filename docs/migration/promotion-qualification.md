# 促销资格与目标范围

2026-10-29：`promotion.QualifyTargets`接收内部配置与服务端可信上下文，输出订单条件/币种匹配结果和普通商品、运费、购买、赠品的源ID集合。`Applicable`只表示订单规则和币种通过，不代表有合格目标，更不代表可用预算、库存或结账成功。无应用方式返回空集合；配置和上下文非法时无部分结果。该入口不提供HTTP、不读写数据库、不消费额度。

普通商品保留不可打折、缺subtotal、非正subtotal、缺quantity的过滤，quantity具体合法性仍由分摊器验证。订单目标复用同一商品规则。运费按shipping_methods作用域筛选，不继承商品的is_discountable或正价过滤；金额阶段仍需过滤。买赠分别判断buy/target规则，不继承普通商品的打折标志；缺购买规则或非正最低购买件数不产生范围。集合按输入顺序返回，价格优先和数量保留交给已有买赠分摊器。原来的truthy行为保留，包括字符串“false”不等于布尔false。

`SortCandidates`复制切片后稳定排序：买赠优先，同类型精确value降序，买赠同值时最低购买件数、赠送件数依次降序，同键保留原顺序。无方法放末尾避免旧比较器解引用空方法；超浮点整数仍按精确数值排序。源数据库value是数值，本阶段没有把字符串字典序当成金额比较。

## 验证

`node retail/scripts/capture-promotion-qualification.cjs --check`直接载入已安装原Medusa纯规则、商品筛选、买赠筛选和排序实现。脚本仅在独立Node模块内临时导出原私有函数，不修改node_modules，不启动旧应用。23组资格用例与4组排序用例及原文件SHA保存在`fixtures/promotion-qualification.json`。`go test ./internal/promotion`逐例对照，并补充非法上下文、重复ID、无方法与大整数排序验证。原分支不统一过滤规则，迁移也不能统一。

完整隔离MySQL套件中的`verifyPromotionActiveRead`另验证从数据库三类关联读出的00123规则能接入资格判断，且不与123混同。该SQL证据须同时检查admin-auth-report中的相关源码SHA。纯资格对照以fixture和Go测试为准，不把MySQL报告当成完整优惠引擎证据。

尚未把真实购物车事实组装、共享折扣图、预算预览反馈、旧调整项移除/新调整项保存、版本与幂等命令连接起来。预算和用量检查必须在最终编排执行，不能直接根据Applicable保存优惠。T032/T033仍未验收。
