# Windows 本机启动

仓库根目录双击 `start.bat`，启动当前 Gin / React Admin / Astro。MySQL 按用户要求使用 Docker；Redis 使用本机服务，不启动旧 Medusa。原启动入口保留为 `start-medusa.bat`。

准备 Go、Node ≥22.23.4、Docker Desktop 和本机 Redis，以及 `retail/admin`、`retail/storefront` 的 npm 依赖。现有 MySQL 5.7 不能承载此项目的迁移结构；本项目通过 `compose.local.yml` 使用专用 MySQL 9.6 容器和持久卷 `eurorefurb-retail-local-mysql`，监听本机 13307，不覆盖其他项目的 3306 数据库。

本机连接与密钥写在 Git 忽略的 `retail/backend/.env` 和 `retail/storefront/.env`，可参考各目录 `.env.example`。backend 使用 `APP_BIND=127.0.0.1`、`APP_PORT=9100`、`ADMIN_ORIGIN=http://127.0.0.1:9200`、`STOREFRONT_ORIGIN=http://127.0.0.1:9300`；storefront 使用 `GIN_API_ORIGIN=http://127.0.0.1:9100`、相同 `STOREFRONT_ORIGIN`。MySQL 使用项目专用数据库，Redis 使用本机无密码6379和独立 `SESSION_REDIS_PREFIX=eurorefurb:windows-local:v1:`。迁移必须显式执行 `go run ./cmd/migrate`，启动脚本不会自动改表或开启外部付款/邮件。

启动先通知旧开发监督进程退出，再关闭 9001、8000、9100、9200、9300 的旧监听进程，保留本机 MySQL/Redis。脚本编译当前 Gin，再依次启动并检查：

- API：http://127.0.0.1:9100/health
- Admin：http://127.0.0.1:9200
- 商店：http://127.0.0.1:9300/dk-zh/store

进程在后台运行，日志与 PID 保存在 `logs/retail-native/`。再次执行脚本会关闭旧项目监听后重新启动；启动失败会指出对应日志文件。可通过 `RETAIL_NODE_EXE` 指定 Node；当前系统版本过旧时也会检测本机已有的 Codex Node runtime。本次新建后台账号 `admin@eurorefurb.local`，随机密码保存在 Git 忽略的 `.private-files/local/admin-credentials.txt`；原 Medusa 的 `.admin-credentials` 不适用于新后台。

本机已从 `mysql-schema102.sql.gz` 恢复专用库 `eurorefurb_local_trial`，显式执行0103字段迁移，重放确认0条待应用。数据库密码及新本机账户加密密钥仅保存在忽略文件中，旧机的私有包没有恢复；历史加密配置及未携带的上传文件可能不可读取。真实付款、退款及外发邮件保持关闭。换机请携带数据库、对应私有配置及必要文件，不能仅依赖本脚本恢复数据。

## Windows Redis 空闲连接

本机 Redis 的空闲超时为65秒。项目客户端在30秒时退休空闲连接，避免 Windows 闲置后首次请求因旧连接关闭而短暂503；不会自动重试会话写入，也不会修改共享 Redis 的服务器设置。专项说明见 `docs/migration/windows-redis-idle.md`。
