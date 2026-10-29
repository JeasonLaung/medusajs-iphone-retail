# 结账联系信息与地址命令（局部实现）

2026-10-29。服务于本期“整个现有前台闭环”的C06，不代表结账、付款或成单完成。

## 原行为与本次实现

来源 `medusa-store/apps/storefront/src/lib/data/cart.ts:setAddresses`（395—434）：提交联系邮箱及收货/账单地址；同账单选中时前端复制收货地址；成功后进入配送步骤。继续使用原结账表单和样式，下一步接入Astro。本次实现Gin命令及HTTP，尚未挂载原表单或自动跳转配送。

`POST /api/store/carts/:id/contacts` 接受version、email、shipping_address、billing_address。地址字段沿用原命名；必填姓名、街道、城市、邮编和国家，其他字段可省略为空字符串。邮编/电话始终按字符串处理，不截断前导零。国家必须为小写两位码；收货国家须属于购物车地区，账单国家须存在且未删除。拒绝旧地址ID、客户/店铺归属、金额、metadata、重复键（含嵌套）、null、未知字段及尾随JSON。

使用现有购物车Redis凭证、Origin、CSRF、店铺/路径/SQL归属及generation校验。Idempotency-Key为UUIDv4；独立联系信息限流32次/15分钟。响应仅购物车ID、版本和重放标记，不回显邮箱、地址或私有凭证，不延长Cookie；包括超限413在内的响应保持private,no-store。

SaveContacts在同一MySQL事务锁住当前购物车，生成两份独立地址值副本，更新邮箱，然后依次刷新商品展示快照、当前价格、已保存人工固定运费、系统税费和已有/自动优惠，检查精确总额，递增版本并保存不可变摘要收据。最后收据失败、无有效价格、无效国家或其他中途失败都回滚整批写入。无库存预留、通知发送、支付渠道调用或订单创建。

迁移0042增加默认关闭的cart_contact_runtime和不含联系信息原文的cart_contact_operations；不在启动时自动执行DDL或打开能力。重放先重查当前权限和开关，相同请求返回原版本收据，换请求/旧版本/错误generation拒绝。收据SHA-256仅用于一致性，不是加密或匿名化承诺。

## 当前边界

有支付集合关联时拒绝新的地址变更，等待接入持久化支付刷新协调，不通过直接删除支付记录模拟成功。保存了当前服务未注册的配送provider或计算式运费时也拒绝；HTTPOptions.ManualProviderIDs只接收服务端注册，不能由请求选择。完整结账仍关闭，尚缺配送选择/优惠命令、支付刷新接入、付款成单、新客户购物车绑定及原页面联调，不能将本接口当作可付款授权。

## 验证

独立待提交内容快照含41份显式迁移，不含工作区既有0037支付通知改动。Go全包测试通过；`python retail/scripts/test-admin-auth.py --cart-contacts-only` 在独占、无外网/无发布端口、各1CPU（MySQL768MiB、Redis128MiB）的容器中验证：8次并发仅一次保存、重复请求、版本/地区/软删国家/generation拒绝、邮编、当前价格与25%税额、缺价及最终收据失败整体回滚、未注册配送保持原状、不预留库存、开关关闭拒绝重放；真实Gin请求覆盖Origin/CSRF/凭证、嵌套JSON注入、私有响应/413、重放、撤销凭证。测试资源清理完成，证据见cart-contacts-runtime-report.json。

当前证据包含真实Gin/MySQL/Redis，不包含浏览器或真实支付。前台原功能、购物、回收、账户/订单、售后、评价、通知和公共页面继续按current-delivery-scope.md逐条验收。

原开发服务检查：9001/health、9001/app、8000/dk-en/cart返回200；无购物车Cookie访问原checkout返回源码规定的404。原PayPal nonce警告与Next流处理旧日志仍在，未新增服务中断。
