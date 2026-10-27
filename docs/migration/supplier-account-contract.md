# 供应商账号创建与授权绑定

T004/T007/T015/T048 的输入和状态证据。源码为 `src/api/admin/devices/supplier-accounts/route.ts`、`src/workflows/device-supplier-account.ts` 和 `src/api/device-access.ts`，路径相对旧 backend。Gin 尚未实现，旧登录账号及授权不迁移；以下流程用于新建员工账号与显式重新授权，不是按邮箱认领历史业务。

## 输入与分支

旧 POST 的 SupplierAccountBody 是路由内联导出：邮箱 trim/email/max254，source_id trim/1—100，order_ids 最多200项、每项 trim/1—100，password 可省略、传入时12—128字符，strict 拒绝未知字段。密码原文保留空格，邮箱不自动转小写。输入允许重复订单，持久化前去重。

| 条件 | 旧实际行为 | Gin 承接边界 |
| --- | --- | --- |
| 未传 password | 查现有员工邮箱，修改供应商来源和订单绑定，200 success | 只能显式选择新系统员工并重新授权；确认目标身份，事务更新作用域/权限版本并审计 |
| 有效 password | 创建供应商登录与员工，201 success | 创建新员工身份、密码散列与受限供应商授权；在可登录前完成限制；不附带客户身份或历史账号权限 |
| password 空字符串 | 输入验证拒绝 | Ant 表单“编辑绑定”应省略该字段；不能悄悄创建账号或清空密码 |
| order_ids=[] | 空授权集合 | 不授予所有订单；来源和设备命令仍逐次校验 |
| 同一订单重复 | 输入接受，工作流 Set 去重 | 规范化为唯一集合，验证全部订单，不因数组长度/顺序绕过授权 |

创建及编辑均先检查操作者为旧 deviceAccess 的 admin，当前 warehouse/supplier 由中间件和动作范围进一步限制。新系统需独立供应商账号管理动作权限，不能复用草稿 role=admin 当作完整 RBAC。

## 旧工作流执行顺序

编辑：查邮箱得到员工，不存在拒绝；不能将自己绑定为供应商；来源必须是有效 supplier source；逐个检查订单存在；保留原 metadata 的其他键，写入 supplier 角色、来源和去重订单。新实现用显式结构字段和受控权限表，不能接收客户端任意 metadata。

创建：先验证 supplier source，拒绝同邮箱已有员工，逐个检查订单存在；注册 emailpass 身份，再创建已带限制的员工，最后将登录身份连接到该员工。成功返回员工 ID 与补偿所需的身份 ID。注册失败不创建员工，登录链接失败尝试清理已创建员工和认证身份。

旧 GET 最多读取1000员工后过滤供应商，返回 email/source_id/order_ids；这不是可靠分页。新列表需服务端筛选分页及 count，DTO 无密码、密码散列、认证身份材料；不因超过1000人静默漏账号。旧接口不存在仅传 password 更新现有密码的分支，不能把创建报重复账号改成隐式重置密码。

供应商订单绑定只授权限定设备命令，通用 `/admin/orders` 列表及详情仍拒绝；GET、导出和嵌套 DTO 不得绕过。授予/移除订单、改变来源、禁用账号后，当前会话与在途命令应检查新的权限版本，不能等待 Redis 缓存自然过期才生效。

## 实际提取与失败观察

`capture-supplier-account-rules.cjs` 从真实 TypeScript AST 仅提取 prepareAccount、createSupplierAccount 的执行与补偿回调，针对人工端口运行20项断言。文件 fixture 只记录人工选项、端口调用次序及断言通过，不写测试密码或真实业务行。MedusaError/StepResponse、容器、用户/认证/订单/来源端口均为显式替身，未加载框架或连接数据库，不能证明 SQL 事务或 provider 行为。

覆盖正常去重/保留字段、非管理员先拒绝、不存在账号/来源/订单、自绑定、重复账号、注册失败/缺身份/抛错、带限制后链接、创建/链接失败清理和框架补偿顺序。

提取结果也明确保留旧缺陷：链接失败时若删除员工抛错，后续认证身份删除不会执行；删除认证身份的错误会覆盖最初创建失败；框架补偿若先删认证失败，员工清理会跳过。这些是旧代码在人工故障下的观察，不能作为 Gin 目标兼容要求。新系统持久化账号/授权应尽可能在同一 MySQL 事务内完成，外部身份任务如仍需要则用可恢复的状态和可靠任务；失败必须不留下可登录的无限制账号，保留原始错误关联和清理重试状态。

```powershell
node retail/scripts/capture-request-validation.cjs --check
node retail/scripts/capture-supplier-account-rules.cjs --check
```

## 新接口结构与显式操作

新增 `schemas/gin-common.schema.json`、`schemas/supplier-accounts.schema.json` 和 `schemas/supplier-account-operations.json`。这些是设计并通过结构校验的独立 Gin 契约，尚无处理器；不是旧 Zod 自动转换。操作表定义4项：

| 操作 | 路径 | 必须条件 |
| --- | --- | --- |
| GET 列表 | /api/admin/devices/supplier-accounts | supplier_account.read，服务端分页/过滤/稳定排序 |
| GET 明细 | /api/admin/devices/supplier-accounts/{staff_id} | 当前新员工ID，范围及supplier模式检查，返回强ETag |
| POST 创建 | /api/admin/devices/supplier-accounts | supplier_account.create，显式密码、来源、订单，Origin/CSRF/幂等键；201新账号 |
| PUT 替换绑定 | /api/admin/devices/supplier-accounts/{staff_id}/binding | supplier_account.bind，目标ID、access_mode=supplier、If-Match及Origin/CSRF/幂等键；200最新绑定 |

编辑不再依赖“省略密码表示绑定”的隐式重载；不接受 email/password/metadata，按路径 staff_id 操作。非supplier员工转为supplier另需 staff.access.replace，必须替换为受限访问模式，不能保留可调用通用后台的管理员授权；禁止绑定自己。旧UI按邮箱选账号改为明确选中目标员工，显示实际目标/来源/订单集合及模式变化；不按邮箱自动找到并继承旧身份。

新账号邮箱限定无空白ASCII email，UI先trim；显示大小写保留，独立canonical键做ASCII lower，员工命名空间内唯一。客户同邮箱独立，不认领历史订单。ID保持大小写，输入不做数字转字符串。订单输入仍允许重复，规范化为二进制排序的唯一集合后检查/幂等hash/持久化；空数组仍无订单权限。

新接口拒绝未标准化的邮箱/ID空白，旧界面已先trim邮箱，新Ant表单需同样处理ID；密码全程保留UTF-8原文，以Unicode码点计12—128，不trim、截断或大小写转换。当前Gin草稿bcrypt和登录max72不能满足该边界，后续T014需安全KDF和登录一致实现。幂等账本不能存密码原文或未加密请求；请求指纹使用独立受保护版本密钥的域隔离HMAC，不能对密码做普通SHA指纹。密钥/散列实现未验证。

绑定替换用强If-Match：缺少428、格式错误400、版本不符409；实体与权限版本为十进制字符串，运行时另限制signed BIGINT范围，避免JS精度损失。相同key及规范化请求重放先验证当前权限，再返回原结果，不因重放旧If-Match误报冲突；同key改参数或前置版本409，新key带旧版本也409。进行中的重复操作返回202及受同等授权的操作查询位置，通用操作查询端点仍待T017契约/实现。

响应白名单只含staff_id/email/source_id/order_ids/enabled/access_mode/version/permission_version/时间；禁用状态的修改走统一员工管理能力，不在绑定接口夹带。错误表定义15个领域/通用码及状态，字段错误不携带原始密码、SQL或私有目标信息。GET Query结构是解码后的类型；原始URL需显式解析并拒绝重复/未知参数、科学计数、无界分页，schema本身不执行HTTP解码。

Ant Design承接：Table查询列表，Form/Select选择来源及明确员工，创建才显示Input.Password；订单输入保留换行/空格/中英文逗号分隔的旧交互，提交前标准化为ID数组。编辑不复用创建密码字段。显示空订单集合的含义；409重新读取版本供用户检查，pending保持原幂等键，不在报错后自动发起另一笔创建。校验和请求失败保留表单，成功后清空密码及内存提交数据。禁止把密码写URL、localStorage或通用日志。

`retail/contracts` 用独立锁定的Ajv校验2020-12 JSON Schema，不依赖Medusa包。27组固定输入、10组生成的长度/Unicode/数组边界、8组响应白名单及12组头部/ID边界样例通过；4个操作及15个错误码的schema引用完整。结构校验不证明授权、HTTP解析、数据库版本/唯一性、密码KDF或事务实现。

```powershell
npm ci --prefix retail/contracts --ignore-scripts --no-audit --no-fund
npm test --prefix retail/contracts
```

尚缺：统一员工选择及权限API、原始HTTP解码、KDF/密钥、MySQL并发唯一约束和失败回滚、会话撤权、混合范围负面用例，以及Ant表单/列表真实状态。其余领域schemas继续补齐，不将此领域结构校验视为T007全量验收。
