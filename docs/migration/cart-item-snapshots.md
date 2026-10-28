# 刷新时的商品快照准备

2026-10-28，T032/T033中间实现，完整重算未验收。

`cart.RefreshItemSnapshots` 在调用者事务内重新核对当前店铺、持有代次、购物车和版本。锁定活动商品行后，以保存的变体ID和服务器渠道读取当前可购买目录。自定义价格行也检查变体存在、公开和配置完整；无变体的手工行保留其历史描述。最多1000活动行，批量查询变体。

原逻辑依据安装的 core-flows/cart/utils/prepare-line-item-data.js。已有 title、可空描述、图片、商品类型、SKU、条码、选项、shipping/discount 标志按原快照保留；显式空字符串不当成缺失。可空字段仅在SQL null时以当前目录补全。product_id 跟随当前变体商品；is_giftcard 取当前商品标志，无变体时为false。metadata 的 SQL null 和 JSON null 归一为空对象，其他JSON内容保留。无变体行不因历史 product_id 自动解析新的商品归属，更不授予账号权限。unit/raw/compare-at、数量、选项快照、库存及税优惠关系不由本阶段修改。已删除行不更新。

`capture-cart-snapshots.cjs` 使用人工输入实际执行原纯准备函数，固定来源SHA和5组输出：null补全、已保存字段、手工行、零划线价及sale回退。不启动原应用或workflow，不访问业务库或provider。证据在 fixtures/cart-item-snapshots.json；它记录原准备函数行为，不能替代Gin数据库验证。来源字段映射还由隔离实库独立验证。

实库专项在 identity/cart_item_snapshots_test.go，由现有价格刷新专项调用，共用新建的隔离人工库。覆盖null补全、显式空值、历史文案、当前商品关联/礼品卡、手工默认、metadata两种null、精确价格及选项保留、删除行保留、后续失败及第二行更新失败的整体回滚、下架及旧版本拒绝。实际通过状态以 admin-auth-report.json.cart_item_snapshots 及源码SHA为准。

```text
node retail/scripts/capture-cart-snapshots.cjs --check
python retail/scripts/test-admin-auth.py
cd retail/backend
go test ./...
go vet ./internal/cart
```

调用者必须在任何错误后回滚整个事务。本组件没有提交、回执、版本递增或独立HTTP能力。最终重算编排需在同一事务完成地址/地区处理、快照准备、选价、税行、促销/配送及版本/幂等回执；具体排序和原自定义hook/翻译仍需整体实现与验收。没有开放结账，没有修改原前台样式。
