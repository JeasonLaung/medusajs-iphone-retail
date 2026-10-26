# 2026-10-26 生产迁移记录

部署目标为宝塔服务器 `47.83.217.5`。前台域名 `shop.eurorefurb.com`，后台域名 `shop-admin.eurorefurb.com`。按用户要求，DNS 和正式 HTTPS 稍后设置。

## 已执行

- 以本机已提交的 dev `e13f9f0b2c7b4b14eb18d44cc62b62c647db640f` 为基础，将部署文件及 Docker 内网 API 访问支持发布到 main。未提交的业务改动没有发布。本机仍在 dev，工作区改动保留。
- 配置 GitHub Actions secrets 和构建变量，并将默认分支设置为 main，使手动 workflow 可使用。
- GitHub BuildKit 限制一个逻辑 CPU、4 GB 内存，前后台串行构建；服务器只拉取镜像。
- 在服务器独立项目 `iphone-retail` 中创建 PostgreSQL、Redis、公开与私有上传卷。没有迁移 Redis 临时缓存。
- 恢复本机一致性 pg_dump：86 个商品、5 位客户、13 笔订单、3 个后台用户；保留原有加密密钥以解密数据库内凭据。
- 迁移 418 个公开/私有文件，逐文件 SHA256 与备份一致。
- 仅在生产副本中将本机/开发隧道图片地址转换为 `https://shop-admin.eurorefurb.com/static/...`。
- 通过宝塔原生站点方法注册两个站点，ID 分别为 9 和 10，配置独立 Nginx 反向代理，并通过 `nginx -t` 后 reload。
- 为本项目新增 2 GB swap，现有 1 GB 保留，共 3 GB。应用与数据服务配置了各自内存/swap 上限。
- 修正 Docker PostgreSQL 的 `sslmode=disable` 和临时迁移任务依赖目录写权限；应用仍以 node 用户运行。

本机备份目录为 `backups/production-migration-20261026-142909`（Git 忽略）；服务器备份与配置在 `/opt/iphone-retail/`，目录仅 root 可读。

## 运行结果及未解决的问题

[生产部署运行](https://github.com/JeasonLaung/medusajs-iphone-retail/actions/runs/40283799899)成功。2026-10-26 15:10（上海时间）部署版本 `7cdaf19a7b4a89b2a597ee076b4a2ecf4648c087` 的 backend、storefront、PostgreSQL、Redis 均显示 healthy。

随后本机到服务器 SSH 握手与 HTTP 访问开始超时；15:17 的[独立验证任务](https://github.com/JeasonLaung/medusajs-iphone-retail/actions/runs/40284843766)也在 SSH banner exchange 超时。进一步探测 22、80 和宝塔 36868 端口均无应用响应，无法完成最终稳定性、登录及代理验证。原因尚未确认，不能据此认定服务器已经稳定上线。

已请求通过云控制台检查服务器资源和连接。恢复时先限定范围清理本次临时发布脚本，再停止新增的两个应用容器，以便排查，不停止其他项目：

```bash
pkill -TERM -f '^bash /opt/iphone-retail/releases/manual-' || true
docker stop -t 5 iphone-retail-backend-1 iphone-retail-storefront-1
free -m
uptime
docker ps
```

不要仅依据历史 workflow success 判断当前运行状态。需要恢复 SSH 后检查系统日志、OOM、swap/I/O 和内存峰值，再决定容量调整与重新启动。两个域名还需解析到服务器并在宝塔启用正式 SSL，之后才能完整验证浏览器、登录、支付回调和图片访问。
