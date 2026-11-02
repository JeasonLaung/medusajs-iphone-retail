# 仓库员工设备编辑

2026-11-03，接通 C05 中原系统已有的 update 操作。来源为 `medusa-store/apps/backend/src/workflows/device-operations.ts` update 分支、`src/api/admin/devices/validators.ts` UpdateDevice，以及 `src/api/warehouse-guards.ts` 和 `src/lib/warehouse-access.ts` 的当前/目标类别、仓库及自有来源限制。

## 行为

私有 GET/POST `/api/admin/warehouse/locations/:location_id/devices/:category/:device_id/edit` 仅允许当前仓库员工、sale/returned分类和明确读写权限。编辑器包含原SKU、来源、当前仓库、IMEI/IMEI2/EID、类别、状态、成色、电池、成本（分）/币种、库位、备注、照片。设备ID、序列号和审计身份不在可编辑字段中。列表仍不投影成本/备注，编辑上下文须写权限；来源目录仅当前范围内自有来源，SKU只读定义使用现有仓库搜索接口，不授予全店商品或订单权限。

事务锁当前权限及仓库范围，取得统一设备身份锁；重新核验原路径、当前对象、目标仓库/类别及来源仓库，所有目标来源必须self。预配/装包/发货活跃分配下SKU/来源/仓库/类别/状态不得改变，其他原允许字段仍可改。不能借编辑进入 reserved/packed/shipped/repair_ready，available/pending必须sale；这些状态原样保留时沿原规则允许。SKU使用原单台库存关联和实际成色/电池规则，号码规范化并跨IMEI/IMEI2/EID查重。

更新设备、删除其旧号码占用、认领新号码、事件和唯一收据在同一事务；最后收据写入失败也回滚。序列号及其他设备占用不变，同字段SQL唯一冲突与跨字段占用都拒绝。修订包含完整编辑字段、照片、设备更新时间、活跃分配及全局设备版本。未知网络结果保留原UUID/正文重试；变化请求或旧修订拒绝。允许授权内移仓后同请求恢复，但每次恢复也重查旧路径/当前位置/目标范围；撤权或移出原目标后不能利用收据绕过。

数量库存、预留、订单和付款不由编辑命令改变。没有新迁移或自动授权。

## 局部验收

`go -C retail/backend build -o ../../logs/retail-native/retail-api-device-edit.exe ./cmd/api`，PowerShell设置 `ACCOUNT_TEST_API_BINARY` 为该自有logs绝对路径，再运行 `python retail/scripts/test-local-customer-account.py --scenario=warehouse-edit`。运行器只恢复自有SQL副本，独立19100/19200/19300服务及Redis，全部清理后记录结果，不重新走付款链。

最终owner `fb795a7a2539`。实际管理员创建商品/ERP SKU/精确价/1件数量库存、自有来源自动建仓、照片及销售/退回两台设备；创建只授权本仓分类操作的新员工。手机真实Ant页面修改销售设备的IMEI/IMEI2、12345分成本、库位和备注，保留SN及原照片，成功后丢响应并原命令恢复；中英文刷新读取一致。六并发首次保存仅一个非重放，异正文、旧版本、跨仓、跨字段占用及直接编辑到发货拒绝。故障触发器只对特定备注在最终收据写入失败，设备完整上下文/修订保持不变。撤销销售写权限后旧会话401，重新登录旧收据403且编辑按钮消失。完整SKU数量库存/预留API响应保持相同，Astro中英文手机商品页仍可购买。

附加SQL子流程只在外层回滚事务中准备活跃预配关系与第二授权仓库：验证已分配结构字段禁止、备注/成本仍可保存，未分配后可授权内移仓，原路径不可读取移动后的对象，同次移动收据可恢复，目标范围撤销后恢复拒绝；供应商来源不出现在可编辑目录且POST拒绝。新增夹具及命令最终全部回滚。该SQL夹具不代表本次重新购买或真实业务调拨。

最终SQL修改收据2/事件2，身份占用3（SN及两IMEI），旧IMEI占用0；主库29项商品/库存/设备表指纹及12项账户/回收/售后计数保持不变。报告与最终owner见 [local-warehouse-device-edit-report.json](local-warehouse-device-edit-report.json)。Go设备/身份/路由测试、相关vet、Admin类型检查与脚本语法通过。首次补充供应商夹具违反同仓唯一来源约束，失败日志保留，改为独立来源仓库后重新验证，不改业务约束。

本次不关闭C05：批量设备操作、历史照片/正式数据组合及剩余C01—C11交付继续开放。

当前服务已由现有start.bat对应脚本受控更新，native API健康、仓库用户页、商店/商品页均200，主只读后台检查通过，仓库专有编辑路由拒绝全店主体403。主设备0/来源2/仓库员工0/业务邮件与投递0，近期日志正常；旧Medusa9001/8000按此前切换保持停止。退回检测场景另以owner `7d5b5d9ee913` 完成必要回归，含照片/原机维修/撤权/库存保持不变与自有资源清理。中文手机视觉核对后把当前SKU的内部ID显示改为原名称。
