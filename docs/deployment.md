# Docker 与 GitHub Actions 生产部署

同一个仓库，前后台分别构建镜像，通过 SSH 更新服务器上的独立容器。自动部署监听 `main`，不影响本地 `start.bat`、开发 Compose 或开发数据库。

GitHub 上的 BuildKit 使用单核（CPU 0，配额 1 CPU）、4 GB 内存上限，前后端串行构建。生产应用设置了独立内存与 swap 上限；单核并不等于内存安全，构建失败不会替换正在运行的应用。

目标服务器：`root@47.83.217.5:22`。前台：`https://shop.eurorefurb.com`；后台：`https://shop-admin.eurorefurb.com/app`。

## 触发规则

| 变化 | 构建、部署 |
| --- | --- |
| `medusa-store/apps/backend/**`（包含 Admin、迁移、静态页面） | backend |
| `medusa-store/apps/storefront/**` | storefront |
| 两者都有变化 | 两者 |
| `medusa-store/shared/**`、锁文件、workspace/package 配置、patches、公共 lint/turbo 配置 | 两者 |
| `deploy/**`、`.dockerignore`、部署 workflow、变更检测脚本 | 两者 |
| 根级文档、本地开发启动脚本及开发 Compose | 不部署 |

自动推送与该分支上最近一次**成功的自动部署 workflow**比较，所以失败或被后续推送取代的变更会进入下一次自动部署。首次运行部署两者。GitHub 对同一 concurrency group 只保留一个等待中的运行；基于最后成功运行比较，可覆盖被替换推送的变更。手动部署不推进自动比较基线，后续可能保守地重建服务。

Actions → Deploy production → Run workflow，可选择 `all`、`backend`、`storefront`。首次服务器部署必须选 `all`；首次新数据库的初始化步骤见下文。工作流只在所选镜像全部构建成功后开始部署，发布任务串行执行，不取消运行中的迁移。

## GitHub 配置

在仓库 Settings → Secrets and variables → Actions 配置以下 **Repository secrets**（也可在 `production` Environment 内配置部署 secrets）：

| Secret | 内容 |
| --- | --- |
| `DEPLOY_HOST` | `47.83.217.5` |
| `DEPLOY_USER` | `root`，或有部署目录和 Docker 权限的专用用户 |
| `DEPLOY_SSH_KEY` | 用户提供的 SSH 私钥全文 |
| `DEPLOY_KNOWN_HOSTS` | 核实过指纹的服务器 known_hosts 条目；22 端口通常以 IP 开头 |

不要将密钥提交到 Git，也不要仅凭未核实的 `ssh-keyscan` 输出建立信任。可复用本地已确认的 `~/.ssh/known_hosts` 对应条目。非标准端口需要 `[host]:port` 格式。

配置以下 **Repository variables**。镜像构建任务使用仓库变量，不读取仅放在 Environment 内的变量：

| Variable | 值 |
| --- | --- |
| `NEXT_PUBLIC_MEDUSA_BACKEND_URL` | `https://shop-admin.eurorefurb.com` |
| `NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY` | 生产数据库真实的 publishable API key，须关联销售渠道 |
| `NEXT_PUBLIC_BASE_URL` | `https://shop.eurorefurb.com` |
| `NEXT_PUBLIC_DEFAULT_REGION` | `dk`（可省略） |
| `DEPLOY_ROOT` | `/opt/iphone-retail`（可省略；仅支持字母、数字、`_`、`-`、`/`） |
| `DEPLOY_SSH_PORT` | `22`（可省略） |

支付可选变量：`NEXT_PUBLIC_STRIPE_KEY`、`NEXT_PUBLIC_MEDUSA_PAYMENTS_PUBLISHABLE_KEY`、`NEXT_PUBLIC_MEDUSA_PAYMENTS_ACCOUNT_ID`。不配置时保持现有支付逻辑。

创建名为 `production` 的 GitHub Environment，按团队需要设置部署分支限制。镜像发布使用 workflow 自带的 `GITHUB_TOKEN`，无需另外配置推送 token。若仓库限制 package 写权限，需要在仓库设置中允许相应权限。

`NEXT_PUBLIC_*` 在 Next.js 构建时写入浏览器脚本。更换域名或公开 key 后，同步服务器 `storefront.env` 并手动重新部署 storefront；仅重启容器不会更新浏览器配置。容器内服务端请求使用 `MEDUSA_INTERNAL_BACKEND_URL=http://backend:9000`；浏览器请求仍使用公开 HTTPS 后台域名。本地未设置该变量时保持原有行为。

## 服务器准备

要求：Linux amd64、Docker Engine、支持 `up --wait` 的 Docker Compose v2（建议 2.24+）、Bash、`flock`、可访问 GHCR 与公开后台域名。镜像采用 Node 22，前后台分别带有运行所需依赖。当前优先保证 pnpm 依赖链接完整，未裁剪所有开发依赖，镜像体积会较大。

2026-10-26 只读检查：目标服务器为 Debian amd64，Docker 29.1.3、Compose v5.0.1，8000/9001 未被占用；80/443 已由现有宝塔 Nginx 使用，已有其他业务容器，应保留。该检查没有修改服务器配置。

当次检查总内存约 3.5 GB，available 约 595 MB，另有 1 GB swap；磁盘剩余约 17 GB。新增 Medusa/Next.js 前应评估现有业务峰值与镜像占用，必要时扩容；构建由 GitHub runner 执行。服务器现已为本项目新增独立 2 GB swap（加上原有 1 GB，共 3 GB），没有修改现有业务的资源配置。两个生产域名尚未解析，按用户要求先完成服务器内部部署，DNS 和正式 HTTPS 随后启用。

服务器只需保存部署配置和持久化数据，不必检出业务源代码。初始化目录：

```bash
sudo install -d -m 700 /opt/iphone-retail
cd /opt/iphone-retail
# 将 deploy/ 内三个示例文件上传到服务器后：
cp .env.example .env
cp backend.env.example backend.env
cp storefront.env.example storefront.env
chmod 600 .env backend.env storefront.env
```

填写真实随机密码、JWT/Cookie 和支付/邮件加密密钥。`.env` 的 `POSTGRES_PASSWORD` 与 `backend.env` 的数据库密码必须一致；建议用十六进制随机密码避免数据库 URL 转义问题。迁移现有数据库时必须保留原加密密钥，否则已保存的支付/邮件凭据无法解密。

数据库和 Redis 只在 Docker 网络可见；前后台分别监听服务器回环地址 9001 和 8000。若端口已被使用，调整 `.env` 的 `BACKEND_PORT` / `STOREFRONT_PORT` 并同步反向代理。Compose 固定使用独立项目 `iphone-retail`。

自动部署使用短期 `GITHUB_TOKEN` 在服务器项目独立的 `.docker` 目录登录 GHCR，发布结束退出登录，不覆盖其他项目的凭据。下面的一次性手动登录仅用于人工初始化或故障处理。对于私有 GHCR 镜像，用具有 `read:packages` 权限且能访问此仓库 package 的 token 在服务器登录一次（通过标准输入，勿在命令行写 token）：

```bash
read -rs -p 'GHCR read token: ' GHCR_TOKEN
printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u YOUR_GITHUB_USER --password-stdin
unset GHCR_TOKEN
```

服务器登录用户须与 workflow 的 SSH 用户一致。首次 package 创建后，必要时在 GitHub package 设置中授予此仓库访问权限。

## 首次数据库初始化

先准备生产数据库、真实 publishable key 和 DK region，再完整部署前台。不要用假 key 构建上线。

若这是全新服务器，先手动运行 workflow，选择 `backend`，取消勾选 `deploy`，仅发布后端镜像，不启动服务器服务。然后在服务器上传 `deploy/compose.production.yaml`，以实际镜像 SHA 初始化后端（此步骤不启动 storefront，STOREFRONT_IMAGE 暂为同 SHA 引用即可）：

```bash
cd /opt/iphone-retail
export DEPLOY_ROOT=/opt/iphone-retail
export BACKEND_IMAGE=ghcr.io/jeasonlaung/medusajs-iphone-retail/backend:COMMIT_SHA
export STOREFRONT_IMAGE=ghcr.io/jeasonlaung/medusajs-iphone-retail/storefront:COMMIT_SHA
docker compose --env-file .env -f compose.production.yaml up -d --wait postgres redis
docker compose --env-file .env -f compose.production.yaml pull backend
docker compose --env-file .env -f compose.production.yaml run --rm --no-deps backend node node_modules/@medusajs/cli/cli.js db:migrate
docker compose --env-file .env -f compose.production.yaml up -d --no-deps --wait backend
```

`db:migrate` 会执行项目自带的 migration scripts；当前包含初始商店/销售渠道/地区和 publishable key 的 seed。确认初始化结果，避免重复手动 seed。然后按 Medusa CLI 创建生产管理员（不要复用本机调试密码）：

```bash
docker compose --env-file .env -f compose.production.yaml exec backend node node_modules/@medusajs/cli/cli.js user -e YOUR_ADMIN_EMAIL -p YOUR_STRONG_PASSWORD
docker compose --env-file .env -f compose.production.yaml exec backend node node_modules/@medusajs/cli/cli.js exec ./src/scripts/setup-warehouse-rbac.js
```

创建用户命令中的密码可能进入 shell 历史，应在受控终端执行并按服务器策略处理。RBAC 初始化只在迁移或首次建站时执行，不在每次部署时重设权限。将真实 publishable key 填入 GitHub Repository variable 与服务器 `storefront.env`，补齐 SSH secrets，重新手动运行 `all` 并勾选 `deploy`。确认后台 HTTPS 可访问后再验证前台。

如果迁移现有业务数据，应先备份并恢复数据库、上传文件及密钥，保持现有 key/region 配置；本方案不会把本机数据库或上传文件自动复制到服务器。

## 域名与 HTTPS

当前使用宝塔 Nginx，站点配置目录是 `/www/server/panel/vhost/nginx/`。DNS 未就绪时先准备 HTTP 反向代理站点，通过 Host 请求核对；DNS 就绪后在宝塔为两个域名申请并启用正式 SSL。生产浏览器 API、登录安全 Cookie 与支付回调需要正式 HTTPS 后才能完整使用。

DNS 将 `shop.eurorefurb.com` 和 `shop-admin.eurorefurb.com` 指向 `47.83.217.5`。服务器上 Nginx 的参考配置在 `deploy/nginx.conf.example`，需合并进现有站点配置并为两个域名申请证书，再执行 `nginx -t` 和 reload。已有 Caddy、面板或其他反向代理时，复用现有入口。

前台域名代理到 `127.0.0.1:8000`；后台域名的所有路径代理到 `127.0.0.1:9001`，包括 `/app`、`/store`、`/static`、`/mobile-intake`、支付回调。保留 Host 和 X-Forwarded-Proto 等转发头；后台启用生产安全 Cookie，需要 HTTPS。CORS 必须与两个实际 origin 一致。不要把后台域名重定向到 `/app`，否则 API 和文件路径会失效。

## 发布、检查与恢复

每份镜像使用 Git commit SHA 标记。服务器 `deploy.sh` 会获得文件锁、复用 PostgreSQL/Redis、只拉取选中的应用镜像，后端先运行一次 `db:migrate`，再用 `up --no-deps --wait` 更新对应容器。迁移失败时后端原容器继续运行；应用健康检查失败时尝试恢复原镜像。迁移需要保持与旧版本兼容，破坏性迁移须另行安排维护。

每成功更新一个服务就独立保存 `.images.env`；前端失败不会丢失已成功后端的版本记录。`.backend.previous-image` 和 `.storefront.previous-image` 保存前一个镜像引用。首次没有旧容器时无法自动恢复。镜像恢复不回滚数据库或上传文件，也不保证零停机；单副本切换期间可能有短暂中断。失败时查看 Actions 输出和服务器日志，修复后手动重跑对应服务，或等下一次自动部署重试。

日常检查：

```bash
cd /opt/iphone-retail
export DEPLOY_ROOT=/opt/iphone-retail
set -a
source .images.env
set +a
docker compose --env-file .env -f compose.production.yaml ps
docker compose --env-file .env -f compose.production.yaml logs --tail 100 backend storefront
curl -f https://shop-admin.eurorefurb.com/health
curl -I https://shop-admin.eurorefurb.com/app
curl -I https://shop.eurorefurb.com/dk
```

恢复指定旧镜像时，读取对应 previous-image 文件，替换 `.images.env` 中该服务的镜像，再使用上面的环境加载命令以及 `docker compose ... up -d --no-deps --wait backend`（或 storefront）。恢复前检查迁移兼容性；不要执行 `down -v`。PostgreSQL、Redis、公开上传和私有上传使用独立命名卷；定期备份数据库和文件。

## 本地验证

```bash
node --test scripts/deploy-changes.test.cjs
# Linux 上另行测试迁移失败、健康检查恢复和前后端独立状态保存：
node --test scripts/deploy-release.test.cjs
docker build -f deploy/backend.Dockerfile -t iphone-retail-backend:check .
docker build -f deploy/storefront.Dockerfile \
  --build-arg NEXT_PUBLIC_MEDUSA_BACKEND_URL=https://shop-admin.eurorefurb.com \
  --build-arg NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY=YOUR_REAL_PUBLISHABLE_KEY \
  --build-arg NEXT_PUBLIC_BASE_URL=https://shop.eurorefurb.com \
  -t iphone-retail-storefront:check .
```

构建在 Docker 内完成，不写入本地开发服务的 `.medusa` / `.next`。Next.js 当前项目配置会跳过构建时 TypeScript/ESLint 检查，镜像构建沿用该行为；Medusa 保留现有构建检查。

参考：[Medusa 部署](https://docs.medusajs.com/learn/deployment/general)、[Next.js 构建时环境变量](https://nextjs.org/docs/app/guides/environment-variables)。
