# 仓库设备批量设为可售

2026-11-03，补 C05 原有 `make_available` 的批量语义。来源 `medusa-store/apps/backend/src/api/admin/devices/validators.ts` 允许 1—200 个唯一设备ID，`workflows/device-operations.ts` 要求销售类别、自有来源、pending 状态、无 reserved/packed/shipped/returned 分配历史，并校验产品/SKU身份及配置。原仓库守卫逐台校验当前仓库和分类写权限。

Gin 新增 `POST /api/admin/warehouse/locations/:location_id/device-batches/make-available`，严格 JSON 为 `operation_id` 与 `devices:[{id,expected_revision}]`。同一命令最多200台。所有原版本先在共享设备锁下检查，然后在单一外层事务中复用既有单台命令，保留身份占位、每台事件和单台收据。每次内部命令按事务内推进后的共享版本继续，不错误地把前一台引起的版本递增当成下一台冲突。内部命令与整批最终收据一起提交，任一台及末尾收据失败都完整回滚。没有迁移或启动DDL。

整批收据复用 `warehouse_device_changes`，保存原列表和唯一操作ID；提交后同一命令返回原结果。重放前仍核对当前员工权限与每台当前仓库、销售分类和软删状态，移出范围或撤权不能读取旧结果。相同UUID改变成员/顺序返回409。数量库存与预留保持原样，设备可售并不自动增加库存数量。

Ant 仓库设备表按原 `make_available` 动作投影启用复选框，跨页最多保留200台，换仓/分类/搜索/刷新清空选择。新中英抽屉显示所选SN、状态、数量及整批失败语义；结果未知时冻结同一命令，仅原命令重试，确定失败后关闭并刷新。

## 子流程验收

```powershell
go -C retail/backend test ./internal/devices ./internal/server
go -C retail/backend vet ./internal/devices ./internal/server
node retail/admin/node_modules/typescript/bin/tsc --noEmit -p retail/admin/tsconfig.json
go -C retail/backend build -o ../../logs/retail-native/retail-api-warehouse-batch.exe ./cmd/api
$env:ACCOUNT_TEST_API_BINARY=Join-Path (Get-Location) 'logs/retail-native/retail-api-warehouse-batch.exe'
python retail/scripts/test-local-customer-account.py --scenario=warehouse-batch
```

最终 owner `e59955c7d90e` 在自有数据库/Redis/Gin/Ant/Astro通过，不模拟业务API。实际新商品/SKU/精确价格/库存及设备入库后，实际后台建立仓库员工，用既有设备编辑接口准备两台pending设备。中英390px真实批量确认、首次提交响应丢失后同UUID重试、六并发首次执行唯一、第二台不合格整批回滚、最终批收据SQL故障整批回滚、旧版本/重复ID/修改命令/越仓及撤权重放全部通过；两台最终available，SQL为2份整批收据和4条可售事件。库存API前后字节相同，Astro中英商品仍可购买。

专项 Go 夹具严格限制同一 owned 数据库，在外层最终回滚事务内准备200台：实际执行200台成功、同命令重放、200条唯一事件、第二台不合格整批回滚、末台移出范围拒绝重放与撤权拒绝重放通过。首次扩展夹具的最后移仓断言误用旧表名 `stock_locations`，该轮失败并清理；改为原生 `inventory_locations` 后从头跑完整局部验证，未拼接为通过。

最终独立库、专有进程及6条Redis键已清理；主账户十二表计数和29表商品/设备运营指纹不变。报告见 [机器证据](local-warehouse-batch-report.json)。本机经原启动脚本受控更新，API/后台/商店/商品路由均200；没有重复跑付款到售后的全流程。

本项只闭合仓库原批量可售动作；不关闭 C05 或 C01—C11，正式数据/文件关系及其余清单继续开放。
