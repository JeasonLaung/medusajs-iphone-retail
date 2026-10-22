# iPhone 回收功能设计分析

## 数据快照

- 来源：Swappie sell prices API，国家 `BE`，抓取时间见 `swappie-buyback-prices/manifest.json`。
- 清单：项目现有 30 个 iPhone 机型；每个机型一个 JSON 文件，原始响应完整保留。
- 抓取策略：使用真实 Chrome 顺序访问，机型之间间隔 5 秒；30/30 成功，HTTP 200。
- 共保存 10,720 个报价组合。每个已返回容量通常有 160 条报价（5 种外观成色 × 32 种功能故障组合）。
- 外观成色：`SEALED_BOX`、`ALMOST_NEW`、`LIKE_NEW`、`GOOD`、`MODERATE`。
- 功能故障：`BROKEN`（无法正常运行/通用故障）、`BROKEN_SCREEN`、`BATTERY_ISSUE`、`BROKEN_FRAME`、`BROKEN_BACK_CAMERA`，接口同时提供这些故障的组合报价。
- `purchasable` 本次全部为 `true`；`price.limit_price` 是按机型、容量、外观变化的最低报价，不应写死为全局常数。
- `ALMOST_NEW` 部分记录另有 `fixed_price` 和 `valid_promo_code`，应保留但默认展示 `price.price`，除非业务明确启用促销规则。

## 建议的产品流程

1. **选择型号**：复用现有 `/sell/iphone` 的机型列表；型号来源改为本地报价快照中的可报价型号，并显示“报价更新时间”。
2. **选择容量**：只展示该型号实际返回的容量，不展示请求参数中没有报价的容量。容量解析应支持 GB/TB，内部统一为 GB。
3. **确认可回收资格**：开机、移动网络、账号锁/激活锁、序列号/IMEI 等作为硬性资格检查；无法开机或无法联网直接结束并说明原因。账号锁检查应在提交前再次确认。
4. **填写电池健康**：保留当前问卷的电池百分比和“无法查看”。Swappie 矩阵把电池问题作为 `BATTERY_ISSUE`，因此电池百分比需要映射到一个可配置阈值（例如低于 80%），不要把阈值写死在组件里。
5. **填写功能与外观**：将现有问题答案映射到接口枚举：屏幕裂损→`BROKEN_SCREEN`，边框→`BROKEN_FRAME`，后摄→`BROKEN_BACK_CAMERA`，电池→`BATTERY_ISSUE`，无法正常运行→`BROKEN`。视觉问题使用 5 级成色映射；不要把“有划痕”直接等同于功能故障。
6. **即时估价**：通过 `(model, storage, visual_condition, sorted(functional_condition))` 精确查表。展示 EUR 金额、最低可能调整价、报价有效时间和“到货检测后可能调整”的说明；找不到组合时降级为人工审核，不要自行插值。
7. **提交回收申请**：把问卷答案、映射后的接口枚举、报价快照版本/hash、报价金额、设备身份信息和用户确认时间一并保存。后续检测修改报价时保留原始报价和调整原因。

## 后端数据结构建议

先做一个只读报价适配层，不要让前端直接调用 Swappie：

```ts
type BuybackQuoteKey = {
  model: string
  storageGb: number
  visualCondition: "SEALED_BOX" | "ALMOST_NEW" | "LIKE_NEW" | "GOOD" | "MODERATE"
  functionalConditions: string[] // 排序后作为 key
}

type BuybackQuote = BuybackQuoteKey & {
  amountEur: number
  limitAmountEur: number
  fixedAmountEur?: number
  currency: "EUR"
  source: "swappie"
  fetchedAt: string
  snapshotSha256: string
}
```

建议新增 `buyback_price_snapshot`（批次元数据）和 `buyback_quote`（规范化报价）两张表；原始 JSON 作为不可变附件保留。适配层负责加载最新成功快照、索引报价和校验映射，不负责创建订单。报价申请单应保存快照 ID，避免价格更新后历史单据漂移。

## 与当前代码的衔接

- 当前 `getSellModels()` 从商品目录读取容量，建议改成“报价适配层优先、目录作为无报价时的回退”，这样不会把未返回报价的容量展示给用户。
- 当前 `sell-flow.tsx` 已有型号、容量、电池、功能和外观步骤；可以保留交互，替换 `quotePending` 为服务端报价查询，并在 summary 阶段展示报价。
- 当前流程把“不能开机/不能联网”作为不可回收条件，这与接口的 `BROKEN` 报价存在差异。产品规则应先决定是否允许“故障回收”；若允许，不能把 `BROKEN` 当成全局拒绝，而应映射到对应故障报价。
- 报价查询应在服务端执行并加缓存；前端只收到规范化金额和可展示的成色/故障标签。

## 分阶段实施

### 第一阶段：离线报价能力

导入当前快照，建立规范化 JSON/数据库索引，增加映射单元测试（容量、故障组合排序、缺少报价、最低价校验）。先在回收页面显示报价，不开放寄送和付款。

### 第二阶段：回收申请与人工检测

建立 `buyback_submission`、设备身份校验、照片/检测记录、报价调整状态和审计日志。申请提交后锁定报价快照，后台可接受、拒绝或调整，并记录原因。

### 第三阶段：库存与结算

检测通过后才创建/转移库存设备；检测不通过进入退回流程。结算方式、退款/转账和订单关联单独实现，避免在问卷阶段直接改变库存或支付状态。

## 必须先确认的业务规则

- 你们是否要完全采用 Swappie 的 5 档外观和故障枚举，还是只在内部保存映射、前台使用更少的选项？
- 电池健康低于多少才算 `BATTERY_ISSUE`？“无法查看”是按风险最低价，还是必须人工检测后报价？
- `BROKEN`（不能正常运行）是否允许回收？接口有报价，但当前页面把无法开机/联网设备判为不可回收。
- 报价是直接作为你们对客户的回收价，还是还要扣除检测、维修、物流和毛利后再计算？
- 是否需要按国家/货币保存多套快照；当前抓取仅覆盖 BE/EUR。
