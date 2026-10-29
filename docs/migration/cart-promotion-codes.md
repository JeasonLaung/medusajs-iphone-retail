# 购物车优惠码选择与事务读取

2026-10-29：`cart.PlanPromotionCodes`保留原getPromotionCodesToApply的有序Set语义：默认add；先从商品、再从运费调整项收集现有非空code并去重；add追加已配置请求码；replace先清空旧集合再追加；remove只删除指定码，不要求被删除码存在。大小写、空格保持，不trim或转大写。请求码需要非空UTF8且不超过191字节；输入最多100请求码，合并集合最多100，超限拒绝而非裁掉旧优惠。未知操作明确拒绝，不能走旧流程的无操作分支。

`ReadPromotionCodes`在调用方写事务里重查当前购物车授权/版本，读取有效商品/运费及其活动调整项，按父行和源ID稳定顺序取得code。软删父行、软删调整项、null/空code不形成已有优惠码。链接里存在但没有有效折扣的code不会隐式带入。每类父行/关系均有整体上限，读取不修改调整项、关系、额度或版本。

add/replace对请求码从promotions读取未软删的精确code，检查的是存在性，不提前套active/活动时间资格；原流程也是先确认配置存在，再在ReadActive中筛选有效活动，因此已配置但inactive的请求不会误报为不存在。已有折扣码不要求当前配置仍存在，后续计算会淘汰失效候选，保存阶段会移除旧coded行。remove不查询请求码存在性。自动促销由后续ReadActive.IncludeAutomatic控制，本函数不负责阻止移除后自动促销再次加入。

## 证据

`capture-cart-promotion-codes.cjs --check`直接执行已安装原步骤回调，以人工graph响应替代查询，只捕获选择结果和失败状态，不启动应用。12组默认add、有序去重、replace清空、remove未知、已有失效码保留、未知请求拒绝、大小写、空格对照存入fixtures/cart-promotion-codes.json并记录源SHA，Go逐例比较。

隔离MySQL checkCartPromotionAdjustments同时验证链接但无折扣不形成已有code、inactive配置存在性、未知/不同大小写请求拒绝、remove未知不报错、保存实际折扣后读取单一SAVE（商品/运费重复合并）、add顺序、replace/remove结果，再沿用原保存/回滚测试。该实库证据由相应测试源码SHA及cart_promotion_adjustments范围承载，完整报告/专项报告需源码SHA匹配。

仍未把完整商品关系/订单/客户可信上下文、候选计算、保存、版本/幂等/支付刷新组成公开购物车命令；完整checkout与Astro接入仍待完成。T032/T033未验收。
