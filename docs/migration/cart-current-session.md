# 当前购物车恢复

2026-10-28。新增 `GET /api/store/cart`，ID完全来自服务器解析的HttpOnly possession。复用详情处理的配置店铺/Origin检查、Redis绑定、`Inspect`数据库当前归属、有效代数/未完成/无客户绑定/渠道与版本核验，返回同一精简快照及items/checkout=false。没有增加可读ID cookie、URL标识、localStorage或sessionStorage。

要求私有no-store响应及请求编号，不接受任何查询参数，不接受scope cookie代替持有凭证；缺失/重复/撤销持有凭证、跨Origin和跨站请求按原私有接口拒绝。读取不发行cookie、不延长TTL、不创建购物车，也不更新数据库。依赖未配置或Redis故障仍503，不能当作空车。

Astro `CartClient.current()` 复用原详情DTO校验和私有传输，仅明确401/404返回null；其余故障抛出。页面可在刷新后调用current，再按其ID读取保存金额；该恢复查找与购买命令分开，不以GET偷偷创建车。旧按ID读取接口保留原行为。

## 验证范围

数据库专项增加当前查找与原详情相同的精简响应、无Set-Cookie、无/错误/重复凭证、未知查询、跨Origin/跨站、SQL撤销、Redis断开及未配置503。Astro新增无ID私有读取、规范版本/私有字段/未知能力拒绝、401/404与503/断网区分，单元总数33。

联合真实浏览器新增进入前current为空、加购后重新加载模块仅从current查找version3车并读取精确合计、移除possession后为空；既有cookie绝对过期与浏览器存储断言不变。SQL回查仍核对两车、一个version3车、单行数量2、两条操作回执及raw精度/未预留。成功与否以最新admin-live-report.json的current_possession_lookup和源码SHA为准。

此处是当前持有凭证的恢复，不是丢失凭证后的身份找回；旧账号或知道购物车ID仍不能认领。还没有完成原商品操作区的建车/命令协调、不确定操作跨刷新保留、地区变更、客户合并及生产页面挂载。支付与完整规则仍待实现；相关完整任务未验收。

本批专项真实数据库测试和3项联合真实浏览器已通过，current_possession_lookup已记录true；Astro类型/build及33项单元、40对展示和24对操作对照继续通过。浏览器资源与临时二进制已清理，原页面没有新增可见交互或样式变更。

最终Go全量与vet、购物车专项及完整身份/业务数据库回归通过；五份报告对记录的156/100/8/152/170项源码SHA全部匹配。自有容器/网络清理完成，旧9001 health/app和8000商品/购物车完整响应均200，既有错误日志未新增。
