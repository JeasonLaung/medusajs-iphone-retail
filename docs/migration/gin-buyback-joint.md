# 回收客户与后台审核真实联合验证

2026-10-30。使用实际 Astro 路由、原问卷与回收账号组件、登录表单和签名 bridge，连接生产 Gin 二进制与独立 MySQL/Redis。无浏览器 API 拦截，后台页面由测试专用 Astro fixture 挂载实际 React Admin，API 全部为生产路由；无新增 Gin 测试 HTTP 路由，无真实客户或旧账户导入。

## 本次已证明

1. 新建的人工账号经真实登录表单获得 HttpOnly 会话，完整填写原型号/容量/九步问卷及联系地址，创建第一笔申请；当前配置生成 294 EUR 估价快照。
2. 浏览器将两张真实 PNG 压缩为 WebP，经签名桥接上传，Gin/MySQL 保存元数据及私有字节；页面预览成功解码。填写 SN/IMEI 后提交审核，刷新页面保留 photo_review、SN-JOINT、原 IMEI 和两张可解码照片。
3. 实际后台登录后查看照片，未审核完照片时批准寄件被拒绝；重拍理由不能为空。要求重拍后，客户补传正面照片并重新提交。后台审核两张最新照片、填写寄件说明并批准；客户填写 TRACK-JOINT 确认寄出；后台确认收货，客户刷新为 received。账户站内提醒随重拍、批准寄件、收货依次更新，未读提醒合并为一条最新状态。后台英文桌面及中文 390px 手机页面均检查通过。
4. 成功提交后，原问卷会清空答案；第二笔申请重新经过完整问卷和申请表创建，经原确认弹框取消，刷新后为 canceled 且有 closed_at。列表读到两笔申请。
5. 缺失 CSRF 的命令和直连未签名 Gin 均拒绝。退出并登录另一人工账号，原申请及照片为 404、自己的列表为空、原详情页显示失败而不泄露信息。
6. 第二阶段后台选择匹配的人工待检测设备并关联，填写 270.01 EUR 及检测说明发送报价；客户在原手机页面接受，双方刷新保持 accepted。关联不发送客户通知，报价更新恰好一条；独立 SQL 另验证客户拒绝进入退回申请。详见 [设备与报价](gin-buyback-inspection.md)。
7. 浏览器完成后独立 SQL 核对：2 申请、2 显式归属/提交回执、3 照片元数据、3 私有字节记录、1 已打款或已退回且已关闭、1 取消且已关闭、1 SN-JOINT、另一账号 0 归属。浏览器没有 hydration 错误。

## 运行与资源

`python retail/scripts/test-buyback-joint.py`。可用 `ADMIN_TEST_NODE` 指定 Node 22，`ADMIN_TEST_BROWSER` 指定 Chrome；`BUYBACK_JOINT_BACKEND` 可指向与待验证代码一致的隔离 backend 目录。本次后端来自提交 3b0a512 的独立快照，加已交付后台审核、检测与报价实现和人工数据准备测试，未混入工作区尚未完成的 Mollie 改动。

运行器使用已有 mysql:8.4/redis:7-alpine 镜像，新建带随机所有权标签的容器与独立 bridge；MySQL 单核 768 MiB、Redis 单核 128 MiB，数据库不发布端口，Gin 仅发布随机回环端口。Astro 使用回环 19383 且 strictPort，不抢占现有服务。测试结束按所有权校验清理两个容器、网络及临时运行器。后端快照位于临时目录，可用于复查。

本轮增加后台审核生产实现，见 [审核交付范围](gin-buyback-admin-review.md)。Go 生产 API 与测试运行器编译、77 个正式数据库迁移及人工准备测试实际执行通过；JS 语法和 Python 编译检查通过。旧 9001 health/app 与 8000 dk-en/sell 均 200，近期 stderr 未新增，既有 locales 404 保留。

## 尚未证明

这是客户创建→照片→后台审核→寄出→后台收货→设备关联→最终报价→客户接受后登记打款（或拒绝后退回送达）及另单取消的联合证据，不能据此认定回收全流程完成。实际转账/承运商送达外部证明、通知邮箱、历史图片导入及 180 天清理仍待后续交付。人工 HTTP 环境没有覆盖 HTTPS Secure cookie。页面主体原样对照范围见 [账号 UI](gin-buyback-account-ui.md)，不声称整站 Next/Ant6 像素验收。

打款/退回登记的最新范围见 [登记与退回](gin-buyback-settlement.md)。

[打款联合报告](buyback-joint-report.json) · [退回联合报告](buyback-return-joint-report.json)
