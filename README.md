# Medusa 本机试用环境

设备库存、手机扫码及订单售后闭环已加入后台：[设备入库出库](http://127.0.0.1:9001/app/device-scan)。使用流程与手机 HTTPS 设置见 [DEVICE-INVENTORY.md](DEVICE-INVENTORY.md)。

Medusa 2 后端与管理后台位于 `medusa-store/apps/backend`，购物前台位于 `medusa-store/apps/storefront`。PostgreSQL 和 Redis 由 `compose.yaml` 启动；它们只监听本机。管理后台运行在 http://127.0.0.1:9001/app，购物前台在 http://127.0.0.1:8000/dk，健康检查为 http://127.0.0.1:9001/health。

管理员账号与密码在本机 `.admin-credentials` 中。此文件和 `.env` 都被忽略，不应提交。

## 启动

### 开发期间保持服务可用

前后台的 `pnpm run dev` 均通过 `scripts/dev-service.cjs` 启动托管进程，意外退出后自动重试，连续启动失败时逐步延长间隔，最多 30 秒；主动停止不会自动拉起。启动中的实例也有独占锁，避免重复启动。后端启动前运行 Medusa lint，错误需要先修正。

后端等待文件停止变化 2 秒后统一更新。接口和工作流优先使用 Medusa 2.21.2 内置的实验性 HMR；共享库、模块、配置变化或 HMR 失败时，只串行执行一次完整重启。完整重启仍会短暂中断服务。该实现使用当前 Medusa 的内部 IPC 协议，升级 Medusa 后需重新验证。测试、独立脚本和迁移文件不触发重启，迁移仍需按需执行。Admin 和商城前端保留原有热更新。

Agent 批量改后端时，从本目录执行以下命令。暂停期间后端继续响应已加载的代码，恢复后统一应用修改；即使验证失败也必须恢复。暂停 30 分钟后自动过期，较长任务需续期。

```powershell
node scripts/dev-service.cjs backend pause
# 写完一批相关文件、完成必要检查或迁移后，在 finally/清理步骤中恢复
node scripts/dev-service.cjs backend resume
```

必要时手动请求一次重启：`node scripts/dev-service.cjs backend restart`。主动停止使用：

```powershell
node scripts/dev-service.cjs backend stop
node scripts/dev-service.cjs storefront stop
```

控制命令提交请求，托管进程会在当前更新完成后处理；通过日志确认执行结果。`logs/dev-backend.lock`、`logs/dev-storefront.lock` 记录托管进程 PID。不要手工删除运行实例的锁文件。

不要在运行开发服务的同一目录执行构建或删除 `.next` / `.medusa`；这会破坏开发产物。先做 lint/typecheck，完整构建使用独立检出目录，或主动停止对应服务、完成构建后重新启动。独立稳定预览还需要隔离数据库、Redis 和后台任务配置，当前不自动复制业务数据或启动第二套后台。

回归测试：`node --test scripts/dev-service.test.cjs`。根目录及 `medusa-store/AGENTS.md` 已同步开发服务规则。

双击本目录的 `start.bat`，会启动数据库、Medusa 管理后台和购物前台。重复运行时，已启动的服务会跳过；启动日志在 `logs/` 目录。

macOS/Linux 可在项目根目录运行 `./sh/start.sh`。它会在后台启动 Docker 服务、Medusa 后端和购物前台；本次启动的输出追加到根目录 `log/yyyymmddHHMM.log`。再次启动时，已记录的运行中进程会跳过。同一分钟内再次启动会继续追加到同名日志。

首次运行前，需要配置根目录 `.env` 中的 `POSTGRES_PASSWORD`、`medusa-store/apps/backend/.env` 中的 `DATABASE_URL`（连接本机 5433 端口）、`REDIS_URL`（本机 6380 端口），以及 `medusa-store/apps/storefront/.env` 中的 `NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY`。后端和前台目录分别有 `.env.template` 可作参考。新数据库还需在后端目录运行 `pnpm exec medusa db:migrate`；初始化数据会创建前台使用的 publishable key。配置文件均被 Git 忽略。

运行 `./sh/log.sh` 可持续查看最新日志，`./sh/log.sh yyyymmddHHMM` 可查看指定启动时间的日志；按 Ctrl+C 退出查看。运行 `./sh/stop.sh` 停止脚本启动的后端、前台及 Docker 服务，并保留数据库卷。

```powershell
cd C:\Users\Administrator\Desktop\other\iphone_retail\medusa-trial
docker compose up -d --wait
cd medusa-store\apps\backend
corepack pnpm run dev
```

在另一个终端启动购物前台：

```powershell
cd C:\Users\Administrator\Desktop\other\iphone_retail\medusa-trial\medusa-store\apps\storefront
corepack pnpm run dev
```

当前后台运行的实例由 `logs/medusa.pid` 记录，购物前台由 `logs/storefront.pid` 记录。重新手动启动前，先停止相应实例。Medusa 项目使用 pnpm 10.11.1；`corepack pnpm` 会使用项目声明的版本。

## 停止

停止 frp 隧道：`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/stop-frp.ps1`。

```powershell
cd C:\Users\Administrator\Desktop\other\iphone_retail\medusa-trial
node scripts/dev-service.cjs backend stop
node scripts/dev-service.cjs storefront stop
docker compose down
```

`docker compose down` 保留数据库卷和试用数据。这个目录与现有 `iphone-retail` 项目独立。

## frp 内网穿透

客户端连接 `43.156.133.126:7000`，使用 TCP 隧道将服务器现有 Nginx 回源端口 `8081` 转发到本机 `127.0.0.1:18080` 网关，通过 `https://frp.xeappliance.com` 访问项目。配置位于 `frp/frpc.toml`。整个 `frp/` 目录仅在本地保留，配置、模板、凭据及程序文件均不进入 Git；新检出项目时需单独准备本地配置。

双击 `start.bat` 会在前后台就绪后自动启动 frpc；已有服务运行时，可单独双击 `start-frp.bat`。首次启动会通过 `scripts/install-frp.ps1` 下载官方 Windows amd64 frp 0.71.0，并核对官方 SHA256。日志为 `logs/frpc.out.log` 和 `logs/frpc.err.log`，进程号为 `logs/frpc.pid`。修改配置后先运行 `scripts/stop-frp.ps1`，再启动。

公网前台入口为 `https://frp.xeappliance.com/`，后台为 `https://frp.xeappliance.com/app`。本地网关将 `/app`、`/admin`、`/auth`、`/store`、`/health`、`/static`、`/uploads`、`/mobile-intake` 路径转发到后台 `9001`，其余路径转发到前台 `8000`。手机入库链接可从 `https://frp.xeappliance.com/app/device-scan/intake` 创建，后端地址使用 `https://frp.xeappliance.com`。网关仅监听本机，进程号记录在 `logs/frp-gateway.pid`；停止 frp 的脚本也会停止网关。浏览器无需指定端口。不再使用 18000/19001 代理端口。

证书和 Cloudflare 回源方式由服务器现有入口负责。前台 `.env.local` 中 `NEXT_PUBLIC_BASE_URL=https://frp.xeappliance.com`、`NEXT_PUBLIC_FRP_DOMAIN=frp.xeappliance.com`；浏览器访问该域名时 API 使用同源地址，服务端请求仍使用本机后台。网关保留公网转发头，并对后台同源 Origin 和 Host 做本机转换。

已验证 frpc 认证与代理注册成功，HTTPS 根路径跳转到 `/dk`，前台 `/dk/store`、后台 `/app` 和 `/health` 曾返回 200，前台浏览器无 localhost 请求。本地网关和后台静态资源均正常，但公网加载时出现间歇性 502 与 frpc session shutdown/写入超时，后台登录表单尚未通过浏览器验证。已启用代理压缩与 10 秒心跳，客户端会自动重连；服务器配置保持原样。

配置格式与校验命令见 [frp 官方文档](https://gofrp.org/zh-cn/docs/features/common/configure/)。

## Swappie iPhone 目录快照

用户提供的 HTML 保存在 `imports/swappie-iphone-source.html`，并已提取到 `imports/swappie-iphone-catalog.json`，其中有 30 个型号卡片、30 个真实 SKU、欧元起价、型号可选容量与颜色，以及原始 `releaseYear` 编号和对应发布年份。`releaseYear` 是内部编号，年份已对照 Apple 的[机型识别页面](https://support.apple.com/en-us/108044)核实。30 张商品图已下载到 `medusa-store/apps/storefront/public/swappie/`，商品图片使用本机 URL。入库脚本是 `medusa-store/apps/backend/src/scripts/import-swappie.ts`；重复运行只更新需要补充的商品字段。

购物前台的 `/dk/store` 只显示这些 iPhone，并提供型号、价格、容量、颜色、电池、成色、发布年份、屏幕尺寸、相机筛选。容量和颜色是来源页面列出的**型号可选项**；用户另行确认所有 30 款型号都提供 85% 和 100% 两种电池选项，以及 S、A 两种成色等级，记录在 `imports/swappie-filter-supplements.json`。容量筛选和列表容量标签现在按已录入 SKU 的容量生成；电池、颜色、成色仍反映机型可选项，不代表已入库 SKU 提供这些配置及对应价格。每个型号在原 HTML 中只有一个可核实的 SKU 与价格。原 HTML 对屏幕和相机只提供筛选标题，因此另据 Apple 的[机型资料](https://support.apple.com/en-us/108044)及各机型的技术规格页补齐 30 款机型规格，存于 `imports/iphone-model-specs.json`。包含发布年份、Apple 型号编号、标称屏幕尺寸与类型、分辨率、像素密度、芯片、前后相机、机身三围与重量、接口及身份验证方式。每款数据含对应 Apple 技术规格链接，由 `scripts/extract-apple-iphone-specs.py` 生成，入库后保存在**商品级** `apple_model_specs` 元数据并显示于商品详情，不写入 SKU/variant。Apple 官方列出的容量和配色也作为机型资料留存，不代表本店 SKU 或库存。商品价格是来源页面快照中的参考起价，并非实时同步价格。

商品卡片上的颜色圆点也从原 HTML 提取，30 款共有 134 个圆点，色值保持原页面的十六进制值，顺序保持原卡片顺序。颜色名称按来源的 `availableColors` 和 Apple 机型颜色资料与圆点对应。iPhone 11、11 Pro、SE 2020 的卡片圆点比 `availableColors` 更完整；两组原始数据分别保存在 `card_colors` 与 `available_colors`，商品级元数据亦分别保留。列表颜色圆点和 Colour 筛选使用 `card_colors`，不代表有对应颜色的已核实 SKU、价格或库存。

## iPhone 系列类目

列表卡片采用圆角边框、容量标签、机型配色与独立价格区。容量标签及 `storage_options` 搜索筛选只读取真实 variant 的 Storage / Capacity 选项，不再使用机型参考元数据 `available_storages`。调整搜索转换逻辑后，可在后台目录运行 `corepack pnpm exec medusa exec ./src/scripts/reindex-product-search.ts` 重建商品搜索索引；该脚本通过搜索模块切换索引版本，不修改商品、SKU 或库存。

后台“商品类目”中的 `iPhone` 下已按机型建立子类目：iPhone 11、12、13、14、15、16、17、SE 和 Air。16e / 17e 分别归入 16 / 17，SE 2020 / 2022 归入 SE。30 款目录商品同时保留父类目关联，确保前台现有 `iPhone` 筛选仍能找到全部商品。系列用于商品分类，库存仍按 SKU 与单台设备管理。同步脚本会将旧的来源网站命名类目原位改名为 `iPhone`（handle 为 `iphone`），保留类目 ID 和父子关系；原始导入文件和来源元数据继续用于溯源。

导入脚本 `import-swappie.ts` 在导入完成后自动同步系列。也可以在 `medusa-store/apps/backend` 运行 `corepack pnpm exec medusa exec ./src/scripts/setup-iphone-series.ts` 单独同步；重复执行不会重复创建类目。同步保留商品已有的其他类目，且不会修改 SKU、价格或库存。

首页 `/dk` 展示 9 个 iPhone 系列图片卡片，链接到对应系列类目。14、15、16、17 和 Air 的系列图从用户提供的 `C:\Users\Administrator\Desktop\other\iphone0902\iphone-images` 复制到前台 `public/iphone-series/`；11、12、13 和 SE 使用现有本地商品图。卡片机型数量读取后台类目数据，首页新增文案按现有 `_medusa_locale` 语言偏好显示中文或英文。

iPhone 父类目与系列子类目页复用 `/store` 的搜索列表、筛选侧栏、价格排序和颜色圆点商品卡片。系列范围作为固定筛选条件叠加到用户筛选上，清除筛选仍保留当前系列范围；其他类目继续使用原有类目模板。

## 售后申请

订单确认页和账户订单详情页已加入 `After-sales support`，点击 `Request after-sales support` 可选择退货退款、换货或维修，勾选订单商品和数量，并填写问题说明。提交需要登录且订单属于当前账户；游客订单需先通过账户中的订单转移流程关联。每个订单同时只允许一个待审核或处理中的申请，申请关闭后可以再次申请。

申请保存在独立的 `after_sales_request` 表。管理后台订单详情底部的 `After-sales requests` 可查看申请，并标记处理中、完成或拒绝；这些状态操作不会自动退款、创建退货单或发出替换商品，实际处理仍使用 Medusa 的订单操作。

新环境启动前，在 `medusa-store/apps/backend` 执行 `corepack pnpm exec medusa db:migrate`。接口回归检查为 `corepack pnpm exec medusa exec ./src/scripts/check-after-sales.ts`，它会创建独立测试订单和客户并清理，不修改真实订单；运行时需后端已监听 9001 端口。设置 `PLAYWRIGHT_MODULE_PATH` 为本机 Playwright 包路径后，还会检查 8000 端口前台的游客提示、桌面/手机表单、实际提交和状态持久化，截图保存到 `logs/after-sales-form-*.png`。

## 流程测试库存

运行 `cd medusa-store/apps/backend; corepack pnpm exec medusa exec ./src/scripts/seed-swappie-demo-stock.ts`，将 30 个已导入 SKU 在默认销售渠道对应仓库中的库存设为每款 100 件。重复运行会把数量恢复为 100，可能覆盖流程测试中产生的库存变化。这些库存是**测试数据**，不代表 Swappie 或本店实际现货；前台也有提示。
