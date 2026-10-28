# 购物车税行的事务参与组件

2026-10-28，T032/T033 中间实现，完整重算及结账未验收。

`cart.RefreshSystemTaxes` 在调用者拥有的 MySQL 事务内再次执行 `LockForWrite`，核对店铺、当前持有代次、活动购物车和所见版本。从保存的地区、配送地址、商品行及配送方法组装输入，再使用已经迁移的 `tax.ReadSystem`。浏览器不能提交税率、provider、国家或商品归属作为可信计算事实。购物车事实使用当前锁定读取，避免调用者先前建立的 RR 快照隐藏已提交地址变化；税务配置仍使用调用者事务的配置快照。

核对的原实现包括 core-flows 的 update-tax-lines、get-item-tax-lines、set-tax-lines-for-items 和 cart 模块的 setLineItemTaxLines_/setShippingMethodTaxLines_。保留整车替换语义：当前税行软删除，新税行使用新 ID；空结果也清理旧行，礼品卡及已经移除的父行不留下活动税行。限定本车，其他购物车的税行保持原样。名称写入 description，rate_id 写入 tax_rate_id，输出 provider_id 为 system；内置配置 ID 仍为 tp_system。精确税率通过字符串参数保存到 DECIMAL(38,18)，税码按191个 Unicode 字符的 SQL 边界核对，不截断多字节内容。

地区关闭自动税且未强制、普通计算缺国家、没有国家税区时，沿用税务组件的空结果语义。强制计算缺国家、停用或未知 provider、税区歧义及依赖错误拒绝；错误时调用者必须回滚整个事务，不能保留已删除或部分插入的税行。

该组件不提交事务、不增加购物车版本、不保存命令回执，不开放 HTTP 或新的启用能力。最终编排必须完成 Origin/CSRF、持久幂等、版本递增、全部价格/促销/配送协调后才提交；任一步失败整体回滚。不能把单独调用本组件作为对用户承诺的重算成功。第三方 provider、原翻译 feature/hook 和自定义税务上下文仍需后续承接。没有修改税区配置，也不把当前税率覆盖到旧成交订单。

隔离 MySQL 专项位于 identity/cart_tax_refresh_test.go，通过正式29个迁移创建人工库和新的匿名购物车。覆盖商品/运费组合税、18位小数、多字节税码、替换清理、无关车保护、后续领域失败及后置运费插入失败整体回滚、旧代次/版本拒绝、无效 provider 保留原税行、自动/强制/地址门槛，以及先建立旧 RR 快照后发生地址变更的读取。实际结果以 admin-auth-report.json 的 cart_tax_refresh 和源码摘要为准；未配置隔离环境的普通 Go 测试会跳过该实库用例。

```text
python retail/scripts/test-admin-auth.py
cd retail/backend
go test ./...
go vet ./internal/cart
```

完整购物车换区命令、促销/配送重算、页面接入和完整交易仍未完成。本批不改变前台 DOM/CSS，也不替代历史浏览器页面对照证据。
