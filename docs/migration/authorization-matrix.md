# 身份、操作与数据范围矩阵

日期：2026-10-27。T004 进行中。下列为已追踪的旧行为及新系统必须维持的边界；不表示完整原生 RBAC 审计已结束。新员工与客户重新建立身份，旧个人角色绑定不导入。

## 证据与授权层级

以 `medusa-store/apps/backend/src/` 为根：`api/middlewares.ts`、`api/device-access.ts`、`api/warehouse-guards.ts`、`lib/warehouse-access.ts`、`api/mobile-intake/access.ts`、`lib/mobile-intake-token.ts`、`api/registration-guards.ts`、`workflows/product-reviews.ts`、`api/store/product-reviews/route.ts`、`api/store/orders/[id]/after-sales/route.ts`、`api/store/buyback/requests/[id]/photos/[photo_id]/route.ts`、`api/admin/mail/messages/[id]/attachments/[attachment_id]/route.ts`。

旧入口的 authenticate、原生策略、项目中间件、工作流内对象检查共同构成授权。`/admin/*` 中间件中 allowUnauthenticated/allowUnregistered 不等于公开后台；后续框架鉴权仍生效。新 Gin 路由默认拒绝，不能移植该宽松参数组合后遗漏后续保护。

授权分四层：身份类型 → 资源动作 → 仓库/来源/对象归属 → 响应字段。菜单隐藏不代替任何服务端检查；详情、导出、附件和后台任务调用相同领域授权入口。

## 身份与范围

| 身份 | 旧系统识别/数据范围 | 新系统承接 |
| --- | --- | --- |
| 普通后台员工 | 原生 RBAC + 项目领域校验；不等于任意员工均有全权限 | 新账号及显式权限；平台管理员初始化与普通角色分开 |
| 供应商 | user.metadata.device_role=supplier，必须指向有效 supplier source；order_ids 为明确授权订单 | 独立持久化来源与订单授权；无来源时拒绝，不能退为管理员 |
| 仓库人员 | 固定仓库角色，或 device_role=warehouse，或 warehouse_location_ids；all 表示全仓，否则为明确仓库集合，空集合无仓库 | 显式范围模型；全仓权限与空集合严格区分；动作仍需授权 |
| 客户 | customer 身份；订单/回收按 customer_id 归属 | 新注册客户 ID；历史主体不能因邮箱相同获得归属 |
| 移动入库持链人 | 两小时签名令牌，包含 actor/source/category；每次重新检查当前员工与来源权限 | 短期受限凭证；撤权即时有效，不给通用后台权限；不沿用旧签名密钥/旧登录身份 |
| 集成密钥 | 旧系统允许 secret API key 进入部分 admin 领域；deviceAccess 将有效 secret key 视为 admin，仓库中间件跳过 key | 旧密钥不迁移；新集成凭证必须独立动作权限和范围，禁止默认超级管理员 |
| 匿名访客 | 公开目录、估价、已审核评价、非私有内容 | 只读公开 DTO；购物车匿名能力单独校验 possession，URL ID 本身不足以访问私有记录 |

## 查询、命令及字段矩阵

“有权员工”仍需资源动作授权；供应商/仓库列的范围是额外限制。22 个设备动作另见 [设备命令表](device-commands.md)，不能用下表摘要替代。

| 能力 | 有权员工 | 仓库 | 供应商 | 客户 / 访客 | 字段与对象检查 |
| --- | --- | --- | --- | --- | --- |
| 商品定义/SKU 查询 | 按商品 read | 允许共享定义，但嵌套库存裁到本仓；限制任意关系展开 | 仅设备 variants 专用入口，通用商品 API 拒绝 | 公开渠道且 published；评价先检查商品可见性 | 不透传 raw payload/任意 fields；不暴露采购、内部 metadata、其他仓库 |
| 商品/分类/价格修改 | 对应 write | 拒绝 | 拒绝 | 拒绝 | 服务端动作授权，不能仅凭可读共享商品获得写权限 |
| 数量库存查询 | 对应 read | 自己仓库层级，数量按 required_quantity 汇总 | 通用库存 API 拒绝 | 仅可售汇总 | stocked/reserved 与设备档案独立；嵌套返回同样裁剪 |
| 订单列表/详情 | order read | 仅含本仓未取消履约的订单，且需 order read | 通用订单 API 拒绝；设备工作台只给被授权订单信息 | 本人订单 | 仓库仅本仓包裹和对应行数量；不泄漏其他仓销售/支付明细 |
| 订单取消、编辑、退款 | 资源 write + 设备业务 guard | 通用写入口拒绝 | 拒绝 | 不开放后台能力 | 设备分配/履约/退回约束不能从原生入口绕过 |
| 配货/发货/解配 | 见设备动作表 | 仅已有本仓履约、对应行与设备；禁止跨仓挪包裹 | 自来源+授权订单；整组 assignments 都要匹配 | 拒绝 | 逐对象检查，不能只检查数组第一项 |
| 设备入库/照片 | 见设备动作表 | 自营来源、本仓、分类动作授权；recycled 拒绝 | 通用入库/照片拒绝 | 仅有效移动凭证的受限操作 | 令牌类别、来源、当前员工权限重新验证 |
| 售后列表 | 对应 read | 先检查订单范围，只保留本仓包裹对应 item 的请求 | 拒绝 | 本人订单的本人请求 | 请求行裁剪后为空则不返回；客户越权按 not-found 处理 |
| 回收请求/照片 | 对应 read/write | 路由白名单未允许，拒绝 | 拒绝 | 只读写本人请求 | 私有照片 private,no-store；每次验证请求归属和 photo 属于请求 |
| 公开评价 | read / moderate 分开 | 通用评价后台拒绝 | 拒绝 | 只展示 approved；提交需客户登录 | 公开仅作者名、星级、标题内容、时间与已购标记，不返回邮箱/customer_id |
| 提交评价 | 不替客户冒名 | 拒绝 | 拒绝 | 同 customer/product 一条；有 order_id 必须本人符合资格订单且含该商品 | 非订单评价也可提交但不标 verified_purchase；新系统需并发唯一约束 |
| 客户通知 | 不从 store 入口读他人 | bootstrap 可返回空通知 | bootstrap 返回空通知 | 本人通知列表和已读 | 已读 ID 也检查归属，禁止跨用户批量更新 |
| 邮件/附件/活动 | 相应运营权限 | 拒绝 | 拒绝 | 拒绝 | 本地附件作为下载；远端附件可能返回签名 URL，禁止公共缓存/日志记录完整 URL |
| SEO/支付/邮箱/授权设置 | 相应配置权限 | 拒绝 | 拒绝 | 仅指定公开设置/支付 browser config | 密钥永不回显；不得复用管理员 raw DTO 作为公开配置 |
| Analytics | 对应统计权限 | 白名单未开放，拒绝 | 拒绝 | 拒绝 | 历史主体与新账号分开；不要因能读商品就能读全店收入 |
| 自己员工姓名 | 按原生用户权限 | 通用写入口未开放 | 仅自己的 first_name/last_name | 拒绝 | 禁止顺带修改 metadata、角色、来源和订单授权 |

## 仓库响应裁剪不能简化成 WHERE location_id

旧 `scopeWarehouseOrder` 对非全仓范围：仅本仓履约；按未取消履约汇总行数量；对应金额按数量比例裁剪；付款集合只保留 status；移除 credit_lines、returns、claims、exchanges、summary、metadata、promotions、shipping_methods；客户只保留业务联系字段。全店订单合计不可直接暴露。

旧代码用 Number 比例运算，Gin 需使用精确金额并定义尾差归属；这属于保留范围语义、修复金额实现，不能把全单总额或其他仓数据作为“兼容”返回。全仓范围分支直接返回原订单，需在新契约中明确该权限的含义，并验证未额外授予支付操作。

共享商品的关系查询经过白名单，库存嵌套同样裁剪；禁止客户端用 `fields=*`、任意深层关联或导出接口绕开范围。

## 注册与文件凭证

供应商账号的创建/编辑分支、[] 无订单授权、自绑定拒绝及失败清理证据见 [账号与绑定契约](supplier-account-contract.md)。作用域绑定不授予通用订单 API；旧登录身份和旧个人授权不导入，新系统显式重建。

- 客户验证邮件只能发往当前 emailpass 身份的邮箱；entity_type 必须 email；重复请求间隔至少 60 秒。客户资料创建前必须邮箱已验证且 body.email 相同。
- 入库令牌使用 purpose、actor_id、source_id、category、expires_at、nonce；长度上限 2048，签名恒定时间比较，默认两小时；旧 nonce 并非一次性消费账本，不应误写成旧流程每链接只用一次。
- 私有回收照片先查归属再读文件。邮件附件接口目前依赖后台鉴权，不存在客户开放下载能力。公开产品图片、设备照片和私有回收/邮件附件的路径、存储与访问规则分别核对，不能一律静态开放。

## 负面用例（待新系统运行）

1. 空仓库范围不能读全部仓库；缺来源供应商不能成为管理员。
2. 仓库读取混合仓订单、售后、嵌套商品库存与导出，只得到本仓范围；更改查询 fields 不能扩大。
3. 供应商提交混合来源/未授权订单 assignments 整批拒绝；replacement 及通用订单/客户/设置 API 拒绝。
4. 修改自己的姓名时夹带 metadata、role、source_id 拒绝；修改其他员工拒绝。
5. 客户跨订单/回收/照片/通知/评价已购证明访问拒绝；同邮箱新账号看不到历史记录。
6. 移动令牌过期、变更类别/来源、员工停用/撤权后拒绝；不能用于其他后台 API。
7. 普通可读角色调用后台修改、退款、配置、导出或附件下载，不能通过隐藏菜单或 URL 直达绕过。
8. 未审核评价不出现在公开统计；重复并发评价仅一条成功；公开 DTO 不含身份字段。

待补：完整原生 RBAC 资源/策略、SEO 各命令、邮件各命令、公开设备照片访问边界、导出和 API key 的新最小权限契约。此表不能据此将 T004 标记已验收。
