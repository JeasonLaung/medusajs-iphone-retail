# Medusa 本机试用环境

设备库存、手机扫码及订单售后闭环已加入后台：[设备扫码工作台](http://127.0.0.1:9001/app/device-scan)。使用流程与手机 HTTPS 设置见 [DEVICE-INVENTORY.md](DEVICE-INVENTORY.md)。

Medusa 2 后端与管理后台位于 `medusa-store/apps/backend`，购物前台位于 `medusa-store/apps/storefront`。PostgreSQL 和 Redis 由 `compose.yaml` 启动；它们只监听本机。管理后台运行在 http://127.0.0.1:9001/app，购物前台在 http://127.0.0.1:8000/dk，健康检查为 http://127.0.0.1:9001/health。

管理员账号与密码在本机 `.admin-credentials` 中。此文件和 `.env` 都被忽略，不应提交。

## 启动

双击本目录的 `start.bat`，会启动数据库、Medusa 管理后台和购物前台。重复运行时，已启动的服务会跳过；启动日志在 `logs/` 目录。

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

```powershell
cd C:\Users\Administrator\Desktop\other\iphone_retail\medusa-trial
taskkill /PID (Get-Content logs/medusa.pid) /T /F
taskkill /PID (Get-Content logs/storefront.pid) /T /F
docker compose down
```

`docker compose down` 保留数据库卷和试用数据。这个目录与现有 `iphone-retail` 项目独立。

## Swappie iPhone 目录快照

用户提供的 HTML 保存在 `imports/swappie-iphone-source.html`，并已提取到 `imports/swappie-iphone-catalog.json`，其中有 30 个型号卡片、30 个真实 SKU、欧元起价、型号可选容量与颜色，以及原始 `releaseYear` 编号和对应发布年份。`releaseYear` 是内部编号，年份已对照 Apple 的[机型识别页面](https://support.apple.com/en-us/108044)核实。30 张商品图已下载到 `medusa-store/apps/storefront/public/swappie/`，商品图片使用本机 URL。入库脚本是 `medusa-store/apps/backend/src/scripts/import-swappie.ts`；重复运行只更新需要补充的商品字段。

购物前台的 `/dk/store` 只显示这些 iPhone，并提供型号、价格、容量、颜色、电池、成色、发布年份、屏幕尺寸、相机筛选。容量和颜色是来源页面列出的**型号可选项**；用户另行确认所有 30 款型号都提供 85% 和 100% 两种电池选项，以及 S、A 两种成色等级，记录在 `imports/swappie-filter-supplements.json`。这些筛选反映型号可选项，不代表已入库的单个 SKU 有两种电池或成色配置及对应价格。每个型号在原 HTML 中只有一个可核实的 SKU 与价格。原 HTML 对屏幕和相机只提供筛选标题，因此另据 Apple 的[机型资料](https://support.apple.com/en-us/108044)及各机型的技术规格页补齐 30 款机型规格，存于 `imports/iphone-model-specs.json`。包含发布年份、Apple 型号编号、标称屏幕尺寸与类型、分辨率、像素密度、芯片、前后相机、机身三围与重量、接口及身份验证方式。每款数据含对应 Apple 技术规格链接，由 `scripts/extract-apple-iphone-specs.py` 生成，入库后保存在**商品级** `apple_model_specs` 元数据并显示于商品详情，不写入 SKU/variant。Apple 官方列出的容量和配色也作为机型资料留存，不代表本店 SKU 或库存。商品价格是来源页面快照中的参考起价，并非实时同步价格。

商品卡片上的颜色圆点也从原 HTML 提取，30 款共有 134 个圆点，色值保持原页面的十六进制值，顺序保持原卡片顺序。颜色名称按来源的 `availableColors` 和 Apple 机型颜色资料与圆点对应。iPhone 11、11 Pro、SE 2020 的卡片圆点比 `availableColors` 更完整；两组原始数据分别保存在 `card_colors` 与 `available_colors`，商品级元数据亦分别保留。列表颜色圆点和 Colour 筛选使用 `card_colors`，不代表有对应颜色的已核实 SKU、价格或库存。

## 售后申请

订单确认页和账户订单详情页已加入 `After-sales support`，点击 `Request after-sales support` 可选择退货退款、换货或维修，勾选订单商品和数量，并填写问题说明。提交需要登录且订单属于当前账户；游客订单需先通过账户中的订单转移流程关联。每个订单同时只允许一个待审核或处理中的申请，申请关闭后可以再次申请。

申请保存在独立的 `after_sales_request` 表。管理后台订单详情底部的 `After-sales requests` 可查看申请，并标记处理中、完成或拒绝；这些状态操作不会自动退款、创建退货单或发出替换商品，实际处理仍使用 Medusa 的订单操作。

新环境启动前，在 `medusa-store/apps/backend` 执行 `corepack pnpm exec medusa db:migrate`。接口回归检查为 `corepack pnpm exec medusa exec ./src/scripts/check-after-sales.ts`，它会创建独立测试订单和客户并清理，不修改真实订单；运行时需后端已监听 9001 端口。设置 `PLAYWRIGHT_MODULE_PATH` 为本机 Playwright 包路径后，还会检查 8000 端口前台的游客提示、桌面/手机表单、实际提交和状态持久化，截图保存到 `logs/after-sales-form-*.png`。

## 流程测试库存

运行 `cd medusa-store/apps/backend; corepack pnpm exec medusa exec ./src/scripts/seed-swappie-demo-stock.ts`，将 30 个已导入 SKU 在默认销售渠道对应仓库中的库存设为每款 100 件。重复运行会把数量恢复为 100，可能覆盖流程测试中产生的库存变化。这些库存是**测试数据**，不代表 Swappie 或本店实际现货；前台也有提示。
